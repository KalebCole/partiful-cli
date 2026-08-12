# Partiful CLI — Agentic Upgrade Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Transform the 1600-line single-file Partiful CLI into a modular, JSON-first, agent-friendly CLI scoring 8+/10 on the cli-architect quality checklist.

**Architecture:** Modular Node.js CLI using Commander for arg parsing. Shared `lib/` layer (output, errors, http, auth, dates). One file per resource in `commands/`. Helper commands in `helpers/`. JSON envelope on every response. Structured exit codes 0-5.

**Tech Stack:** Node.js, Commander, dotenv, vitest (dev)

**Design Spec:** `docs/plans/2026-03-23-agentic-cli-upgrade-design.md`

---

## Phase 1: Scaffold & Core Lib

### Task 1.1: Initialize Package and Project Structure

**Files:**
- Modify: `package.json` (create)
- Create: `.env.example`
- Create: `bin/partiful`
- Create: `src/cli.js`

**Step 1: Create package.json**

```json
{
  "name": "partiful-cli",
  "version": "2.0.0",
  "description": "CLI for creating and managing Partiful events via API — JSON-first, agent-friendly",
  "type": "module",
  "bin": {
    "partiful": "./bin/partiful"
  },
  "scripts": {
    "test": "vitest run",
    "test:watch": "vitest"
  },
  "dependencies": {
    "commander": "^13.0.0",
    "dotenv": "^16.4.0"
  },
  "devDependencies": {
    "vitest": "^3.0.0"
  },
  "license": "MIT"
}
```

**Step 2: Create .env.example**

```bash
# Partiful CLI Configuration
# PARTIFUL_TOKEN=           # Firebase access token (priority 1)
# PARTIFUL_CREDENTIALS_FILE= # Path to auth.json (priority 2, default: ~/.config/partiful/auth.json)
# PARTIFUL_TIMEOUT=30       # Request timeout in seconds
# PARTIFUL_MAX_RETRIES=3    # Max retry attempts for 429/5xx
# PARTIFUL_FORMAT=json      # Default output format (json|table|csv|ndjson)
```

**Step 3: Create bin/partiful**

```javascript
#!/usr/bin/env node
import 'dotenv/config';
import { run } from '../src/cli.js';
run();
```

**Step 4: Create src/cli.js (skeleton)**

```javascript
import { Command } from 'commander';

export function run() {
  const program = new Command();

  program
    .name('partiful')
    .description('Manage Partiful events from the command line — JSON-first, agent-friendly')
    .version('2.0.0')
    .option('--format <format>', 'Output format: json, table, csv, ndjson', process.env.PARTIFUL_FORMAT || 'json')
    .option('--dry-run', 'Preview request without executing')
    .option('-y, --yes', 'Skip confirmation prompts')
    .option('--force', 'Skip confirmation and overwrite protection')
    .option('-v, --verbose', 'Show request details on stderr')
    .option('-o, --output <path>', 'Write output to file')
    .option('--no-color', 'Disable colored output');

  program.parse();
}
```

**Step 5: Create directory structure**

```bash
mkdir -p src/commands src/helpers src/lib tests skills
```

**Step 6: Install dependencies**

Run: `npm install`

**Step 7: Verify bin entry point works**

Run: `node bin/partiful --help`
Expected: Commander help output with global flags listed.

**Step 8: Commit**

```bash
git add -A
git commit -m "feat: scaffold project structure with commander"
```

---

### Task 1.2: Output Module (`src/lib/output.js`)

**Files:**
- Create: `src/lib/output.js`
- Create: `tests/output.test.js`

**Step 1: Write failing tests**

```javascript
// tests/output.test.js
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { jsonOutput, jsonError, formatOutput } from '../src/lib/output.js';

describe('jsonOutput', () => {
  let stdout;
  beforeEach(() => {
    stdout = '';
    vi.spyOn(process.stdout, 'write').mockImplementation((chunk) => { stdout += chunk; });
  });
  afterEach(() => vi.restoreAllMocks());

  it('wraps data in success envelope', () => {
    jsonOutput({ id: '123', title: 'Party' });
    const parsed = JSON.parse(stdout);
    expect(parsed.status).toBe('success');
    expect(parsed.data.id).toBe('123');
  });

  it('includes metadata when provided', () => {
    jsonOutput({ events: [] }, { count: 0 });
    const parsed = JSON.parse(stdout);
    expect(parsed.metadata.count).toBe(0);
  });
});

describe('jsonError', () => {
  let stdout;
  let exitCode;
  beforeEach(() => {
    stdout = '';
    vi.spyOn(process.stdout, 'write').mockImplementation((chunk) => { stdout += chunk; });
    vi.spyOn(process, 'exit').mockImplementation((code) => { exitCode = code; throw new Error('EXIT'); });
  });
  afterEach(() => vi.restoreAllMocks());

  it('outputs error envelope and exits with code', () => {
    try { jsonError('Not found', 4, 'not_found'); } catch {}
    const parsed = JSON.parse(stdout);
    expect(parsed.status).toBe('error');
    expect(parsed.error.code).toBe(4);
    expect(parsed.error.type).toBe('not_found');
    expect(parsed.error.message).toBe('Not found');
    expect(exitCode).toBe(4);
  });
});
```

**Step 2: Run tests to verify they fail**

Run: `npx vitest run tests/output.test.js`
Expected: FAIL — module not found

**Step 3: Implement output.js**

```javascript
// src/lib/output.js
import fs from 'fs';

/**
 * Write JSON success envelope to stdout.
 * @param {object} data
 * @param {object} [metadata]
 * @param {object} [opts] - { format, output }
 */
export function jsonOutput(data, metadata = {}, opts = {}) {
  const envelope = { status: 'success', data, metadata };
  const json = JSON.stringify(envelope);

  if (opts.output) {
    fs.writeFileSync(opts.output, json + '\n');
  } else {
    process.stdout.write(json + '\n');
  }
}

/**
 * Write JSON error envelope to stdout and exit.
 * @param {string} message
 * @param {number} code - Exit code (1-5)
 * @param {string} type - Error type identifier
 * @param {object} [details]
 */
export function jsonError(message, code = 5, type = 'internal_error', details = null) {
  const envelope = {
    status: 'error',
    error: { code, type, message, ...(details ? { details } : {}) }
  };
  process.stdout.write(JSON.stringify(envelope) + '\n');
  process.exit(code);
}

/**
 * Format data as table string for --format table.
 * @param {Array<object>} rows
 * @param {string[]} columns
 * @returns {string}
 */
export function formatTable(rows, columns) {
  if (!rows || rows.length === 0) return '(no results)';

  const widths = columns.map(col =>
    Math.max(col.length, ...rows.map(r => String(r[col] ?? '').length))
  );

  const header = columns.map((col, i) => col.padEnd(widths[i])).join('  ');
  const sep = widths.map(w => '─'.repeat(w)).join('──');
  const body = rows.map(r =>
    columns.map((col, i) => String(r[col] ?? '').padEnd(widths[i])).join('  ')
  ).join('\n');

  return `${header}\n${sep}\n${body}`;
}

/**
 * Format data as CSV string.
 * @param {Array<object>} rows
 * @param {string[]} columns
 * @returns {string}
 */
export function formatCsv(rows, columns) {
  const escape = (v) => {
    const s = String(v ?? '');
    return s.includes(',') || s.includes('"') || s.includes('\n')
      ? `"${s.replace(/"/g, '""')}"`
      : s;
  };
  const header = columns.map(escape).join(',');
  const body = rows.map(r => columns.map(col => escape(r[col])).join(',')).join('\n');
  return `${header}\n${body}`;
}

// Exit code constants
export const EXIT = {
  SUCCESS: 0,
  API_ERROR: 1,
  AUTH_ERROR: 2,
  VALIDATION_ERROR: 3,
  NOT_FOUND: 4,
  INTERNAL_ERROR: 5,
};
```

**Step 4: Run tests to verify they pass**

Run: `npx vitest run tests/output.test.js`
Expected: PASS

**Step 5: Commit**

```bash
git add src/lib/output.js tests/output.test.js
git commit -m "feat: add output module with JSON envelope, table, CSV formatters"
```

---

### Task 1.3: Error Module (`src/lib/errors.js`)

**Files:**
- Create: `src/lib/errors.js`
- Create: `tests/errors.test.js`

**Step 1: Write failing tests**

```javascript
// tests/errors.test.js
import { describe, it, expect } from 'vitest';
import { PartifulError, ApiError, AuthError, ValidationError, NotFoundError } from '../src/lib/errors.js';
import { EXIT } from '../src/lib/output.js';

