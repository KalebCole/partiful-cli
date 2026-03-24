/**
 * Blasts commands: send (stub)
 */

import { jsonError } from '../lib/output.js';

export function registerBlastsCommands(program) {
  const blasts = program.command('blasts').description('Text blasts to event guests');

  blasts
    .command('send')
    .description('Send a text blast to event guests (requires browser — stub)')
    .argument('<eventId>', 'Event ID')
    .option('--message <msg>', 'Message to send')
    .action(async (eventId) => {
      jsonError(
        `Text blasts require Firestore SDK (not available via REST). Use the web UI: https://partiful.com/e/${eventId}`,
        5,
        'not_implemented',
        { workaround: `https://partiful.com/e/${eventId}` }
      );
    });
}
