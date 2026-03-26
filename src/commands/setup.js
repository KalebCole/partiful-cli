/**
 * Setup command: configure OpenClaw integration.
 */

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import { jsonOutput, jsonError } from '../lib/output.js';

function getPackageSkillsDir() {
  const thisFile = fileURLToPath(import.meta.url);
  const packageRoot = path.resolve(path.dirname(thisFile), '..', '..');
  return path.join(packageRoot, 'skills');
}

function resolveWorkspace(optPath) {
  if (optPath) return optPath;
  if (process.env.OPENCLAW_WORKSPACE) return process.env.OPENCLAW_WORKSPACE;
  const defaultPath = path.join(process.env.HOME, '.openclaw', 'workspace');
  if (fs.existsSync(defaultPath)) return defaultPath;
  return null;
}

function getSkillDirs(skillsSource) {
  return fs.readdirSync(skillsSource).filter(
    d => d.startsWith('partiful-') && fs.statSync(path.join(skillsSource, d)).isDirectory()
  );
}

export function registerSetupCommands(program) {
  const setup = program
    .command('setup')
    .description('Setup and integration commands');

  setup
    .command('openclaw')
    .description('Link partiful skills into an OpenClaw workspace')
    .option('--workspace <path>', 'OpenClaw workspace path')
    .option('--uninstall', 'Remove symlinks instead of creating them')
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      const force = globalOpts.force || false;
      const dryRun = globalOpts.dryRun || false;

      const workspace = resolveWorkspace(opts.workspace);
      if (!workspace) {
        jsonError(
          'Could not find OpenClaw workspace. Set $OPENCLAW_WORKSPACE, ensure ~/.openclaw/workspace exists, or pass --workspace <path>.',
          3, 'validation_error'
        );
        return;
      }

      const skillsSource = getPackageSkillsDir();
      if (!fs.existsSync(skillsSource)) {
        jsonError(`Skills directory not found: ${skillsSource}`, 5, 'internal_error');
        return;
      }

      const workspaceSkills = path.join(workspace, 'skills');
      let sourceDirs;
      try {
        sourceDirs = getSkillDirs(skillsSource);
      } catch (e) {
        jsonError(`Cannot read skills directory: ${e.message}`, 5, 'internal_error');
        return;
      }

      if (sourceDirs.length === 0) {
        jsonError('No partiful-* skill directories found in package.', 5, 'internal_error');
        return;
      }

      // Uninstall mode
      if (opts.uninstall) {
        const removed = [];
        const skipped = [];

        for (const dir of sourceDirs) {
          const linkPath = path.join(workspaceSkills, dir);
          let stat;
          try { stat = fs.lstatSync(linkPath); } catch { skipped.push({ skill: dir, reason: 'not found' }); continue; }

          if (!stat.isSymbolicLink()) {
            skipped.push({ skill: dir, reason: 'not a symlink' });
            continue;
          }

          if (!dryRun) fs.unlinkSync(linkPath);
          removed.push({ skill: dir, path: linkPath });
        }

        jsonOutput({ action: 'uninstall', dryRun, workspace, removed, skipped }, {}, globalOpts);
        return;
      }

      // Install mode
      if (!dryRun && !fs.existsSync(workspaceSkills)) {
        fs.mkdirSync(workspaceSkills, { recursive: true });
      }

      const linked = [];
      const skipped = [];

      for (const dir of sourceDirs) {
        const target = path.join(skillsSource, dir);
        const linkPath = path.join(workspaceSkills, dir);

        // Check if something already exists at linkPath
        let stat;
        try { stat = fs.lstatSync(linkPath); } catch { stat = null; }

        if (stat) {
          if (stat.isSymbolicLink()) {
            const existing = fs.readlinkSync(linkPath);
            const resolvedExisting = path.resolve(path.dirname(linkPath), existing);
            if (resolvedExisting === target) {
              skipped.push({ skill: dir, reason: 'already linked' });
              continue;
            }
            // Points somewhere else
            if (force) {
              if (!dryRun) fs.unlinkSync(linkPath);
            } else {
              skipped.push({ skill: dir, reason: `symlink exists → ${existing}` });
              continue;
            }
          } else {
            skipped.push({ skill: dir, reason: 'path exists (not a symlink)' });
            continue;
          }
        }

        if (!dryRun) fs.symlinkSync(target, linkPath);
        linked.push({ skill: dir, from: target, to: linkPath });
      }

      jsonOutput({ action: 'install', dryRun, workspace, linked, skipped }, {}, globalOpts);
    });
}
