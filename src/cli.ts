import { Command } from 'commander';
import { createRequire } from 'module';
import { registerAuthCommands } from './commands/auth.js';
import { registerEventsCommands } from './commands/events.js';
import { registerGuestsCommands } from './commands/guests.js';
import { registerContactsCommands } from './commands/contacts.js';
import { registerCohostsCommands } from './commands/cohosts.js';
import { registerBlastsCommands } from './commands/blasts.js';
import { registerWatchHelper } from './helpers/watch.js';
import { registerExportHelper } from './helpers/export.js';
import { registerSchemaCommand } from './commands/schema.js';
import { registerPosterCommands } from './commands/posters.js';
import { registerDoctorCommands } from './commands/doctor.js';
import { registerTemplateCommands } from './commands/templates.js';
import { registerBulkCommands } from './commands/bulk.js';
import { registerRsvpCommands } from './commands/rsvp.js';
import { registerSkillCommands } from './commands/skill.js';
import { jsonOutput } from './lib/output.js';

// Single source of truth for the version — read from package.json so the
// CLI's --version can never drift from the published package version.
const { version: pkgVersion } = createRequire(import.meta.url)('../package.json') as { version: string };

export function run(): void {
  const program = new Command();

  program
    .name('partiful')
    .description('Manage Partiful events from the command line — JSON-first, agent-friendly')
    .version(pkgVersion)
    .option('--format <format>', 'Output format: json, table, csv, ndjson', process.env.PARTIFUL_FORMAT || 'json')
    .option('--dry-run', 'Preview request without executing')
    .option('-y, --yes', 'Skip confirmation prompts')
    .option('--force', 'Skip confirmation and overwrite protection')
    .option('-v, --verbose', 'Show request details on stderr')
    .option('-o, --output <path>', 'Write output to file')
    .option('--no-color', 'Disable colored output');

  registerAuthCommands(program);
  registerEventsCommands(program);
  registerGuestsCommands(program);
  registerContactsCommands(program);
  registerCohostsCommands(program);
  registerBlastsCommands(program);
  registerWatchHelper(program);
  registerExportHelper(program);
  registerSchemaCommand(program);
  registerPosterCommands(program);
  registerDoctorCommands(program);
  registerTemplateCommands(program);
  registerBulkCommands(program);
  registerSkillCommands(program);
  // RSVP / interest verbs, shared across the canonical `events` group and the
  // `explore` alias group. Look up the `events` command created above; create
  // the `explore` group here.
  const eventsCmd = program.commands.find((c) => c.name() === 'events');
  const exploreCmd = program.command('explore').description('Discover public events and RSVP to them');
  registerRsvpCommands(program, { events: eventsCmd!, explore: exploreCmd });

  program
    .command('version')
    .description('Show CLI version and info')
    .action((_opts: unknown, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals();
      jsonOutput({ version: program.version(), cli: 'partiful', node: process.version }, {}, globalOpts);
    });

  // Deprecated aliases — rewrite argv before parsing
  const args = process.argv.slice(2);
  const aliasMap: Record<string, string[]> = {
    list: ['events', 'list'],
    get: ['events', 'get'],
    cancel: ['events', 'cancel'],
    clone: ['events', 'clone'],
  };

  // Find first non-option token (skip --format <val>, -o <val>, etc.)
  const optsWithValue = new Set(['--format', '-o', '--output']);
  let cmdIndex = 0;
  while (cmdIndex < args.length && args[cmdIndex]!.startsWith('-')) {
    cmdIndex += optsWithValue.has(args[cmdIndex]!) ? 2 : 1;
  }

  const legacy = args[cmdIndex];
  if (legacy && aliasMap[legacy]) {
    const rewritten = [
      ...args.slice(0, cmdIndex),
      ...aliasMap[legacy]!,
      ...args.slice(cmdIndex + 1),
    ];
    process.stderr.write(
      `[deprecated] "partiful ${legacy}" → use "partiful ${aliasMap[legacy]!.join(' ')}" instead\n`,
    );
    process.argv = [...process.argv.slice(0, 2), ...rewritten];
  }

  // Special case: `partiful guests <id>` → `partiful guests list <id>`
  if (args[0] === 'guests' && args[1] && !['list', 'invite', '--help', '-h'].includes(args[1])) {
    process.stderr.write(`[deprecated] "partiful guests <id>" → use "partiful guests list <id>" instead\n`);
    process.argv = [...process.argv.slice(0, 2), 'guests', 'list', ...args.slice(1)];
  }

  program.parse();
}
