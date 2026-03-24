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