describe('error classes', () => {
  it('ApiError has code 1', () => {
    const err = new ApiError('Service unavailable');
    expect(err.exitCode).toBe(EXIT.API_ERROR);
    expect(err.type).toBe('api_error');
  });

  it('AuthError has code 2', () => {
    const err = new AuthError('Token expired');
    expect(err.exitCode).toBe(EXIT.AUTH_ERROR);
    expect(err.type).toBe('auth_error');
  });

  it('ValidationError has code 3', () => {
    const err = new ValidationError('--title is required');
    expect(err.exitCode).toBe(EXIT.VALIDATION_ERROR);
    expect(err.type).toBe('validation_error');
  });

  it('NotFoundError has code 4', () => {
    const err = new NotFoundError('Event not found');
    expect(err.exitCode).toBe(EXIT.NOT_FOUND);
    expect(err.type).toBe('not_found');
  });

  it('toJSON returns error envelope shape', () => {
    const err = new ApiError('fail', { statusCode: 500 });
    const json = err.toJSON();
    expect(json.code).toBe(1);
    expect(json.type).toBe('api_error');
    expect(json.message).toBe('fail');
    expect(json.details.statusCode).toBe(500);
  });
});
```

**Step 2: Run tests to verify they fail**

Run: `npx vitest run tests/errors.test.js`
Expected: FAIL

**Step 3: Implement errors.js**

```javascript
// src/lib/errors.js
import { EXIT } from './output.js';

export class PartifulError extends Error {
  constructor(message, exitCode, type, details = null) {
    super(message);
    this.exitCode = exitCode;
    this.type = type;
    this.details = details;
  }

  toJSON() {
    return {
      code: this.exitCode,
      type: this.type,
      message: this.message,
      ...(this.details ? { details: this.details } : {}),
    };
  }
}

export class ApiError extends PartifulError {
  constructor(message, details) {
    super(message, EXIT.API_ERROR, 'api_error', details);
  }
}

export class AuthError extends PartifulError {
  constructor(message, details) {
    super(message, EXIT.AUTH_ERROR, 'auth_error', details);
  }
}

export class ValidationError extends PartifulError {
  constructor(message, details) {
    super(message, EXIT.VALIDATION_ERROR, 'validation_error', details);
  }
}

export class NotFoundError extends PartifulError {
  constructor(message, details) {
    super(message, EXIT.NOT_FOUND, 'not_found', details);
  }
}
```

**Step 4: Run tests**

Run: `npx vitest run tests/errors.test.js`
Expected: PASS

**Step 5: Commit**

```bash
git add src/lib/errors.js tests/errors.test.js
git commit -m "feat: add structured error classes with exit code mapping"
```

---

### Task 1.4: Date Parsing Module (`src/lib/dates.js`)

**Files:**
- Create: `src/lib/dates.js`
- Create: `tests/dates.test.js`

**Step 1: Write failing tests**

```javascript
// tests/dates.test.js
import { describe, it, expect, vi, afterEach } from 'vitest';
import { parseDateTime } from '../src/lib/dates.js';

describe('parseDateTime', () => {
  afterEach(() => vi.useRealTimers());

  it('parses "Apr 15 7pm"', () => {
    vi.useFakeTimers(new Date('2026-03-23T12:00:00'));
    const d = parseDateTime('Apr 15 7pm');
    expect(d.getMonth()).toBe(3); // April
    expect(d.getDate()).toBe(15);
    expect(d.getHours()).toBe(19);
  });

  it('parses "tomorrow"', () => {
    vi.useFakeTimers(new Date('2026-03-23T12:00:00'));
    const d = parseDateTime('tomorrow');
    expect(d.getDate()).toBe(24);
    expect(d.getHours()).toBe(19); // default 7pm
  });

  it('parses "next Friday 8pm"', () => {
    vi.useFakeTimers(new Date('2026-03-23T12:00:00')); // Monday
    const d = parseDateTime('next Friday 8pm');
    expect(d.getDay()).toBe(5); // Friday
    expect(d.getHours()).toBe(20);
  });

  it('parses ISO date', () => {
    const d = parseDateTime('2026-04-15T19:00:00');
    expect(d.getFullYear()).toBe(2026);
  });

  it('throws on garbage', () => {
    expect(() => parseDateTime('not a date')).toThrow();
  });
});
```

**Step 2: Run tests to verify they fail**

Run: `npx vitest run tests/dates.test.js`
Expected: FAIL

**Step 3: Extract date parsing from old `partiful` file**

Copy the `parseDateTime`, `parseTimeString`, `hasExplicitYear`, `needsYearFix`, `tryAddYear`, `stripMarkdown`, `formatDate` functions from the original `partiful` file into `src/lib/dates.js`. Export them as named exports.

```javascript
// src/lib/dates.js

export function parseDateTime(dateStr, timezone = 'America/Los_Angeles') {
  // [exact copy of existing parseDateTime from old partiful file, lines ~200-260]
  const lower = dateStr.trim().toLowerCase();
  const now = new Date();

  if (lower === 'tomorrow') {
    const d = new Date(now);
    d.setDate(d.getDate() + 1);
    d.setHours(19, 0, 0, 0);
    return d;
  }

  const nextDayMatch = lower.match(/^next\s+(sunday|monday|tuesday|wednesday|thursday|friday|saturday)(?:\s+(.+))?$/i);
  if (nextDayMatch) {
    const dayNames = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'];
    const targetDay = dayNames.indexOf(nextDayMatch[1].toLowerCase());
    const d = new Date(now);
    let daysAhead = targetDay - d.getDay();
    if (daysAhead <= 0) daysAhead += 7;
    d.setDate(d.getDate() + daysAhead);
    if (nextDayMatch[2]) {
      const timeParsed = parseTimeString(nextDayMatch[2].trim());
      if (timeParsed) {
        d.setHours(timeParsed.hours, timeParsed.minutes, 0, 0);
      } else {
        d.setHours(19, 0, 0, 0);
      }
    } else {
      d.setHours(19, 0, 0, 0);
    }
    return d;
  }

  const cleanStr = dateStr.replace(/(\d{1,2})(am|pm)/i, '$1:00 $2');
  let date = new Date(cleanStr);

  if (isNaN(date.getTime()) || needsYearFix(dateStr, date)) {
    const withYear = tryAddYear(dateStr, now);
    if (withYear) {
      const cleanWithYear = withYear.replace(/(\d{1,2})(am|pm)/i, '$1:00 $2');
      date = new Date(cleanWithYear);
    }
  }

  if (isNaN(date.getTime())) {
    throw new Error(`Could not parse date: ${dateStr}`);
  }

  if (!hasExplicitYear(dateStr) && date < now) {
    date.setFullYear(date.getFullYear() + 1);
  }

  return date;
}

export function parseTimeString(str) {
  const match = str.match(/^(\d{1,2})(?::(\d{2}))?\s*(am|pm)?$/i);
  if (!match) return null;
  let hours = parseInt(match[1]);
  const minutes = parseInt(match[2] || '0');
  const ampm = match[3]?.toLowerCase();
  if (ampm === 'pm' && hours < 12) hours += 12;
  if (ampm === 'am' && hours === 12) hours = 0;
  return { hours, minutes };
}

export function hasExplicitYear(dateStr) {
  return /\b20\d{2}\b/.test(dateStr);
}

function needsYearFix(dateStr, date) {
  if (hasExplicitYear(dateStr)) return false;
  const currentYear = new Date().getFullYear();
  return date.getFullYear() < currentYear || date.getFullYear() > currentYear + 1;
}

function tryAddYear(dateStr, now) {
  const year = now.getFullYear();
  const timeMatch = dateStr.match(/^(.+?)(\d{1,2}(?::\d{2})?\s*(?:am|pm).*)$/i);
  if (timeMatch) {
    return `${timeMatch[1].trim()} ${year} ${timeMatch[2].trim()}`;
  }
  return `${dateStr} ${year}`;
}

