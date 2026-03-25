/**
 * Doctor command: diagnose CLI setup health.
 */

import os from 'os';
import fs from 'fs';
import { execSync } from 'child_process';
import { loadConfig, refreshAccessToken, resolveCredentialsPath, wrapPayload } from '../lib/auth.js';
import { apiRequest } from '../lib/http.js';
import { jsonOutput, jsonError } from '../lib/output.js';
import { PartifulError } from '../lib/errors.js';

const CHECKS = [
  { name: 'config_file', label: 'Config file' },
  { name: 'token_refresh', label: 'Token refresh' },
  { name: 'api_connectivity', label: 'API connectivity' },
  { name: 'environment', label: 'Environment' },
  { name: 'platform', label: 'Platform' },
];

function mask(value, visibleEnd = 4) {
  if (!value || typeof value !== 'string') return '(empty)';
  if (value.length <= visibleEnd) return '****';
  return '****' + value.slice(-visibleEnd);
}

async function runChecks() {
  const results = [];

  // 1. Config file
  const configPath = resolveCredentialsPath();
  const displayPath = configPath.replace(process.env.HOME, '~');
  let config = null;
  try {
    if (!fs.existsSync(configPath)) {
      results.push({ name: 'config_file', passed: false, detail: `Not found: ${displayPath}` });
    } else {
      const raw = fs.readFileSync(configPath, 'utf8');
      let parsed;
      try {
        parsed = JSON.parse(raw);
      } catch {
        results.push({ name: 'config_file', passed: false, detail: 'Invalid JSON' });
        return results;
      }
      const required = ['apiKey', 'refreshToken', 'userId'];
      const missing = required.filter(f => !parsed[f]);
      if (missing.length > 0) {
        results.push({ name: 'config_file', passed: false, detail: `Missing fields: ${missing.join(', ')}` });
      } else {
        config = parsed;
        results.push({ name: 'config_file', passed: true, detail: displayPath });
      }
    }
  } catch (e) {
    results.push({ name: 'config_file', passed: false, detail: e.message });
  }

  // 2. Token refresh
  if (!config) {
    results.push({ name: 'token_refresh', passed: false, detail: 'Skipped (no valid config)' });
  } else {
    try {
      const tokenResult = await refreshAccessToken(config);
      const expiresIn = parseInt(tokenResult.expires_in) || 0;
      const minutes = Math.floor(expiresIn / 60);
      config.accessToken = tokenResult.id_token;
      config.tokenExpiry = Date.now() + expiresIn * 1000;
      if (tokenResult.refresh_token) config.refreshToken = tokenResult.refresh_token;
      results.push({ name: 'token_refresh', passed: true, detail: `Token valid for ${minutes} min` });
    } catch (e) {
      results.push({ name: 'token_refresh', passed: false, detail: e.message });
    }
  }

  // 3. API connectivity
  if (!config || !config.accessToken) {
    results.push({ name: 'api_connectivity', passed: false, detail: 'Skipped (no token)' });
  } else {
    try {
      const payload = {
        data: wrapPayload(config, {
          params: {},
          amplitudeSessionId: Date.now(),
          userId: config.userId,
        }),
      };
      await apiRequest('POST', '/getMyUpcomingEventsForHomePage', config.accessToken, payload, false);
      results.push({ name: 'api_connectivity', passed: true, detail: 'api.partiful.com reachable' });
    } catch (e) {
      results.push({ name: 'api_connectivity', passed: false, detail: e.message });
    }
  }

  // 4. Environment
  const pkgPath = new URL('../../package.json', import.meta.url);
  const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));
  results.push({
    name: 'environment',
    passed: true,
    detail: `CLI v${pkg.version}, Node ${process.version}`,
  });

  // 5. Platform
  const platform = os.platform();
  const arch = os.arch();
  let smsDetail;
  if (platform === 'darwin') {
    try {
      execSync('which imsg', { stdio: 'ignore' });
      smsDetail = 'imsg available';
    } catch {
      smsDetail = 'imsg not found';
    }
  } else if (platform === 'linux' && process.env.TERMUX_VERSION) {
    smsDetail = 'termux detected';
  } else {
    smsDetail = 'no SMS auto-retrieve';
  }
  results.push({
    name: 'platform',
    passed: true,
    detail: `${platform} ${arch}, ${smsDetail}`,
  });

  return results;
}

function printTable(checks) {
  process.stderr.write('\nPartiful CLI — Doctor\n');
  process.stderr.write('─'.repeat(50) + '\n');
  for (const check of checks) {
    const icon = check.passed ? '✓' : '✗';
    const label = CHECKS.find(c => c.name === check.name)?.label || check.name;
    process.stderr.write(`  ${icon}  ${label.padEnd(20)} ${check.detail}\n`);
  }
  const allPassed = checks.every(c => c.passed);
  process.stderr.write('─'.repeat(50) + '\n');
  process.stderr.write(allPassed ? '  All checks passed ✓\n\n' : '  Some checks failed ✗\n\n');
}

export function registerDoctorCommands(program) {
  program
    .command('doctor')
    .description('Check CLI setup health and report diagnostics')
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();

      if (globalOpts.dryRun) {
        jsonOutput({
          checks: CHECKS.map(c => ({ name: c.name, label: c.label })),
          note: 'Dry run — no checks executed',
        });
        return;
      }

      try {
        const checks = await runChecks();
        const allPassed = checks.every(c => c.passed);

        if (globalOpts.format !== 'json') {
          printTable(checks);
        }

        const envelope = {
          status: 'success',
          data: { checks, allPassed },
        };
        process.stdout.write(JSON.stringify(envelope) + '\n');

        if (!allPassed) process.exit(1);
      } catch (e) {
        if (e instanceof PartifulError) jsonError(e.message, e.exitCode, e.type, e.details);
        else jsonError(e.message);
      }
    });
}
