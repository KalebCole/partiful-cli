import crypto from 'crypto';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import { Command } from 'commander';
import { jsonError, jsonOutput } from '../lib/output.js';

const AGENTS = ['hermes', 'openclaw', 'copilot', 'claude'] as const;
type Agent = (typeof AGENTS)[number];

const MARKER_FILE = '.partiful-cli-install.json';
const LEGACY_OPENCLAW_SKILLS = [
  'partiful-events',
  'partiful-guests',
  'partiful-posters',
  'partiful-blasts',
  'partiful-shared',
] as const;

interface InstallMarker {
  installer: 'partiful-cli';
  skill: 'partiful';
  sourceHash: string;
}

interface GlobalOptions {
  dryRun?: boolean;
  force?: boolean;
  output?: string;
  [key: string]: unknown;
}

function packageSkillsDir(): string {
  const thisFile = fileURLToPath(import.meta.url);
  return path.resolve(path.dirname(thisFile), '..', '..', 'skills');
}

function sourceSkillDir(): string {
  return path.join(packageSkillsDir(), 'partiful');
}

function homeDir(): string {
  return process.env['HOME'] || process.env['USERPROFILE'] || '';
}

function parseAgent(raw: string): Agent {
  const agent = raw.toLowerCase();
  if (!AGENTS.includes(agent as Agent)) {
    jsonError(`Unsupported agent "${raw}". Supported agents: ${AGENTS.join(', ')}.`, 3, 'validation_error');
  }
  return agent as Agent;
}

function destinationFor(agent: Agent): string {
  const home = homeDir();
  if (!home) {
    jsonError('Cannot resolve user home directory. Set HOME (or USERPROFILE on Windows).', 3, 'validation_error');
  }

  const roots: Record<Agent, string> = {
    hermes: path.join(process.env['HERMES_HOME'] || path.join(home, '.hermes'), 'skills'),
    openclaw: path.join(home, '.openclaw', 'skills'),
    copilot: path.join(home, '.copilot', 'skills'),
    claude: path.join(home, '.claude', 'skills'),
  };
  return path.join(roots[agent], 'partiful');
}

function lstat(target: string): fs.Stats | null {
  try {
    return fs.lstatSync(target);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return null;
    throw error;
  }
}

function hashDirectory(root: string, ignored = new Set<string>()): string {
  const hash = crypto.createHash('sha256');

  function visit(current: string, relative = ''): void {
    const entries = fs.readdirSync(current, { withFileTypes: true })
      .filter((entry) => !ignored.has(path.join(relative, entry.name)))
      .sort((a, b) => a.name.localeCompare(b.name));
    for (const entry of entries) {
      const childRelative = path.join(relative, entry.name);
      const child = path.join(current, entry.name);
      hash.update(`${entry.isDirectory() ? 'd' : 'f'}:${childRelative}\0`);
      if (entry.isDirectory()) visit(child, childRelative);
      else hash.update(fs.readFileSync(child));
    }
  }

  visit(root);
  return hash.digest('hex');
}

function readMarker(destination: string): InstallMarker | null {
  try {
    const marker = JSON.parse(fs.readFileSync(path.join(destination, MARKER_FILE), 'utf8')) as Partial<InstallMarker>;
    if (marker.installer === 'partiful-cli' && marker.skill === 'partiful' && typeof marker.sourceHash === 'string') {
      return marker as InstallMarker;
    }
  } catch {
    // Missing or malformed provenance is intentionally treated as unowned.
  }
  return null;
}

function copySkill(source: string, destination: string, sourceHash: string): void {
  const parent = path.dirname(destination);
  fs.mkdirSync(parent, { recursive: true });
  const temporary = path.join(parent, `.partiful.tmp-${process.pid}-${crypto.randomBytes(4).toString('hex')}`);
  try {
    fs.cpSync(source, temporary, { recursive: true });
    const marker: InstallMarker = { installer: 'partiful-cli', skill: 'partiful', sourceHash };
    fs.writeFileSync(path.join(temporary, MARKER_FILE), `${JSON.stringify(marker, null, 2)}\n`);
    if (lstat(destination)) fs.rmSync(destination, { recursive: true, force: true });
    fs.renameSync(temporary, destination);
  } finally {
    fs.rmSync(temporary, { recursive: true, force: true });
  }
}