export function stripMarkdown(text) {
  if (!text) return text;
  return text
    .replace(/\*\*(.*?)\*\*/g, '$1')
    .replace(/\*(.*?)\*/g, '$1')
    .replace(/\[(.*?)\]\(.*?\)/g, '$1')
    .replace(/#{1,6}\s+/g, '')
    .replace(/`(.*?)`/g, '$1')
    .replace(/~~(.*?)~~/g, '$1')
    .replace(/>\s+/g, '');
}

export function formatDate(isoStr) {
  const d = new Date(isoStr);
  return d.toLocaleDateString('en-US', {
    weekday: 'short', month: 'short', day: 'numeric',
    hour: 'numeric', minute: '2-digit'
  });
}
```

**Step 4: Run tests**

Run: `npx vitest run tests/dates.test.js`
Expected: PASS

**Step 5: Commit**

```bash
git add src/lib/dates.js tests/dates.test.js
git commit -m "feat: extract date parsing module from monolith"
```

---

### Task 1.5: Auth Module (`src/lib/auth.js`)

**Files:**
- Create: `src/lib/auth.js`
- Create: `tests/auth.test.js`

**Step 1: Write failing tests**

```javascript
// tests/auth.test.js
import { describe, it, expect, vi, afterEach } from 'vitest';
import { resolveCredentialsPath, loadConfig } from '../src/lib/auth.js';

describe('resolveCredentialsPath', () => {
  afterEach(() => vi.unstubAllEnvs());

  it('uses PARTIFUL_CREDENTIALS_FILE env var first', () => {
    vi.stubEnv('PARTIFUL_CREDENTIALS_FILE', '/custom/auth.json');
    expect(resolveCredentialsPath()).toBe('/custom/auth.json');
  });

  it('falls back to default path', () => {
    vi.stubEnv('PARTIFUL_CREDENTIALS_FILE', '');
    const result = resolveCredentialsPath();
    expect(result).toMatch(/\.config\/partiful\/auth\.json$/);
  });
});

describe('loadConfig', () => {
  afterEach(() => vi.unstubAllEnvs());

  it('uses PARTIFUL_TOKEN env var as priority 1', () => {
    vi.stubEnv('PARTIFUL_TOKEN', 'my-token');
    const config = loadConfig();
    expect(config.accessToken).toBe('my-token');
    expect(config._source).toBe('env_token');
  });
});
```

**Step 2: Run tests to verify they fail**

Run: `npx vitest run tests/auth.test.js`
Expected: FAIL

**Step 3: Implement auth.js**

```javascript
// src/lib/auth.js
import fs from 'fs';
import path from 'path';
import https from 'https';
import http from 'http';
import crypto from 'crypto';
import { AuthError } from './errors.js';

const DEFAULT_CONFIG_PATH = path.join(process.env.HOME, '.config/partiful/auth.json');
const GOOGLE_TOKEN_URL = 'securetoken.googleapis.com';
const DEFAULT_API_KEY = 'AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k';

export function resolveCredentialsPath() {
  return process.env.PARTIFUL_CREDENTIALS_FILE || DEFAULT_CONFIG_PATH;
}

/**
 * Load config following credential precedence:
 * 1. PARTIFUL_TOKEN env var (raw access token)
 * 2. PARTIFUL_CREDENTIALS_FILE env var → file
 * 3. Default ~/.config/partiful/auth.json
 */
export function loadConfig() {
  // Priority 1: Direct token from env
  if (process.env.PARTIFUL_TOKEN) {
    return {
      accessToken: process.env.PARTIFUL_TOKEN,
      _source: 'env_token',
    };
  }

  // Priority 2/3: Credentials file
  const configPath = resolveCredentialsPath();
  if (!fs.existsSync(configPath)) {
    throw new AuthError(`No auth config found at ${configPath}. Run: partiful auth login`);
  }

  try {
    const config = JSON.parse(fs.readFileSync(configPath, 'utf8'));
    config._source = 'file';
    config._configPath = configPath;
    return config;
  } catch (e) {
    throw new AuthError(`Failed to parse auth config at ${configPath}: ${e.message}`);
  }
}

export function saveConfig(config) {
  const configPath = config._configPath || resolveCredentialsPath();
  const toSave = { ...config };
  delete toSave._source;
  delete toSave._configPath;
  const dir = path.dirname(configPath);
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(configPath, JSON.stringify(toSave, null, 2));
}

export async function refreshAccessToken(config) {
  const apiKey = config.apiKey || DEFAULT_API_KEY;
  return new Promise((resolve, reject) => {
    const postData = `grant_type=refresh_token&refresh_token=${config.refreshToken}`;
    const options = {
      hostname: GOOGLE_TOKEN_URL,
      port: 443,
      path: `/v1/token?key=${apiKey}`,
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Content-Length': Buffer.byteLength(postData),
        'Referer': 'https://partiful.com/',
      },
    };

    const req = https.request(options, (res) => {
      let data = '';
      res.on('data', (chunk) => (data += chunk));
      res.on('end', () => {
        try {
          const result = JSON.parse(data);
          if (result.error) reject(new AuthError(result.error.message || 'Token refresh failed'));
          else resolve(result);
        } catch (e) {
          reject(new AuthError(`Token refresh response parse error: ${e.message}`));
        }
      });
    });
    req.on('error', (e) => reject(new AuthError(`Token refresh network error: ${e.message}`)));
    req.write(postData);
    req.end();
  });
}

export async function getValidToken(config) {
  // If token came from env, use it directly (no refresh)
  if (config._source === 'env_token') return config.accessToken;

  // Check cached token expiry
  if (config.accessToken && config.tokenExpiry) {
    if (Date.now() < config.tokenExpiry - 60000) return config.accessToken;
  }

  if (!config.refreshToken) {
    throw new AuthError('No refresh token available. Run: partiful auth login');
  }

  console.error('Refreshing access token...');
  const result = await refreshAccessToken(config);

  config.accessToken = result.id_token;
  config.tokenExpiry = Date.now() + parseInt(result.expires_in) * 1000;
  if (result.refresh_token) config.refreshToken = result.refresh_token;

  saveConfig(config);
  return config.accessToken;
}

export function generateAmplitudeDeviceId() {
  return crypto.randomBytes(12).toString('base64').replace(/[+/=]/g, '');
}

/**
 * Build the standard Partiful API payload wrapper.
 */
export function wrapPayload(config, params = {}) {
  return {
    data: {
      params,
      amplitudeDeviceId: config.amplitudeDeviceId || generateAmplitudeDeviceId(),
      amplitudeSessionId: Date.now(),
      userId: config.userId,
    },
  };
}

// Re-export for auth login flow
export { http, https, DEFAULT_CONFIG_PATH };
```

**Step 4: Run tests**

Run: `npx vitest run tests/auth.test.js`
Expected: PASS

**Step 5: Commit**

```bash
git add src/lib/auth.js tests/auth.test.js
git commit -m "feat: add auth module with credential precedence chain"
```

---

### Task 1.6: HTTP Module with Retry (`src/lib/http.js`)

**Files:**
- Create: `src/lib/http.js`
- Create: `tests/http.test.js`

**Step 1: Write failing tests**

```javascript
// tests/http.test.js
import { describe, it, expect, vi, afterEach } from 'vitest';
import { apiRequest, firestoreRequest, firestoreListDocuments } from '../src/lib/http.js';

// We'll test retry logic by mocking https.request
describe('apiRequest', () => {
  it('exports apiRequest function', () => {
    expect(typeof apiRequest).toBe('function');
  });

  it('exports firestoreRequest function', () => {
    expect(typeof firestoreRequest).toBe('function');
  });

  it('exports firestoreListDocuments function', () => {
    expect(typeof firestoreListDocuments).toBe('function');
  });
});
```

**Step 2: Run tests to verify they fail**

Run: `npx vitest run tests/http.test.js`
Expected: FAIL

**Step 3: Implement http.js**

```javascript
// src/lib/http.js
import https from 'https';
import { ApiError, AuthError, NotFoundError } from './errors.js';

const API_BASE = 'api.partiful.com';
const FIRESTORE_BASE = 'firestore.googleapis.com';
const FIRESTORE_PROJECT = 'getpartiful';

const MAX_RETRIES = parseInt(process.env.PARTIFUL_MAX_RETRIES || '3');
const TIMEOUT = parseInt(process.env.PARTIFUL_TIMEOUT || '30') * 1000;

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

function retryDelay(attempt, retryAfterHeader) {
  if (retryAfterHeader) {
    const seconds = parseInt(retryAfterHeader);
    if (!isNaN(seconds)) return seconds * 1000;
    // Try HTTP-date format
    const date = new Date(retryAfterHeader);
    if (!isNaN(date.getTime())) return Math.max(0, date.getTime() - Date.now());
  }
  // Exponential backoff with jitter: min(30s, 2^attempt + random(0,1))
  return Math.min(30000, Math.pow(2, attempt) * 1000 + Math.random() * 1000);
}

function isRetryable(statusCode) {
  return statusCode === 429 || statusCode >= 500;
}

function classifyHttpError(statusCode, body, endpoint) {
  const msg = typeof body === 'string' ? body.slice(0, 200) : JSON.stringify(body).slice(0, 200);
  if (statusCode === 401 || statusCode === 403) {
    throw new AuthError(`${endpoint}: ${statusCode} ${msg}`);
  }
  if (statusCode === 404) {
    throw new NotFoundError(`${endpoint}: Not found`);
  }
  throw new ApiError(`${endpoint}: ${statusCode} ${msg}`, { statusCode, endpoint });
}

