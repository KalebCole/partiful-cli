/**
 * Auth commands: login, logout, status
 *
 * Login flow (2026-03-24):
 *   1. sendAuthCode(phoneNumber) → SMS sent
 *   2. Auto-retrieve code via imsg (macOS) or prompt user
 *   3. getLoginToken(phoneNumber, authCode) → custom JWT
 *   4. signInWithCustomToken → Firebase idToken + refreshToken
 *   5. Save to ~/.config/partiful/auth.json
 *
 * See docs/research/2026-03-24-auth-flow-endpoints.md
 */

import fs from 'fs';
import path from 'path';
import os from 'os';
import { execSync, spawnSync } from 'child_process';
import readline from 'readline';
import { loadConfig, saveConfig, getValidToken, resolveCredentialsPath, generateAmplitudeDeviceId } from '../lib/auth.js';
import { jsonOutput, jsonError } from '../lib/output.js';

const FIREBASE_API_KEY = 'AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k';
const API_BASE = 'https://api.partiful.com';
const IDENTITY_TOOLKIT = 'https://identitytoolkit.googleapis.com';
const PARTIFUL_SMS_SENDER = '+18449460698';
const CODE_POLL_INTERVAL_MS = 3000;
const CODE_POLL_TIMEOUT_MS = 120000; // 2 minutes

// ─── Platform Detection ───────────────────────────────────────

function detectPlatform() {
  const platform = os.platform();

  if (platform === 'darwin') {
    // macOS — check for imsg CLI
    const hasImsg = hasCommand('imsg');
    return { os: 'macos', canAutoRetrieve: hasImsg, method: hasImsg ? 'imsg' : 'manual' };
  }

  if (platform === 'linux') {
    // Could be Android via Termux or regular Linux
    // Check for Android-specific paths or tools
    const isAndroid = fs.existsSync('/system/build.prop') || process.env.ANDROID_ROOT;
    if (isAndroid) {
      // TODO: Android SMS retrieval (content://sms/inbox via termux-sms-list)
      const hasTermuxSms = hasCommand('termux-sms-list');
      return { os: 'android', canAutoRetrieve: hasTermuxSms, method: hasTermuxSms ? 'termux-sms' : 'manual' };
    }
    return { os: 'linux', canAutoRetrieve: false, method: 'manual' };
  }

  if (platform === 'win32') {
    return { os: 'windows', canAutoRetrieve: false, method: 'manual' };
  }

  return { os: platform, canAutoRetrieve: false, method: 'manual' };
}

function hasCommand(cmd) {
  try {
    execSync(`which ${cmd}`, { stdio: 'ignore' });
    return true;
  } catch {
    return false;
  }
}

// ─── SMS Code Retrieval ───────────────────────────────────────

async function pollForCodeImsg(phoneNumber, sentAt) {
  const deadline = Date.now() + CODE_POLL_TIMEOUT_MS;
  console.error('Watching for SMS verification code via iMessage...');

  while (Date.now() < deadline) {
    try {
      // Find the Partiful SMS chat
      const chatsRaw = execSync('imsg chats --limit 30 --json', { encoding: 'utf8', timeout: 10000 });
      const chats = chatsRaw.trim().split('\n').map(line => JSON.parse(line));

      const partifulChat = chats.find(c =>
        c.identifier === PARTIFUL_SMS_SENDER ||
        c.identifier?.includes('8449460698')
      );

      if (partifulChat) {
        const historyRaw = execSync(`imsg history --chat-id ${partifulChat.id} --limit 3 --json`, {
          encoding: 'utf8', timeout: 10000
        });
        const messages = historyRaw.trim().split('\n').map(line => JSON.parse(line));

        for (const msg of messages) {
          const msgTime = new Date(msg.created_at).getTime();
          if (msgTime >= sentAt - 5000) { // within 5s of send
            const codeMatch = msg.text?.match(/(\d{6})\s+is your Partiful verification code/);
            if (codeMatch) {
              console.error(`✓ Code received: ${codeMatch[1]}`);
              return codeMatch[1];
            }
          }
        }
      }
    } catch (e) {
      // imsg failed — continue polling
    }

    await new Promise(r => setTimeout(r, CODE_POLL_INTERVAL_MS));
    const remaining = Math.round((deadline - Date.now()) / 1000);
    if (remaining > 0 && remaining % 15 === 0) {
      console.error(`  Still waiting... (${remaining}s remaining)`);
    }
  }

  return null; // Timed out
}

