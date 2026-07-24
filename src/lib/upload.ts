/**
 * Custom image upload helper for event posters.
 */

import { readFileSync, existsSync, statSync, writeFileSync, unlinkSync } from 'fs';
import { basename, extname, join } from 'path';
import { tmpdir } from 'os';
import { randomBytes } from 'crypto';
import type { PartifulConfig } from './auth.js';

const ALLOWED_EXTENSIONS = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.avif'];
const MAX_SIZE = 10 * 1024 * 1024; // 10MB
const MIME_TYPES: Record<string, string> = {
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.webp': 'image/webp',
  '.avif': 'image/avif',
};

/** The uploadData object returned by the uploadPhoto endpoint. */
export interface UploadData {
  url: string;
  contentType?: string;
  height?: number;
  width?: number;
  [extra: string]: unknown;
}

/** The image object we attach to an event when using a custom upload. */
export interface UploadImage {
  source: 'upload';
  type: 'image';
  upload: UploadData;
  url: string;
  contentType?: string;
  name: string;
  height?: number;
  width?: number;
}

/** A downloaded temp file plus its cleanup handle. */
export interface TempDownload {
  tempPath: string;
  cleanup: () => void;
}

export async function uploadEventImage(
  filePath: string,
  token: string,
  config: PartifulConfig | null | undefined,
  verbose?: boolean,
): Promise<UploadData> {
  if (!existsSync(filePath)) {
    throw new Error(`File not found: ${filePath}`);
  }

  const ext = extname(filePath).toLowerCase();
  if (!ALLOWED_EXTENSIONS.includes(ext)) {
    throw new Error(`Unsupported image type "${ext}". Allowed types: ${ALLOWED_EXTENSIONS.join(', ')}`);
  }

  const stat = statSync(filePath);
  if (stat.size > MAX_SIZE) {
    throw new Error(`File too large: ${(stat.size / 1024 / 1024).toFixed(1)}MB exceeds 10MB limit`);
  }

  const fileData = readFileSync(filePath);
  const contentType = MIME_TYPES[ext] || 'application/octet-stream';
  const blob = new Blob([fileData], { type: contentType });
  const formData = new FormData();
  formData.append('file', blob, basename(filePath));

  const url = 'https://us-central1-getpartiful.cloudfunctions.net/uploadPhoto?uploadType=event_poster';

  if (verbose) {
    console.error(`[upload] POSTing ${basename(filePath)} (${stat.size} bytes) to uploadPhoto`);
  }

  const uploadTimeoutMs = Number(config?.uploadTimeoutMs ?? 30000);
  const uploadController = new AbortController();
  const uploadTimeoutId = setTimeout(() => uploadController.abort(), uploadTimeoutMs);

  let response: Response;
  try {
    response = await fetch(url, {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body: formData,
      signal: uploadController.signal,
    });
  } catch (err) {
    if ((err as Error).name === 'AbortError') {
      throw new Error(`Upload timed out after ${uploadTimeoutMs / 1000}s`);
    }
    throw err;
  } finally {
    clearTimeout(uploadTimeoutId);
  }

  if (!response.ok) {
    throw new Error(`Upload failed: ${response.status} ${response.statusText}`);
  }

  let result: { uploadData?: UploadData; result?: { uploadData?: UploadData } };
  try {
    result = (await response.json()) as typeof result;
  } catch {
    throw new Error('Upload failed: invalid JSON response body');
  }

  const uploadData = result?.uploadData ?? result?.result?.uploadData;
  if (!uploadData || typeof uploadData.url !== 'string' || !uploadData.url) {
    throw new Error('Upload failed: missing uploadData.url in response');
  }

  return uploadData;
}

const CONTENT_TYPE_TO_EXT: Record<string, string> = {
  'image/png': '.png',
  'image/jpeg': '.jpg',
  'image/gif': '.gif',
  'image/webp': '.webp',
  'image/avif': '.avif',
};

export async function downloadToTemp(url: string): Promise<TempDownload> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 15000);

  let response: Response;
  try {
    response = await fetch(url, { signal: controller.signal });
  } catch (err) {
    clearTimeout(timeout);
    if ((err as Error).name === 'AbortError') {
      throw new Error(`Download timed out after 15s: ${url}`);
    }
    throw new Error(`Download failed: ${(err as Error).message}`);
  } finally {
    clearTimeout(timeout);
  }

  if (!response.ok) {
    throw new Error(`Download failed: ${response.status} ${response.statusText} from ${url}`);
  }

  const contentType = (response.headers.get('content-type') || '').split(';')[0]!.trim();
  const ext = CONTENT_TYPE_TO_EXT[contentType];
  if (!ext) {
    throw new Error(`Unsupported content type "${contentType}" from ${url}. Expected an image type.`);
  }

  const contentLength = Number(response.headers.get('content-length') || 0);
  if (contentLength && contentLength > MAX_SIZE) {
    throw new Error(`Download failed: ${(contentLength / 1024 / 1024).toFixed(1)}MB exceeds 10MB limit`);
  }

  const buffer = Buffer.from(await response.arrayBuffer());
  if (buffer.length > MAX_SIZE) {
    throw new Error(`Download failed: ${(buffer.length / 1024 / 1024).toFixed(1)}MB exceeds 10MB limit`);
  }

  const rand = randomBytes(8).toString('hex');
  const tempPath = join(tmpdir(), `partiful-upload-${rand}${ext}`);
  writeFileSync(tempPath, buffer);

  return {
    tempPath,
    cleanup() {
      try {
        unlinkSync(tempPath);
      } catch {
        /* best-effort cleanup */
      }
    },
  };
}

export function buildUploadImage(uploadData: UploadData, filename: string): UploadImage {
  return {
    source: 'upload',
    type: 'image',
    upload: uploadData,
    url: uploadData.url,
    contentType: uploadData.contentType,
    name: filename,
    height: uploadData.height,
    width: uploadData.width,
  };
}