function makeRequest(hostname, options, bodyStr, verbose) {
  return new Promise((resolve, reject) => {
    if (verbose) console.error(`${options.method} ${hostname}${options.path} (attempt)`);

    const req = https.request({ hostname, ...options, timeout: TIMEOUT }, (res) => {
      let data = '';
      res.on('data', (chunk) => (data += chunk));
      res.on('end', () => {
        if (verbose) console.error(`→ ${res.statusCode} (${data.length} bytes)`);
        resolve({ statusCode: res.statusCode, data, headers: res.headers });
      });
    });
    req.on('timeout', () => { req.destroy(); reject(new ApiError('Request timed out')); });
    req.on('error', reject);
    if (bodyStr) req.write(bodyStr);
    req.end();
  });
}

async function requestWithRetry(hostname, options, bodyStr, verbose) {
  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    const result = await makeRequest(hostname, options, bodyStr, verbose);

    if (result.statusCode >= 200 && result.statusCode < 300) {
      try {
        return { data: result.data ? JSON.parse(result.data) : {}, statusCode: result.statusCode };
      } catch {
        return { data: { _raw: result.data }, statusCode: result.statusCode };
      }
    }

    if (isRetryable(result.statusCode) && attempt < MAX_RETRIES) {
      const delay = retryDelay(attempt, result.headers['retry-after']);
      console.error(`Retrying in ${Math.round(delay / 1000)}s (${result.statusCode}, attempt ${attempt + 1}/${MAX_RETRIES})...`);
      await sleep(delay);
      continue;
    }

    // Non-retryable or exhausted retries
    let parsed;
    try { parsed = JSON.parse(result.data); } catch { parsed = result.data; }
    classifyHttpError(result.statusCode, parsed, options.path);
  }
}

export async function apiRequest(method, endpoint, token, body = null, verbose = false) {
  const bodyStr = body ? JSON.stringify(body) : null;
  const options = {
    port: 443,
    path: endpoint,
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
      Accept: 'application/json, text/plain, */*',
      Origin: 'https://partiful.com',
      Referer: 'https://partiful.com/',
      ...(bodyStr ? { 'Content-Length': Buffer.byteLength(bodyStr) } : {}),
    },
  };

  return requestWithRetry(API_BASE, options, bodyStr, verbose);
}

export async function firestoreRequest(method, eventId, body, token, updateFields = [], verbose = false) {
  let fpath = `/v1/projects/${FIRESTORE_PROJECT}/databases/(default)/documents/events/${eventId}`;
  if (method === 'PATCH' && updateFields.length > 0) {
    fpath += '?' + updateFields.map((f) => `updateMask.fieldPaths=${f}`).join('&');
  }

  const bodyStr = body ? JSON.stringify(body) : null;
  const options = {
    port: 443,
    path: fpath,
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
      Referer: 'https://partiful.com/',
      ...(bodyStr ? { 'Content-Length': Buffer.byteLength(bodyStr) } : {}),
    },
  };

  return requestWithRetry(FIRESTORE_BASE, options, bodyStr, verbose);
}

export async function firestoreListDocuments(collectionPath, token, pageSize = 100, pageToken = null, verbose = false) {
  let fpath = `/v1/projects/${FIRESTORE_PROJECT}/databases/(default)/documents/${collectionPath}?pageSize=${pageSize}`;
  if (pageToken) fpath += `&pageToken=${encodeURIComponent(pageToken)}`;

  const options = {
    port: 443,
    path: fpath,
    method: 'GET',
    headers: {
      Authorization: `Bearer ${token}`,
      Referer: 'https://partiful.com/',
    },
  };

  return requestWithRetry(FIRESTORE_BASE, options, null, verbose);
}
```

**Step 4: Run tests**

Run: `npx vitest run tests/http.test.js`
Expected: PASS

**Step 5: Commit**

```bash
git add src/lib/http.js tests/http.test.js
git commit -m "feat: add HTTP module with exponential backoff retry on 429/5xx"
```

---

## Phase 2: Commands

### Task 2.1: Auth Commands (`src/commands/auth.js`)

**Files:**
- Create: `src/commands/auth.js`
- Modify: `src/cli.js` — wire auth subcommand

**Step 1: Implement auth.js commands**

```javascript
// src/commands/auth.js
import readline from 'readline';
import http from 'http';
import { jsonOutput, jsonError, EXIT } from '../lib/output.js';
import { loadConfig, getValidToken, saveConfig, resolveCredentialsPath, generateAmplitudeDeviceId } from '../lib/auth.js';
import { AuthError } from '../lib/errors.js';
import fs from 'fs';
import path from 'path';

export function registerAuthCommands(program) {
  const auth = program.command('auth').description('Manage authentication');

  auth
    .command('status')
    .description('Check authentication status')
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        let tokenValid = false;
        let expiresIn = null;

        try {
          await getValidToken(config);
          tokenValid = true;
          if (config.tokenExpiry) {
            expiresIn = Math.round((config.tokenExpiry - Date.now()) / 1000);
          }
        } catch {}

        jsonOutput({
          user: config.displayName || null,
          phone: config.phoneNumber || null,
          userId: config.userId || null,
          tokenValid,
          expiresIn,
          source: config._source,
        }, {}, globalOpts);
      } catch (e) {
        if (e instanceof AuthError) {
          jsonError(e.message, e.exitCode, e.type);
        }
        throw e;
      }
    });

  auth
    .command('login')
    .description('Set up authentication via bookmarklet')
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      const configPath = resolveCredentialsPath();
      const configDir = path.dirname(configPath);
      if (!fs.existsSync(configDir)) fs.mkdirSync(configDir, { recursive: true });

      const PORT = 9876;
      const extractorCode = `(async function(){try{const dbReq=indexedDB.open('firebaseLocalStorageDb');dbReq.onsuccess=function(e){const db=e.target.result;const tx=db.transaction('firebaseLocalStorage','readonly');const store=tx.objectStore('firebaseLocalStorage');const getReq=store.getAll();getReq.onsuccess=function(){const items=getReq.result;const authItem=items.find(i=>i.fbase_key&&i.fbase_key.includes('firebase:authUser'));if(!authItem||!authItem.value){alert('No auth found. Make sure you are logged into Partiful.');return;}const v=authItem.value;const data={apiKey:v.apiKey,refreshToken:v.stsTokenManager?.refreshToken,userId:v.uid,displayName:v.displayName,phoneNumber:v.phoneNumber};if(!data.refreshToken){alert('No refresh token found. Try logging out and back in.');return;}fetch('http://localhost:${PORT}/auth',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data)}).then(r=>r.ok?alert('Auth saved! You can close this tab.'):alert('Failed to save auth')).catch(()=>alert('Could not connect to CLI. Is it running?'));};};dbReq.onerror=()=>alert('Could not open IndexedDB');}catch(e){alert('Error: '+e.message);}})();`;
      const bookmarklet = 'javascript:' + encodeURIComponent(extractorCode);

      console.error(`\nPartiful CLI Auth Setup\n`);
      console.error(`1. Open https://partiful.com and log in`);
      console.error(`2. Create a bookmarklet with this URL:`);
      console.error(`\n${bookmarklet}\n`);
      console.error(`3. Click the bookmarklet while on partiful.com`);
      console.error(`\nWaiting for auth on http://localhost:${PORT}... (Ctrl+C to cancel)\n`);

      return new Promise((resolve, reject) => {
        const server = http.createServer((req, res) => {
          res.setHeader('Access-Control-Allow-Origin', '*');
          res.setHeader('Access-Control-Allow-Methods', 'POST, OPTIONS');
          res.setHeader('Access-Control-Allow-Headers', 'Content-Type');

          if (req.method === 'OPTIONS') { res.writeHead(204); res.end(); return; }

          if (req.method === 'POST' && req.url === '/auth') {
            let body = '';
            req.on('data', (chunk) => (body += chunk));
            req.on('end', () => {
              try {
                const data = JSON.parse(body);
                if (!data.refreshToken || !data.userId) {
                  res.writeHead(400); res.end('Missing fields'); return;
                }
                const config = {
                  apiKey: data.apiKey || 'AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k',
                  refreshToken: data.refreshToken,
                  userId: data.userId,
                  displayName: data.displayName || 'Unknown',
                  phoneNumber: data.phoneNumber || 'Unknown',
                };
                fs.writeFileSync(configPath, JSON.stringify(config, null, 2));
                res.writeHead(200); res.end('OK');
                console.error(`Auth saved to ${configPath}`);
                jsonOutput({ user: config.displayName, configPath }, {}, globalOpts);
                server.close();
                resolve();
              } catch {
                res.writeHead(400); res.end('Invalid JSON');
              }
            });
          } else {
            res.writeHead(404); res.end('Not found');
          }
        });
        server.on('error', (e) => {
          if (e.code === 'EADDRINUSE') jsonError(`Port ${PORT} in use`, EXIT.INTERNAL_ERROR, 'internal_error');
          else jsonError(e.message, EXIT.INTERNAL_ERROR, 'internal_error');
        });
        server.listen(PORT);
      });
    });

  auth
    .command('logout')
    .description('Remove stored credentials')
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      const configPath = resolveCredentialsPath();
      if (fs.existsSync(configPath)) {
        fs.unlinkSync(configPath);
        jsonOutput({ removed: configPath }, {}, globalOpts);
      } else {
        jsonOutput({ removed: null, message: 'Already logged out' }, {}, globalOpts);
      }
    });
}
```

**Step 2: Wire into cli.js**

Add to `src/cli.js`:
```javascript
import { registerAuthCommands } from './commands/auth.js';

