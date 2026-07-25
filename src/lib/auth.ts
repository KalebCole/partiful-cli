/**
 * Authentication module for Partiful CLI.
 * Handles credential loading, token refresh, and payload wrapping.
 */

import fs from 'fs';
import path from 'path';
import crypto from 'crypto';
import type { RefreshTokenResponse } from './api/endpoints.js';

const GOOGLE_TOKEN_URL = 'securetoken.googleapis.com';

/** On-disk auth config. Mutated in place by getValidToken() then persisted. */
export interface PartifulConfig {
  accessToken?: string;
  tokenExpiry?: number;
  refreshToken?: string;
  apiKey?: string;
  userId?: string | null;
  amplitudeDeviceId?: string;
  name?: string;
  uploadTimeoutMs?: number;
  [extra: string]: unknown;
}

/** Decoded Firebase JWT payload (identity claims only; signature unverified). */
export interface JwtPayload {
  user_id?: string;
  sub?: string;
  [claim: string]: unknown;
}

export function resolveCredentialsPath(): string {
  return (
    process.env.PARTIFUL_CREDENTIALS_FILE ||
    path.join(process.env.HOME as string, '.config/partiful/auth.json')
  );
}

export function loadConfig(): PartifulConfig {
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

export function saveConfig(config: PartifulConfig): void {
  const configPath = resolveCredentialsPath();
  const configDir = path.dirname(configPath);
  if (!fs.existsSync(configDir)) {
    fs.mkdirSync(configDir, { recursive: true });
  }
  fs.writeFileSync(configPath, JSON.stringify(config, null, 2));
}

export async function refreshAccessToken(config: PartifulConfig): Promise<RefreshTokenResponse> {
  const postData = `grant_type=refresh_token&refresh_token=${config.refreshToken}`;

  const resp = await fetch(`https://${GOOGLE_TOKEN_URL}/v1/token?key=${config.apiKey}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
      Referer: 'https://partiful.com/',
    },
    body: postData,
  });

  const result = (await resp.json()) as RefreshTokenResponse;
  if (result.error) {
    throw new Error(result.error.message || 'Token refresh failed');
  }
  return result;
}

export async function getValidToken(config: PartifulConfig): Promise<string> {
  if (config.accessToken && config.tokenExpiry) {
    const now = Date.now();
    if (now < config.tokenExpiry - 60000) {
      return config.accessToken;
    }
  }

  const result = await refreshAccessToken(config);
  config.accessToken = result.id_token;
  config.tokenExpiry = Date.now() + parseInt(result.expires_in ?? '0') * 1000;

  if (result.refresh_token) {
    config.refreshToken = result.refresh_token;
  }

  // Self-heal: older auth.json files predate userId capture. Backfill it from
  // the token here (the refresh path already persists config below), so
  // host detection and any userId-dependent payloads work without re-login.
  // Note: the env-var token path returns early above and never writes to disk.
  if (!config.userId) {
    const uid = getUserIdFromToken(config.accessToken!);
    if (uid) config.userId = uid;
  }

  saveConfig(config);
  return config.accessToken!;
}

/**
 * Decode the (unverified) payload of a Firebase JWT.
 *
 * The CLI does not need to verify the signature — the token was already issued
 * to us by Firebase and is only decoded to read identity claims. Returns null
 * for anything that is not a well-formed three-segment JWT.
 */
export function decodeJwtPayload(token: string): JwtPayload | null {
  if (typeof token !== 'string') return null;
  const parts = token.split('.');
  if (parts.length !== 3) return null;
  try {
    // JWTs use base64url; normalise to base64 before decoding.
    const b64 = parts[1]!.replace(/-/g, '+').replace(/_/g, '/');
    return JSON.parse(Buffer.from(b64, 'base64').toString('utf8'));
  } catch {
    return null;
  }
}

/**
 * Extract the authenticated user's Partiful user ID from a Firebase token.
 * Firebase ID tokens carry the UID in both `user_id` and the standard `sub`
 * claim; we prefer `user_id` and fall back to `sub`.
 */
export function getUserIdFromToken(token: string): string | null {
  const payload = decodeJwtPayload(token);
  if (!payload || typeof payload !== 'object') return null;
  return payload.user_id || payload.sub || null;
}

/** The wrapped payload passed as `data` in a firebase-callable envelope. */
export interface WrappedPayload {
  amplitudeDeviceId: string;
  [key: string]: unknown;
}

export function wrapPayload(
  config: PartifulConfig,
  params: Record<string, unknown> = {},
): WrappedPayload {
  return {
    ...params,
    amplitudeDeviceId: config.amplitudeDeviceId || generateAmplitudeDeviceId(),
  };
}

export function generateAmplitudeDeviceId(): string {
  return crypto.randomBytes(12).toString('base64').replace(/[+/=]/g, '');
}
