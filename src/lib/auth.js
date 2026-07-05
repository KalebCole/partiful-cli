/**
 * Authentication module for Partiful CLI.
 * Handles credential loading, token refresh, and payload wrapping.
 */

import fs from 'fs';
import path from 'path';
import crypto from 'crypto';

const GOOGLE_TOKEN_URL = 'securetoken.googleapis.com';

export function resolveCredentialsPath() {
  return process.env.PARTIFUL_CREDENTIALS_FILE
    || path.join(process.env.HOME, '.config/partiful/auth.json');
}

export function loadConfig() {
  // Check env var for direct token
  if (process.env.PARTIFUL_TOKEN) {
    return { accessToken: process.env.PARTIFUL_TOKEN, tokenExpiry: Date.now() + 3600000 };
  }

  const configPath = resolveCredentialsPath();
  if (!fs.existsSync(configPath)) {
    throw new Error(`No auth config found at ${configPath}. Run 'partiful auth login' first.`);
  }
  return JSON.parse(fs.readFileSync(configPath, 'utf8'));
}

export function saveConfig(config) {
  const configPath = resolveCredentialsPath();
  const configDir = path.dirname(configPath);
  if (!fs.existsSync(configDir)) {
    fs.mkdirSync(configDir, { recursive: true });
  }
  fs.writeFileSync(configPath, JSON.stringify(config, null, 2));
}

export async function refreshAccessToken(config) {
  const postData = `grant_type=refresh_token&refresh_token=${config.refreshToken}`;

  const resp = await fetch(`https://${GOOGLE_TOKEN_URL}/v1/token?key=${config.apiKey}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
      'Referer': 'https://partiful.com/'
    },
    body: postData,
  });

  const result = await resp.json();
  if (result.error) {
    throw new Error(result.error.message || 'Token refresh failed');
  }
  return result;
}

export async function getValidToken(config) {
  if (config.accessToken && config.tokenExpiry) {
    const now = Date.now();
    if (now < config.tokenExpiry - 60000) {
      return config.accessToken;
    }
  }

  const result = await refreshAccessToken(config);
  config.accessToken = result.id_token;
  config.tokenExpiry = Date.now() + (parseInt(result.expires_in) * 1000);

  if (result.refresh_token) {
    config.refreshToken = result.refresh_token;
  }

  // Self-heal: older auth.json files predate userId capture. Backfill it from
  // the token here (the refresh path already persists config below), so
  // host detection and any userId-dependent payloads work without re-login.
  // Note: the env-var token path returns early above and never writes to disk.
  if (!config.userId) {
    const uid = getUserIdFromToken(config.accessToken);
    if (uid) config.userId = uid;
  }

  saveConfig(config);
  return config.accessToken;
}

/**
 * Decode the (unverified) payload of a Firebase JWT.
 *
 * The CLI does not need to verify the signature — the token was already issued
 * to us by Firebase and is only decoded to read identity claims. Returns null
 * for anything that is not a well-formed three-segment JWT.
 *
 * @param {string} token JWT string (header.payload.signature).
 * @returns {object|null} Decoded payload object, or null on any parse failure.
 */
export function decodeJwtPayload(token) {
  if (typeof token !== 'string') return null;
  const parts = token.split('.');
  if (parts.length !== 3) return null;
  try {
    // JWTs use base64url; normalise to base64 before decoding.
    const b64 = parts[1].replace(/-/g, '+').replace(/_/g, '/');
    return JSON.parse(Buffer.from(b64, 'base64').toString('utf8'));
  } catch {
    return null;
  }
}

/**
 * Extract the authenticated user's Partiful user ID from a Firebase token.
 * Firebase ID tokens carry the UID in both `user_id` and the standard `sub`
 * claim; we prefer `user_id` and fall back to `sub`.
 *
 * @param {string} token Firebase JWT.
 * @returns {string|null} The user ID, or null if it cannot be determined.
 */
export function getUserIdFromToken(token) {
  const payload = decodeJwtPayload(token);
  if (!payload || typeof payload !== 'object') return null;
  return payload.user_id || payload.sub || null;
}

export function wrapPayload(config, params = {}) {
  return {
    ...params,
    amplitudeDeviceId: config.amplitudeDeviceId || generateAmplitudeDeviceId(),
  };
}

export function generateAmplitudeDeviceId() {
  return crypto.randomBytes(12).toString('base64').replace(/[+/=]/g, '');
}
