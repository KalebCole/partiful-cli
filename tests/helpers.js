import { execFileSync } from 'child_process';
import { resolve } from 'path';

const CLI = resolve('bin/partiful');

export function run(args, opts = {}) {
  const stdout = execFileSync('node', [CLI, ...args], {
    encoding: 'utf-8',
    env: { ...process.env, PARTIFUL_TOKEN: 'fake-token', ...opts.env },
    timeout: 10000,
  });
  return JSON.parse(stdout.trim());
}

export function runRaw(args, opts = {}) {
  try {
    const stdout = execFileSync('node', [CLI, ...args], {
      encoding: 'utf-8',
      env: { ...process.env, PARTIFUL_TOKEN: 'fake-token', ...opts.env },
      timeout: 10000,
    });
    return { stdout, exitCode: 0 };
  } catch (e) {
    return { stdout: e.stdout || '', stderr: e.stderr || '', exitCode: e.status };
  }
}
