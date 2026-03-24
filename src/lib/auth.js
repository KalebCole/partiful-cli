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

  saveConfig(config);
  return config.accessToken;
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