function install(agentRaw: string, cmd: Command): void {
  const agent = parseAgent(agentRaw);
  const globalOpts = cmd.optsWithGlobals<GlobalOptions>();
  const dryRun = Boolean(globalOpts.dryRun);
  const force = Boolean(globalOpts.force);
  const source = sourceSkillDir();
  const destination = destinationFor(agent);

  if (!fs.existsSync(path.join(source, 'SKILL.md'))) {
    jsonError(`Bundled Partiful skill not found: ${source}`, 5, 'internal_error');
  }

  const sourceHash = hashDirectory(source);
  const destinationStat = lstat(destination);
  let state = dryRun ? 'would_install' : 'installed';

  if (destinationStat) {
    const marker = destinationStat.isDirectory() ? readMarker(destination) : null;
    if (marker) {
      const destinationHash = hashDirectory(destination, new Set([MARKER_FILE]));
      if (destinationHash === sourceHash) {
        state = 'already_installed';
        jsonOutput({ action: 'install', agent, dryRun, state, source, destination }, {}, globalOpts);
        return;
      }
      if (!force && destinationHash !== marker.sourceHash) {
        jsonError(
          `Skill at ${destination} was modified after installation. Re-run with --force to replace it.`,
          3,
          'validation_error',
        );
      }
      state = dryRun ? 'would_update' : 'updated';
    } else if (!force) {
      jsonError(`Destination already exists and is not owned by partiful-cli: ${destination}. Re-run with --force to replace it.`, 3, 'validation_error');
    }
  }

  if (!dryRun) copySkill(source, destination, sourceHash);
  jsonOutput({ action: 'install', agent, dryRun, state, source, destination }, {}, globalOpts);
}

function legacyWorkspace(opts: Record<string, unknown>): string {
  const explicit = opts['workspace'];
  if (typeof explicit === 'string' && explicit) return explicit;
  if (process.env['OPENCLAW_WORKSPACE']) return process.env['OPENCLAW_WORKSPACE'];
  return path.join(homeDir(), '.openclaw', 'workspace');
}

function cleanupLegacyOpenClaw(workspace: string, dryRun: boolean, force: boolean): string[] {
  const removed: string[] = [];
  const workspaceSkills = path.join(workspace, 'skills');
  const currentPackageSkills = packageSkillsDir();
  for (const skill of LEGACY_OPENCLAW_SKILLS) {
    const link = path.join(workspaceSkills, skill);
    const stat = lstat(link);
    if (!stat?.isSymbolicLink()) continue;
    const target = path.resolve(path.dirname(link), fs.readlinkSync(link));
    const targetParent = path.dirname(target);
    const packageDir = path.dirname(targetParent);
    const packageOwned = path.basename(target) === skill
      && path.basename(targetParent) === 'skills'
      && (
        targetParent === currentPackageSkills
        || (path.basename(packageDir) === 'partiful-cli' && path.basename(path.dirname(packageDir)) === 'node_modules')
      );
    if (!packageOwned && !force) continue;
    if (!dryRun) fs.unlinkSync(link);
    removed.push(link);
  }
  return removed;
}

function uninstall(agentRaw: string, opts: Record<string, unknown>, cmd: Command): void {
  const agent = parseAgent(agentRaw);
  const globalOpts = cmd.optsWithGlobals<GlobalOptions>();
  const dryRun = Boolean(globalOpts.dryRun);
  const force = Boolean(globalOpts.force);
  const source = sourceSkillDir();
  const destination = destinationFor(agent);
  const destinationStat = lstat(destination);
  let state = 'not_installed';

  if (destinationStat) {
    const marker = destinationStat.isDirectory() ? readMarker(destination) : null;
    const ownedCopy = Boolean(marker);
    const ownedLink = destinationStat.isSymbolicLink()
      && path.resolve(path.dirname(destination), fs.readlinkSync(destination)) === source;
    if (!ownedCopy && !ownedLink) {
      jsonError(`Skill at ${destination} was not installed by partiful-cli; refusing to remove it.`, 3, 'validation_error');
    }
    if (marker && !force) {
      const destinationHash = hashDirectory(destination, new Set([MARKER_FILE]));
      if (destinationHash !== marker.sourceHash) {
        jsonError(
          `Skill at ${destination} was modified after installation. Re-run with --force to remove it.`,
          3,
          'validation_error',
        );
      }
    }
    state = dryRun ? 'would_remove' : 'removed';
    if (!dryRun) fs.rmSync(destination, { recursive: true, force: true });
  }

  const workspace = agent === 'openclaw' ? legacyWorkspace(opts) : null;
  const legacyRemoved = workspace ? cleanupLegacyOpenClaw(workspace, dryRun, force) : [];
  jsonOutput(
    { action: 'uninstall', agent, dryRun, state, destination, legacyWorkspace: workspace, legacyRemoved },
    {},
    globalOpts,
  );
}

export function registerSkillCommands(program: Command): void {
  const skill = program.command('skill').description('Install the bundled Partiful skill for an AI agent');

  skill
    .command('install <agent>')
    .description(`Install for one of: ${AGENTS.join(', ')}`)
    .action((agent: string, _opts: unknown, cmd: Command) => install(agent, cmd));

  skill
    .command('uninstall <agent>')
    .description(`Uninstall from one of: ${AGENTS.join(', ')}`)
    .option('--workspace <path>', 'Legacy OpenClaw workspace to clean')
    .action((agent: string, opts: Record<string, unknown>, cmd: Command) => uninstall(agent, opts, cmd));
}