async function pollForCodeTermux(phoneNumber, sentAt) {
  const deadline = Date.now() + CODE_POLL_TIMEOUT_MS;
  console.error('Watching for SMS verification code via Termux...');

  while (Date.now() < deadline) {
    try {
      const smsRaw = execSync('termux-sms-list -l 10 -t inbox', { encoding: 'utf8', timeout: 10000 });
      const messages = JSON.parse(smsRaw);

      for (const msg of messages) {
        const msgTime = new Date(msg.received).getTime();
        if (msgTime >= sentAt - 5000) {
          const codeMatch = msg.body?.match(/(\d{6})\s+is your Partiful verification code/);
          if (codeMatch) {
            console.error(`✓ Code received: ${codeMatch[1]}`);
            return codeMatch[1];
          }
        }
      }
    } catch (e) {
      // termux-sms-list failed — continue polling
    }

    await new Promise(r => setTimeout(r, CODE_POLL_INTERVAL_MS));
  }

  return null;
}

async function promptForCode() {
  const rl = readline.createInterface({ input: process.stdin, output: process.stderr });
  return new Promise(resolve => {
    rl.question('Enter verification code: ', answer => {
      rl.close();
      resolve(answer.trim());
    });
  });
}

// ─── API Calls ────────────────────────────────────────────────

async function sendAuthCode(phoneNumber) {
  const payload = {
    data: {
      params: {
        displayName: '',
        phoneNumber,
        silent: false,
        channelPreference: 'sms',
        captchaToken: null,
        useAppleBusinessUpdates: false,
      },
      amplitudeDeviceId: generateAmplitudeDeviceId(),
      amplitudeSessionId: Date.now(),
    },
  };

  // Use sendAuthCodeTrusted — doesn't require reCAPTCHA token
  const resp = await fetch(`${API_BASE}/sendAuthCodeTrusted`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Origin': 'https://partiful.com',
      'Referer': 'https://partiful.com/',
    },
    body: JSON.stringify(payload),
  });

  if (!resp.ok) {
    const text = await resp.text().catch(() => '');
    throw new Error(`sendAuthCode failed (${resp.status}): ${text}`);
  }

  return resp.json();
}

async function getLoginToken(phoneNumber, authCode) {
  const payload = {
    data: {
      params: {
        phoneNumber,
        authCode,
        affiliateId: null,
        utms: {},
      },
      amplitudeDeviceId: generateAmplitudeDeviceId(),
      amplitudeSessionId: Date.now(),
    },
  };

  const resp = await fetch(`${API_BASE}/getLoginToken`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Origin': 'https://partiful.com',
      'Referer': 'https://partiful.com/',
    },
    body: JSON.stringify(payload),
  });

  if (!resp.ok) {
    const text = await resp.text().catch(() => '');
    throw new Error(`getLoginToken failed (${resp.status}): ${text}`);
  }

  const result = await resp.json();
  return result?.result?.data || result?.result || result;
}

async function signInWithCustomToken(customToken) {
  const resp = await fetch(
    `${IDENTITY_TOOLKIT}/v1/accounts:signInWithCustomToken?key=${FIREBASE_API_KEY}`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Referer': 'https://partiful.com/',
        'Origin': 'https://partiful.com',
      },
      body: JSON.stringify({ token: customToken, returnSecureToken: true }),
    }
  );

  if (!resp.ok) {
    const text = await resp.text().catch(() => '');
    throw new Error(`signInWithCustomToken failed (${resp.status}): ${text}`);
  }

  return resp.json();
}

async function lookupUser(idToken) {
  const resp = await fetch(
    `${IDENTITY_TOOLKIT}/v1/accounts:lookup?key=${FIREBASE_API_KEY}`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Referer': 'https://partiful.com/',
      },
      body: JSON.stringify({ idToken }),
    }
  );

  if (!resp.ok) return null;
  const result = await resp.json();
  return result?.users?.[0] || null;
}

// ─── Commands ─────────────────────────────────────────────────

