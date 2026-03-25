/**
 * Custom image upload helper for event posters.
 */

import { readFileSync, existsSync, statSync, writeFileSync, unlinkSync } from 'fs';
import { basename, extname } from 'path';
import { randomBytes } from 'crypto';

const ALLOWED_EXTENSIONS = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.avif'];
const MAX_SIZE = 10 * 1024 * 1024; // 10MB
const MIME_TYPES = {
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.webp': 'image/webp',
  '.avif': 'image/avif',
};

export async function uploadEventImage(filePath, token, config, verbose) {
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

  const response = await fetch(url, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: formData,
  });

  if (!response.ok) {
    throw new Error(`Upload failed: ${response.status} ${response.statusText}`);
  }

  const result = await response.json();
  return result.uploadData || result.result?.uploadData || result;
}

const CONTENT_TYPE_TO_EXT = {
  'image/png': '.png',
  'image/jpeg': '.jpg',
  'image/gif': '.gif',
  'image/webp': '.webp',
  'image/avif': '.avif',
};

export async function downloadToTemp(url) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 15000);

  let response;
  try {
    response = await fetch(url, { signal: controller.signal });
  } catch (err) {
    clearTimeout(timeout);
    if (err.name === 'AbortError') {
      throw new Error(`Download timed out after 15s: ${url}`);
    }
    throw new Error(`Download failed: ${err.message}`);
  } finally {
    clearTimeout(timeout);
  }

  if (!response.ok) {
    throw new Error(`Download failed: ${response.status} ${response.statusText} from ${url}`);
  }

  const contentType = (response.headers.get('content-type') || '').split(';')[0].trim();
  const ext = CONTENT_TYPE_TO_EXT[contentType];
  if (!ext) {
    throw new Error(`Unsupported content type "${contentType}" from ${url}. Expected an image type.`);
  }

  const buffer = Buffer.from(await response.arrayBuffer());
  const rand = randomBytes(8).toString('hex');
  const tempPath = `/tmp/partiful-upload-${rand}${ext}`;
  writeFileSync(tempPath, buffer);

  return {
    tempPath,
    cleanup() {
      try { unlinkSync(tempPath); } catch {}
    },
  };
}

export function buildUploadImage(uploadData, filename) {
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