// Inside run(), after program definition:
registerAuthCommands(program);
```

**Step 3: Verify**

Run: `node bin/partiful auth status --help`
Expected: Shows auth status help

**Step 4: Commit**

```bash
git add src/commands/auth.js src/cli.js
git commit -m "feat: add auth commands (login, logout, status) with JSON output"
```

---

### Task 2.2: Events Commands (`src/commands/events.js`)

**Files:**
- Create: `src/commands/events.js`
- Create: `tests/events.test.js`
- Modify: `src/cli.js` — wire events subcommand

**Step 1: Implement events.js**

This is the largest command file. Migrate `listEvents`, `getEvent`, `createEvent`, `updateEvent`, `cancelEvent` from the old monolith. Each command function:
- Uses `loadConfig()` + `getValidToken()`
- Uses `apiRequest()` / `firestoreRequest()` from `src/lib/http.js`
- Uses `wrapPayload()` from `src/lib/auth.js`
- Outputs via `jsonOutput()` / `jsonError()`
- Respects `--dry-run`, `--yes`, `--verbose` global flags
- Uses `parseDateTime()` from `src/lib/dates.js`

```javascript
// src/commands/events.js
import { jsonOutput, jsonError, formatTable, EXIT } from '../lib/output.js';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest, firestoreRequest } from '../lib/http.js';
import { parseDateTime, stripMarkdown, formatDate } from '../lib/dates.js';
import { ValidationError, PartifulError } from '../lib/errors.js';
import readline from 'readline';

async function confirm(question) {
  const rl = readline.createInterface({ input: process.stdin, output: process.stderr });
  return new Promise((r) => {
    rl.question(question + ' [y/N]: ', (answer) => {
      rl.close();
      r(answer.toLowerCase() === 'y' || answer.toLowerCase() === 'yes');
    });
  });
}

export function registerEventsCommands(program) {
  const events = program.command('events').description('Manage Partiful events');

  // ── list ──
  events
    .command('list')
    .description('List upcoming or past events')
    .option('--past', 'Show past events')
    .option('--include-cancelled', 'Include cancelled events')
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);
        const endpoint = opts.past ? '/getMyPastEventsForHomePage' : '/getMyUpcomingEventsForHomePage';
        const payload = wrapPayload(config);

        if (globalOpts.dryRun) {
          console.error('[dry-run] Would POST', endpoint);
          console.error(JSON.stringify(payload, null, 2));
          process.exit(0);
        }

        const result = await apiRequest('POST', endpoint, token, payload, globalOpts.verbose);
        let eventList = opts.past
          ? result.data.result?.data?.pastEvents
          : result.data.result?.data?.upcomingEvents;

        if (!opts.includeCancelled && eventList) {
          eventList = eventList.filter((e) => e.status !== 'CANCELED');
        }

        const events = (eventList || []).map((e) => ({
          id: e.id,
          title: e.title,
          startDate: e.startDate,
          endDate: e.endDate || null,
          location: e.location || null,
          status: e.status || 'ACTIVE',
          role: e.ownerIds?.includes(config.userId) ? 'hosting' : 'invited',
          guestCounts: {
            going: e.guestStatusCounts?.GOING || 0,
            maybe: e.guestStatusCounts?.MAYBE || 0,
            invited: e.guestStatusCounts?.SENT || 0,
            declined: e.guestStatusCounts?.DECLINED || 0,
          },
          url: `https://partiful.com/e/${e.id}`,
        }));

        jsonOutput({ events }, { count: events.length }, globalOpts);
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type);
        jsonError(e.message, EXIT.INTERNAL_ERROR, 'internal_error');
      }
    });

  // ── get ──
  events
    .command('get <eventId>')
    .description('Get event details')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);
        const payload = wrapPayload(config, { eventId });

        if (globalOpts.dryRun) {
          console.error('[dry-run] Would POST /getEvent');
          console.error(JSON.stringify(payload, null, 2));
          process.exit(0);
        }

        const result = await apiRequest('POST', '/getEvent', token, payload, globalOpts.verbose);
        const event = result.data.result?.data?.event;

        if (!event) jsonError(`Event ${eventId} not found`, EXIT.NOT_FOUND, 'not_found');

        jsonOutput({
          id: eventId,
          title: event.title,
          startDate: event.startDate,
          endDate: event.endDate || null,
          location: event.location || null,
          address: event.address || null,
          description: event.description || null,
          status: event.status || 'ACTIVE',
          visibility: event.visibility || 'public',
          guestCounts: {
            going: event.guestStatusCounts?.GOING || 0,
            maybe: event.guestStatusCounts?.MAYBE || 0,
            invited: event.guestStatusCounts?.SENT || 0,
            declined: event.guestStatusCounts?.DECLINED || 0,
          },
          url: `https://partiful.com/e/${eventId}`,
        }, {}, globalOpts);
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type);
        jsonError(e.message, EXIT.INTERNAL_ERROR, 'internal_error');
      }
    });

  // ── create ──
  events
    .command('create')
    .description('Create a new event')
    .requiredOption('--title <title>', 'Event title')
    .requiredOption('--date <date>', 'Start date/time')
    .option('--end-date <endDate>', 'End date/time')
    .option('--location <location>', 'Venue name')
    .option('--address <address>', 'Street address')
    .option('--description <desc>', 'Event description')
    .option('--capacity <n>', 'Guest limit', parseInt)
    .option('--no-waitlist', 'Disable waitlist')
    .option('--private', 'Make event private')
    .option('--timezone <tz>', 'Timezone', 'America/Los_Angeles')
    .option('--theme <theme>', 'Color theme', 'oxblood')
    .option('--effect <effect>', 'Visual effect', 'sunbeams')
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const startDate = parseDateTime(opts.date, opts.timezone);
        const endDate = opts.endDate ? parseDateTime(opts.endDate, opts.timezone) : null;

        const eventData = {
          title: opts.title,
          startDate: startDate.toISOString(),
          endDate: endDate ? endDate.toISOString() : null,
          timezone: opts.timezone,
          location: opts.location || null,
          address: opts.address || null,
          description: opts.description ? stripMarkdown(opts.description) : null,
          guestStatusCounts: {
            READY_TO_SEND: 0, SENDING: 0, SENT: 0, SEND_ERROR: 0,
            DELIVERY_ERROR: 0, INTERESTED: 0, MAYBE: 0, GOING: 0,
            DECLINED: 0, WAITLIST: 0, PENDING_APPROVAL: 0, APPROVED: 0,
            WITHDRAWN: 0, RESPONDED_TO_FIND_A_TIME: 0,
            WAITLISTED_FOR_APPROVAL: 0, REJECTED: 0,
          },
          displaySettings: { theme: opts.theme, effect: opts.effect, titleFont: 'display' },
          showHostList: true, showGuestCount: true, showGuestList: true,
          showActivityTimestamps: true, displayInviteButton: true,
          visibility: opts.private ? 'private' : 'public',
          allowGuestPhotoUpload: true, enableGuestReminders: true,
          rsvpsEnabled: true, allowGuestsToInviteMutuals: true,
          rsvpButtonGlyphType: 'emojis', status: 'UNSAVED',
        };

        if (opts.capacity) {
          eventData.guestLimit = opts.capacity;
          eventData.enableWaitlist = opts.waitlist !== false;
        }

        // Remove null values
        Object.keys(eventData).forEach((k) => { if (eventData[k] === null) delete eventData[k]; });

        const payload = wrapPayload(config, { event: eventData, cohostIds: [] });

        if (globalOpts.dryRun) {
          console.error('[dry-run] Would POST /createEvent');
          jsonOutput({ dryRun: true, payload: payload.data.params }, {}, globalOpts);
          process.exit(0);
        }

        console.error(`Creating event: ${opts.title}`);
        const result = await apiRequest('POST', '/createEvent', token, payload, globalOpts.verbose);
        const eventId = result.data.result?.data || result.data.result?.eventId;

        jsonOutput({
          id: eventId,
          title: opts.title,
          url: `https://partiful.com/e/${eventId}`,
        }, {}, globalOpts);
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type);
        jsonError(e.message, EXIT.INTERNAL_ERROR, 'internal_error');
      }
    });

  // ── update ──
  events
    .command('update <eventId>')
    .description('Update an existing event')
    .option('--title <title>', 'New title')
    .option('--date <date>', 'New start date')
    .option('--end-date <endDate>', 'New end date')
    .option('--location <location>', 'New location')
    .option('--description <desc>', 'New description')
    .option('--capacity <n>', 'New guest limit', parseInt)
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const fields = {};
        const updateFields = [];

        if (opts.title) { fields.title = { stringValue: opts.title }; updateFields.push('title'); }
        if (opts.location) { fields.location = { stringValue: opts.location }; updateFields.push('location'); }
        if (opts.description) { fields.description = { stringValue: stripMarkdown(opts.description) }; updateFields.push('description'); }
        if (opts.date) { fields.startDate = { timestampValue: parseDateTime(opts.date).toISOString() }; updateFields.push('startDate'); }
        if (opts.endDate) { fields.endDate = { timestampValue: parseDateTime(opts.endDate).toISOString() }; updateFields.push('endDate'); }
        if (opts.capacity) { fields.guestLimit = { integerValue: String(opts.capacity) }; updateFields.push('guestLimit'); }

        if (updateFields.length === 0) {
          jsonError('No fields to update. Use --title, --location, --description, --date, --end-date, or --capacity', EXIT.VALIDATION_ERROR, 'validation_error');
        }

        if (globalOpts.dryRun) {
          console.error(`[dry-run] Would PATCH event ${eventId}`);
          jsonOutput({ dryRun: true, fields: updateFields, payload: fields }, {}, globalOpts);
          process.exit(0);
        }

        console.error(`Updating event ${eventId}: ${updateFields.join(', ')}`);
        await firestoreRequest('PATCH', eventId, { fields }, token, updateFields, globalOpts.verbose);

        jsonOutput({
          id: eventId,
          updated: updateFields,
          url: `https://partiful.com/e/${eventId}`,
        }, {}, globalOpts);
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type);
        jsonError(e.message, EXIT.INTERNAL_ERROR, 'internal_error');
      }
    });

  // ── cancel ──
  events
    .command('cancel <eventId>')
    .description('Cancel an event')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);
        const payload = wrapPayload(config, { eventId });

        if (globalOpts.dryRun) {
          console.error(`[dry-run] Would cancel event ${eventId}`);
          jsonOutput({ dryRun: true, eventId }, {}, globalOpts);
          process.exit(0);
        }

        if (!globalOpts.yes && !globalOpts.force) {
          // Fetch event details for confirmation
          const getResult = await apiRequest('POST', '/getEvent', token, wrapPayload(config, { eventId }), globalOpts.verbose);
          const event = getResult.data.result?.data?.event;
          if (event) {
            const going = event.guestStatusCounts?.GOING || 0;
            const maybe = event.guestStatusCounts?.MAYBE || 0;
            console.error(`\nAbout to cancel: ${event.title}`);
            console.error(`  Date: ${formatDate(event.startDate)}`);
            console.error(`  Guests: ${going} going, ${maybe} maybe\n`);
          }
          const confirmed = await confirm('Are you sure? This cannot be undone.');
          if (!confirmed) {
            jsonOutput({ cancelled: false, reason: 'User aborted' }, {}, globalOpts);
            process.exit(0);
          }
        }

        console.error(`Cancelling event ${eventId}...`);
        await apiRequest('POST', '/cancelEvent', token, payload, globalOpts.verbose);
        jsonOutput({ id: eventId, cancelled: true }, {}, globalOpts);
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type);
        jsonError(e.message, EXIT.INTERNAL_ERROR, 'internal_error');
      }
    });
}
```

**Step 2: Wire into cli.js**

```javascript
import { registerEventsCommands } from './commands/events.js';
// Inside run():
registerEventsCommands(program);
```

**Step 3: Verify**

Run: `node bin/partiful events --help`
Expected: Shows events subcommands (list, get, create, update, cancel)

**Step 4: Commit**

```bash
git add src/commands/events.js src/cli.js
git commit -m "feat: add events commands (list, get, create, update, cancel) with JSON output"
```

---

### Task 2.3: Guests Commands (`src/commands/guests.js`)

**Files:**
- Create: `src/commands/guests.js`
- Modify: `src/cli.js`

**Step 1: Implement guests.js**

Migrate `showGuests`, `inviteToEvent` logic. Use `firestoreListDocuments` for pagination. Output JSON envelopes.

```javascript
// src/commands/guests.js
import { jsonOutput, jsonError, EXIT } from '../lib/output.js';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest, firestoreListDocuments } from '../lib/http.js';
import { PartifulError } from '../lib/errors.js';