export function registerAuthCommands(program) {
  const auth = program.command('auth').description('Manage authentication');

  auth
    .command('status')
    .description('Check authentication status and token validity')
    .action(async (opts, cmd) => {
      try {
        const config = loadConfig();
        const info = {
          user: config.displayName || null,
          phone: config.phoneNumber || null,
          userId: config.userId || null,
          configPath: resolveCredentialsPath(),
          tokenValid: false,
        };

        try {
          await getValidToken(config);
          info.tokenValid = true;
        } catch (e) {
          info.tokenError = e.message;
        }

        jsonOutput(info);
      } catch (e) {
        jsonError(e.message, 2, 'auth_error');
      }
    });

  auth
    .command('login')
    .description('Authenticate via SMS verification code')
    .argument('<phone>', 'Phone number in E.164 format (e.g. +12066993977)')
    .option('--code <code>', 'Skip SMS — provide verification code directly')
    .option('--no-auto', 'Disable auto-retrieval of SMS code')
    .action(async (phone, opts, cmd) => {
      try {
        // Normalize phone number
        let phoneNumber = phone.replace(/[\s\-\(\)]/g, '');
        if (!phoneNumber.startsWith('+')) {
          // Assume US if no country code
          phoneNumber = '+1' + phoneNumber;
        }

        if (!/^\+\d{10,15}$/.test(phoneNumber)) {
          jsonError(`Invalid phone number: ${phoneNumber}. Use E.164 format (+12066993977)`, 3, 'validation_error');
          return;
        }

        const platform = detectPlatform();
        let code = opts.code || null;

        if (!code) {
          // Step 1: Send verification code
          console.error(`\nPartiful Login`);
          console.error(`Phone: ${phoneNumber}`);
          console.error(`Platform: ${platform.os} (code retrieval: ${platform.method})`);
          console.error('');
          console.error('Sending verification code...');

          const sentAt = Date.now();
          await sendAuthCode(phoneNumber);
          console.error('✓ Code sent via SMS');

          // Step 2: Get the code
          if (platform.canAutoRetrieve && opts.auto !== false) {
            console.error('');

            if (platform.method === 'imsg') {
              code = await pollForCodeImsg(phoneNumber, sentAt);
            } else if (platform.method === 'termux-sms') {
              code = await pollForCodeTermux(phoneNumber, sentAt);
            }

            if (!code) {
              console.error('⚠ Auto-retrieval timed out. Enter code manually:');
              code = await promptForCode();
            }
          } else {
            console.error('');
            if (platform.os === 'macos' && !platform.canAutoRetrieve) {
              console.error('Tip: Install imsg CLI for auto-retrieval (npm i -g imsg-cli)');
            }
            code = await promptForCode();
          }
        }

        if (!code || !/^\d{6}$/.test(code)) {
          jsonError('Invalid verification code. Expected 6 digits.', 3, 'validation_error');
          return;
        }

        // Step 3: Get custom login token from Partiful
        console.error('Verifying code...');
        const loginResult = await getLoginToken(phoneNumber, code);
        const customToken = loginResult.token;

        if (!customToken) {
          jsonError('No token received. Code may be expired or invalid.', 2, 'auth_error', { response: loginResult });
          return;
        }

        // Step 4: Exchange for Firebase tokens
        console.error('Exchanging for Firebase tokens...');
        const firebaseResult = await signInWithCustomToken(customToken);

        if (!firebaseResult.idToken || !firebaseResult.refreshToken) {
          jsonError('Firebase token exchange failed', 2, 'auth_error');
          return;
        }

        // Step 5: Look up user info
        const user = await lookupUser(firebaseResult.idToken);

        // Step 6: Save config
        const config = {
          apiKey: FIREBASE_API_KEY,
          refreshToken: firebaseResult.refreshToken,
          accessToken: firebaseResult.idToken,
          tokenExpiry: Date.now() + (parseInt(firebaseResult.expiresIn) * 1000),
          userId: firebaseResult.localId,
          displayName: user?.displayName || '',
          phoneNumber: phoneNumber,
          photoUrl: user?.photoUrl || null,
        };

        saveConfig(config);

        console.error('✓ Authenticated successfully!');
        console.error('');

        jsonOutput({
          user: config.displayName || 'Unknown',
          phone: phoneNumber,
          userId: config.userId,
          configPath: resolveCredentialsPath(),
          platform: platform.os,
          codeMethod: code === opts.code ? 'provided' : platform.method,
        });
      } catch (e) {
        jsonError(e.message, 2, 'auth_error');
      }
    });

  auth
    .command('logout')
    .description('Remove stored credentials')
    .action(async () => {
      const configPath = resolveCredentialsPath();
      if (fs.existsSync(configPath)) {
        fs.unlinkSync(configPath);
        jsonOutput({ removed: true, configPath });
      } else {
        jsonOutput({ removed: false, message: 'Already logged out (no config found)' });
      }
    });
}
