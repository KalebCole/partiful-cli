/**
 * Custom image upload helper for event posters.
 */

import { readFileSync, existsSync, statSync } from 'fs';
import { basename, extname } from 'path';

const ALLOWED_EXTENSIONS = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.avif'];
const MAX_SIZE = 10 * 1024 * 1024; // 10MB

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
  const blob = new Blob([fileData]);
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