async function fetchGuests(eventId, config, token, verbose) {
  // Get event title + counts from API
  const payload = wrapPayload(config, { eventId });
  let event = null;
  let counts = {};

  try {
    const eventResult = await apiRequest('POST', '/getEvent', token, payload, verbose);
    event = eventResult.data.result?.data?.event;
    if (event) counts = event.guestStatusCounts || {};
  } catch {
    console.error('Warning: Could not fetch event metadata, falling back to Firestore');
  }

  // Fetch guests from Firestore (auto-paginate)
  const guests = [];
  let pageToken = null;
  do {
    const result = await firestoreListDocuments(`events/${eventId}/guests`, token, 100, pageToken, verbose);
    if (result.data.documents) {
      for (const doc of result.data.documents) {
        const f = doc.fields || {};
        guests.push({
          name: f.name?.stringValue || 'Unknown',
          status: f.status?.stringValue || 'UNKNOWN',
          createdAt: f.createdAt?.timestampValue || null,
          inviteDate: f.inviteDate?.timestampValue || null,
          count: parseInt(f.count?.integerValue || '1'),
          channel: f.inviteMetadata?.mapValue?.fields?.channel?.stringValue || null,
        });
      }
    }
    pageToken = result.data.nextPageToken || null;
  } while (pageToken);

  // Compute counts from guest list if API didn't return them
  if (Object.keys(counts).length === 0) {
    for (const g of guests) {
      counts[g.status] = (counts[g.status] || 0) + 1;
    }
  }

  return {
    eventId,
    eventTitle: event?.title || 'Unknown Event',
    counts: {
      going: counts.GOING || 0,
      maybe: counts.MAYBE || 0,
      invited: counts.SENT || 0,
      declined: counts.DECLINED || 0,
      waitlist: counts.WAITLIST || 0,
    },
    total: guests.length,
    guests,
  };
}

export function registerGuestsCommands(program) {
  const guestsCmd = program.command('guests').description('Manage event guests');

  // ── list ──
  guestsCmd
    .command('list <eventId>')
    .description('List guests for an event')
    .option('--status <status>', 'Filter by status (going, maybe, declined, invited)')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        if (globalOpts.dryRun) {
          console.error(`[dry-run] Would fetch guests for event ${eventId}`);
          process.exit(0);
        }

        const summary = await fetchGuests(eventId, config, token, globalOpts.verbose);

        // Filter by status if requested
        if (opts.status) {
          const statusUpper = opts.status.toUpperCase();
          summary.guests = summary.guests.filter((g) => g.status === statusUpper);
        }

        jsonOutput(summary, { count: summary.guests.length }, globalOpts);
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type);
        jsonError(e.message, EXIT.INTERNAL_ERROR, 'internal_error');
      }
    });

  // ── invite ──
  guestsCmd
    .command('invite <eventId>')
    .description('Send invites to an event')
    .option('--phone <number...>', 'Phone numbers to invite')
    .option('--user-id <id...>', 'Partiful user IDs to invite')
    .option('--message <msg>', 'Custom invitation message')
    .action(async (eventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        const phones = opts.phone || [];
        const userIds = opts.userId || [];

        if (phones.length === 0 && userIds.length === 0) {
          jsonError('Provide --phone or --user-id to invite', EXIT.VALIDATION_ERROR, 'validation_error');
        }

        const params = {
          eventId,
          userIdsToInvite: userIds,
          phoneContactsToInvite: phones.map((p) => ({
            phoneNumber: p.replace(/[^+\d]/g, ''),
            firstName: '',
            lastName: '',
          })),
          invitationMessage: opts.message || '',
          otherMutualsCount: 0,
        };

        const payload = wrapPayload(config, params);

        if (globalOpts.dryRun) {
          console.error('[dry-run] Would POST /addInvitedGuestsAsHost');
          jsonOutput({ dryRun: true, payload: params }, {}, globalOpts);
          process.exit(0);
        }

        console.error(`Sending invites to event ${eventId}...`);
        await apiRequest('POST', '/addInvitedGuestsAsHost', token, payload, globalOpts.verbose);

        const invited = userIds.length + phones.length;
        jsonOutput({
          eventId,
          invited,
          url: `https://partiful.com/e/${eventId}`,
        }, {}, globalOpts);
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type);
        jsonError(e.message, EXIT.INTERNAL_ERROR, 'internal_error');
      }
    });
}

