import { Command } from 'commander';
import { registerAuthCommands } from './commands/auth.js';
import { registerEventsCommands } from './commands/events.js';
import { registerGuestsCommands } from './commands/guests.js';
import { registerContactsCommands } from './commands/contacts.js';
import { registerBlastsCommands } from './commands/blasts.js';
import { registerCloneHelper } from './helpers/clone.js';
import { registerWatchHelper } from './helpers/watch.js';
import { registerExportHelper } from './helpers/export.js';
import { registerShareHelper } from './helpers/share.js';

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

  registerAuthCommands(program);
  registerEventsCommands(program);
  registerGuestsCommands(program);
  registerContactsCommands(program);
  registerBlastsCommands(program);
  registerCloneHelper(program);
  registerWatchHelper(program);
  registerExportHelper(program);
  registerShareHelper(program);

  program.parse();
}
