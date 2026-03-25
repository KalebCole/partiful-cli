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
      '--poster': { type: 'string', required: false, description: 'Built-in poster ID' },
      '--poster-search': { type: 'string', required: false, description: 'Search poster library, use best match' },
      '--image': { type: 'string', required: false, description: 'Custom image file path or URL to upload' },
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
      '--poster': { type: 'string', required: false, description: 'Built-in poster ID' },
      '--poster-search': { type: 'string', required: false, description: 'Search poster library, use best match' },
      '--image': { type: 'string', required: false, description: 'Custom image file path or URL to upload' },
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
  'blasts.send': {
    command: 'blasts send <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true, description: 'Event ID' },
      '--message': { type: 'string', required: false, description: 'Message to send' },
    },
  },
  'posters.list': {
    command: 'posters list',
    parameters: {
      '--category': { type: 'string', required: false, description: 'Filter by category' },
      '--type': { type: 'string', required: false, description: 'Filter by content type (png, gif, jpeg)' },
      '--limit': { type: 'integer', required: false, default: 20, description: 'Max results' },
    },
  },
  'posters.search': {
    command: 'posters search <query>',
    parameters: {
      query: { type: 'string', required: true, positional: true, description: 'Search query' },
      '--limit': { type: 'integer', required: false, default: 10, description: 'Max results' },
    },
  },
  'posters.get': {
    command: 'posters get <posterId>',
    parameters: {
      posterId: { type: 'string', required: true, positional: true, description: 'Poster ID' },
    },
  },
};

export function registerSchemaCommand(program) {
  program
    .command('schema [path]')
    .description('Introspect command parameters (e.g., events.create)')
    .action((path, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      if (!path) {
        jsonOutput({ commands: Object.keys(SCHEMAS) }, { count: Object.keys(SCHEMAS).length }, globalOpts);
        return;
      }
      if (!Object.hasOwn(SCHEMAS, path)) {
        const available = Object.keys(SCHEMAS).join(', ');
        jsonError(`Unknown schema path: ${path}. Available: ${available}`, 4, 'not_found');
        return;
      }
      const schema = SCHEMAS[path];
      jsonOutput(schema, {}, globalOpts);
    });
}