// Export for helpers
export { fetchGuests };
```

**Step 2: Wire into cli.js**

```javascript
import { registerGuestsCommands } from './commands/guests.js';
registerGuestsCommands(program);
```

**Step 3: Commit**

```bash
git add src/commands/guests.js src/cli.js
git commit -m "feat: add guests commands (list, invite) with Firestore pagination"
```

---

### Task 2.4: Contacts & Blasts Commands

**Files:**
- Create: `src/commands/contacts.js`
- Create: `src/commands/blasts.js`
- Modify: `src/cli.js`

**Step 1: Implement contacts.js**

```javascript
// src/commands/contacts.js
import { jsonOutput, jsonError, EXIT } from '../lib/output.js';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { PartifulError } from '../lib/errors.js';

export function registerContactsCommands(program) {
  const contacts = program.command('contacts').description('Manage Partiful contacts');

  contacts
    .command('list [query]')
    .description('List or search contacts')
    .option('--limit <n>', 'Max results', parseInt, 20)
    .action(async (query, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);
        const payload = wrapPayload(config);

        if (globalOpts.dryRun) {
          console.error('[dry-run] Would POST /getContacts');
          process.exit(0);
        }

        const result = await apiRequest('POST', '/getContacts', token, payload, globalOpts.verbose);
        let contactList = result.data.result?.data || [];

        if (query) {
          const q = query.toLowerCase();
          contactList = contactList.filter((c) => (c.name || '').toLowerCase().includes(q));
        }

        const limited = contactList.slice(0, opts.limit);

        jsonOutput(
          { contacts: limited },
          { count: limited.length, total: contactList.length, query: query || null },
          globalOpts
        );
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type);
        jsonError(e.message, EXIT.INTERNAL_ERROR, 'internal_error');
      }
    });
}
```

**Step 2: Implement blasts.js**

```javascript
// src/commands/blasts.js
import { jsonError, EXIT } from '../lib/output.js';

export function registerBlastsCommands(program) {
  const blasts = program.command('blasts').description('Event text blasts');

  blasts
    .command('send <eventId>')
    .description('Send a text blast (opens web UI)')
    .option('--message <msg>', 'Message to send')
    .action(async (eventId, opts, cmd) => {
      jsonError(
        `Text blasts require Firestore SDK. Use web UI: https://partiful.com/e/${eventId}`,
        EXIT.INTERNAL_ERROR,
        'not_implemented',
        { workaround: `https://partiful.com/e/${eventId}`, message: opts.message || null }
      );
    });
}
```

**Step 3: Wire both into cli.js and commit**

```bash
git add src/commands/contacts.js src/commands/blasts.js src/cli.js
git commit -m "feat: add contacts list and blasts send (stub) commands"
```

---

## Phase 3: Helpers

### Task 3.1: Clone Helper (`src/helpers/clone.js`)

**Files:**
- Create: `src/helpers/clone.js`
- Modify: `src/commands/events.js` — register +clone

**Step 1: Implement clone.js**

Migrate clone logic from monolith. Uses `events get` → build create options → `events create`. Preserves duration. Handles `--reinvite`.

```javascript
// src/helpers/clone.js
import { jsonOutput, jsonError, EXIT } from '../lib/output.js';
import { loadConfig, getValidToken, wrapPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { parseDateTime, stripMarkdown } from '../lib/dates.js';
import { PartifulError } from '../lib/errors.js';
import { fetchGuests } from '../commands/guests.js';

export function registerCloneHelper(eventsCommand) {
  eventsCommand
    .command('+clone <sourceEventId>')
    .description('Clone an event with a new date')
    .requiredOption('--date <date>', 'New start date/time')
    .option('--end-date <endDate>', 'New end date')
    .option('--title <title>', 'Override title')
    .option('--location <location>', 'Override location')
    .option('--address <address>', 'Override address')
    .option('--description <desc>', 'Override description')
    .option('--capacity <n>', 'Override capacity', parseInt)
    .option('--private', 'Make private')
    .option('--timezone <tz>', 'Timezone', 'America/Los_Angeles')
    .option('--theme <theme>', 'Override theme')
    .option('--reinvite [status]', 'List guests to reinvite (optionally filter by status)')
    .action(async (sourceEventId, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const config = loadConfig();
        const token = await getValidToken(config);

        // Fetch source event
        console.error(`Fetching source event ${sourceEventId}...`);
        const getPayload = wrapPayload(config, { eventId: sourceEventId });
        const result = await apiRequest('POST', '/getEvent', token, getPayload, globalOpts.verbose);
        const source = result.data.result?.data?.event;

        if (!source) jsonError(`Source event not found: ${sourceEventId}`, EXIT.NOT_FOUND, 'not_found');

        // Build create payload from source + overrides
        const startDate = parseDateTime(opts.date, opts.timezone);
        let endDate = opts.endDate ? parseDateTime(opts.endDate, opts.timezone) : null;

        // Preserve duration if source has end date
        if (!endDate && source.endDate && source.startDate) {
          const srcDuration = new Date(source.endDate) - new Date(source.startDate);
          if (srcDuration > 0) endDate = new Date(startDate.getTime() + srcDuration);
        }

        const eventData = {
          title: opts.title || source.title,
          startDate: startDate.toISOString(),
          endDate: endDate ? endDate.toISOString() : null,
          timezone: opts.timezone || source.timezone || 'America/Los_Angeles',
          location: opts.location || source.location || null,
          address: opts.address || source.address || null,
          description: opts.description ? stripMarkdown(opts.description) : source.description || null,
          displaySettings: {
            theme: opts.theme || source.displaySettings?.theme || 'oxblood',
            effect: source.displaySettings?.effect || 'sunbeams',
            titleFont: 'display',
          },
          visibility: opts.private ? 'private' : source.visibility || 'public',
          guestStatusCounts: {
            READY_TO_SEND: 0, SENDING: 0, SENT: 0, SEND_ERROR: 0,
            DELIVERY_ERROR: 0, INTERESTED: 0, MAYBE: 0, GOING: 0,
            DECLINED: 0, WAITLIST: 0, PENDING_APPROVAL: 0, APPROVED: 0,
            WITHDRAWN: 0, RESPONDED_TO_FIND_A_TIME: 0,
            WAITLISTED_FOR_APPROVAL: 0, REJECTED: 0,
          },
          showHostList: true, showGuestCount: true, showGuestList: true,
          showActivityTimestamps: true, displayInviteButton: true,
          allowGuestPhotoUpload: true, enableGuestReminders: true,
          rsvpsEnabled: true, allowGuestsToInviteMutuals: true,
          rsvpButtonGlyphType: 'emojis', status: 'UNSAVED',
        };

        if (opts.capacity || source.guestLimit) {
          eventData.guestLimit = opts.capacity || source.guestLimit;
          eventData.enableWaitlist = source.enableWaitlist !== false;
        }

        Object.keys(eventData).forEach((k) => { if (eventData[k] === null) delete eventData[k]; });

        const createPayload = wrapPayload(config, { event: eventData, cohostIds: [] });

        if (globalOpts.dryRun) {
          console.error(`[dry-run] Would clone ${source.title} → ${eventData.title}`);
          jsonOutput({ dryRun: true, source: sourceEventId, payload: eventData }, {}, globalOpts);
          process.exit(0);
        }

        console.error(`Cloning: ${source.title} → ${eventData.title}`);
        const createResult = await apiRequest('POST', '/createEvent', token, createPayload, globalOpts.verbose);
        const newId = createResult.data.result?.data || createResult.data.result?.eventId;

        const output = {
          id: newId,
          source: sourceEventId,
          title: eventData.title,
          url: `https://partiful.com/e/${newId}`,
        };

        // Handle reinvite
        if (opts.reinvite) {
          const guestData = await fetchGuests(sourceEventId, config, token, globalOpts.verbose);
          const statusFilter = typeof opts.reinvite === 'string' ? opts.reinvite.toUpperCase() : null;
          const toReinvite = guestData.guests.filter((g) => !statusFilter || g.status === statusFilter);
          output.reinviteGuests = toReinvite.map((g) => ({ name: g.name, status: g.status }));
          output.reinviteCount = toReinvite.length;
          console.error(`Found ${toReinvite.length} guest(s) to reinvite`);
        }

        jsonOutput(output, {}, globalOpts);
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type);
        jsonError(e.message, EXIT.INTERNAL_ERROR, 'internal_error');
      }
    });
}
```

**Step 2: Register in events.js**

At the end of `registerEventsCommands`, add:
```javascript
import { registerCloneHelper } from '../helpers/clone.js';
registerCloneHelper(events);
```

**Step 3: Commit**

```bash
git add src/helpers/clone.js src/commands/events.js
git commit -m "feat: add events +clone helper with duration preservation"
```

---

### Task 3.2: Watch & Export Helpers

**Files:**
- Create: `src/helpers/watch.js`
- Create: `src/helpers/export.js`
- Create: `src/helpers/share.js`
- Modify: `src/commands/guests.js` — register +watch, +export
- Modify: `src/commands/events.js` — register +share

**Step 1: Implement watch.js** — RSVP polling, NDJSON change output
**Step 2: Implement export.js** — `fetchGuests` → CSV or JSON file
**Step 3: Implement share.js** — `getEvent` → share URL

These follow the same pattern as clone. Output JSON envelopes. Watch uses `setInterval` and emits NDJSON lines per change.

**Step 4: Commit**

```bash
git add src/helpers/watch.js src/helpers/export.js src/helpers/share.js src/commands/guests.js src/commands/events.js
git commit -m "feat: add +watch, +export, +share helpers"
```

---

## Phase 4: Schema, Version & Aliases

### Task 4.1: Schema Command

**Files:**
- Create: `src/commands/schema.js`
- Modify: `src/cli.js`

**Step 1: Implement schema.js**

Build a static registry of command parameters. `partiful schema events.create` returns the parameter definitions.

```javascript
// src/commands/schema.js
import { jsonOutput, jsonError, EXIT } from '../lib/output.js';

