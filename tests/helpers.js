import { execFileSync } from 'child_process';
import { resolve } from 'path';

const CLI = resolve('bin/partiful');
const POSTER_CATALOG_FIXTURE = resolve('tests/fixtures/posters-catalog.json');

const baseEnv = {
  ...process.env,
  PARTIFUL_TOKEN: 'fake-token',
  PARTIFUL_POSTER_CATALOG_FILE: POSTER_CATALOG_FIXTURE,
};

export function run(args, opts = {}) {
  const stdout = execFileSync('node', [CLI, ...args], {
    encoding: 'utf-8',
    env: { ...baseEnv, ...opts.env },
    timeout: 10000,
  });
  return JSON.parse(stdout.trim());
}

export function runRaw(args, opts = {}) {
  try {
    const stdout = execFileSync('node', [CLI, ...args], {
      encoding: 'utf-8',
      env: { ...baseEnv, ...opts.env },
      timeout: 10000,
    });
    return { stdout, exitCode: 0 };
  } catch (e) {
    return { stdout: e.stdout || '', stderr: e.stderr || '', exitCode: e.status };
  }
}