const SCHEMAS = {
  'events.list': {
    command: 'events list',
    parameters: {
      '--past': { type: 'boolean', required: false, description: 'Show past events' },
      '--include-cancelled': { type: 'boolean', required: false, description: 'Include cancelled events' },
    },
  },
  'events.get': {
    command: 'events get <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, description: 'Event ID', positional: true },
    },
  },
  'events.create': {
    command: 'events create',
    parameters: {
      '--title': { type: 'string', required: true, description: 'Event title' },
      '--date': { type: 'string', required: true, description: 'Start date/time (natural language)' },
      '--end-date': { type: 'string', required: false, description: 'End date/time' },
      '--location': { type: 'string', required: false, description: 'Venue name' },
      '--address': { type: 'string', required: false, description: 'Street address' },
      '--description': { type: 'string', required: false, description: 'Event description' },
      '--capacity': { type: 'integer', required: false, description: 'Guest limit' },
      '--private': { type: 'boolean', required: false, default: false, description: 'Make event private' },
      '--timezone': { type: 'string', required: false, default: 'America/Los_Angeles', description: 'Timezone' },
      '--theme': { type: 'string', required: false, default: 'oxblood', description: 'Color theme' },
    },
  },
  'events.update': {
    command: 'events update <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true },
      '--title': { type: 'string', required: false },
      '--date': { type: 'string', required: false },
      '--end-date': { type: 'string', required: false },
      '--location': { type: 'string', required: false },
      '--description': { type: 'string', required: false },
      '--capacity': { type: 'integer', required: false },
    },
  },
  'events.cancel': {
    command: 'events cancel <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true },
    },
  },
  'guests.list': {
    command: 'guests list <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true },
      '--status': { type: 'string', required: false, description: 'Filter by status' },
    },
  },
  'guests.invite': {
    command: 'guests invite <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true },
      '--phone': { type: 'string[]', required: false, description: 'Phone numbers' },
      '--user-id': { type: 'string[]', required: false, description: 'Partiful user IDs' },
      '--message': { type: 'string', required: false, description: 'Custom invitation message' },
    },
  },
  'contacts.list': {
    command: 'contacts list [query]',
    parameters: {
      query: { type: 'string', required: false, positional: true, description: 'Search query' },
      '--limit': { type: 'integer', required: false, default: 20 },
    },
  },
};

export function registerSchemaCommand(program) {
  program
    .command('schema <path>')
    .description('Introspect command parameters (e.g., events.create)')
    .action((path, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      const schema = SCHEMAS[path];
      if (!schema) {
        const available = Object.keys(SCHEMAS).join(', ');
        jsonError(`Unknown schema path: ${path}. Available: ${available}`, EXIT.NOT_FOUND, 'not_found');
      }
      jsonOutput(schema, {}, globalOpts);
    });
}
```

**Step 2: Wire into cli.js and commit**

```bash
git add src/commands/schema.js src/cli.js
git commit -m "feat: add schema introspection command for agent self-discovery"
```

---

### Task 4.2: Version Command & Deprecated Aliases

**Files:**
- Modify: `src/cli.js`

**Step 1: Add version command**

```javascript
program
  .command('version')
  .description('Show CLI version')
  .action(() => {
    jsonOutput({ version: '2.0.0', cli: 'partiful' });
  });
```

**Step 2: Add deprecated aliases**

```javascript
// Deprecated aliases — remove in v2.1
function deprecatedAlias(program, oldCmd, newCmd) {
  program.command(oldCmd, { hidden: true }).action((...args) => {
    console.error(`[deprecated] Use "partiful ${newCmd}" instead of "partiful ${oldCmd}"`);
    // Re-invoke with corrected args
    process.argv.splice(2, 1, ...newCmd.split(' '));
    program.parse();
  });
}
```

Add aliases for: `list` → `events list`, `get` → `events get`, `cancel` → `events cancel`, `clone` → `events +clone`, `contacts` → `contacts list`, `guests` → `guests list`.

**Step 3: Commit**

```bash
git add src/cli.js
git commit -m "feat: add version command and deprecated aliases for v1 commands"
```

---

## Phase 5: Skills

### Task 5.1: Write SKILL.md Files

**Files:**
- Create: `skills/partiful-shared/SKILL.md`
- Create: `skills/partiful-events/SKILL.md`
- Create: `skills/partiful-guests/SKILL.md`
- Create: `skills/recipe-clone-event/SKILL.md`

Follow the templates from cli-architect SKILL.md §3.3. Each skill includes: frontmatter, prerequisites, command reference, examples, exit codes.

**Step 1: Write all 4 skills**

Content follows the GWS pattern — auth precedence table, command signatures, example invocations with JSON output, error handling guidance.

**Step 2: Commit**

```bash
git add skills/
git commit -m "docs: add SKILL.md files for agent discoverability"
```

---

## Phase 6: Tests

### Task 6.1: Integration Tests

**Files:**
- Create: `tests/events.test.js`
- Create: `tests/guests.test.js`

Write tests that mock `apiRequest`/`firestoreRequest` and verify:
- JSON envelope shape on success
- Exit codes on various error types
- `--dry-run` returns payload without executing
- `--format` flag changes output

**Step 1: Write tests**
**Step 2: Run: `npx vitest run`** — all pass
**Step 3: Commit**

```bash
git add tests/
git commit -m "test: add integration tests for events and guests commands"
```

---

## Phase 7: README & Cleanup

### Task 7.1: Rewrite README

**Files:**
- Modify: `README.md`

GWS-style README with: badges, install, quick start, auth precedence, command reference table, exit codes, env vars, architecture, troubleshooting.

### Task 7.2: Remove Old Monolith

**Files:**
- Delete: `partiful` (old single-file CLI)
- Verify: `bin/partiful` is the new entry point

**Step 1: Remove old file, update .gitignore if needed**
**Step 2: Run full test suite**

Run: `npx vitest run`
Expected: All pass

**Step 3: Final commit**

```bash
git rm partiful
git add -A
git commit -m "feat: complete agentic CLI upgrade — remove v1 monolith"
```

### Task 7.3: Tag Release

```bash
git tag v2.0.0
git push origin main --tags
```

---

## Summary

| Phase | Tasks | Key Deliverable |
|-------|-------|----------------|
| 1 | 1.1–1.6 | Project scaffold + all `lib/` modules |
| 2 | 2.1–2.4 | All resource commands with JSON output |
| 3 | 3.1–3.2 | Helper commands (+clone, +watch, +export, +share) |
| 4 | 4.1–4.2 | Schema introspection + version + aliases |
| 5 | 5.1 | SKILL.md files for agent discoverability |
| 6 | 6.1 | Test suite |
| 7 | 7.1–7.3 | README, cleanup old monolith, tag release |

**Total tasks:** 15
**Estimated time:** 4-6 hours of implementation
