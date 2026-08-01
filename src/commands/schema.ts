import { jsonOutput, jsonError, EXIT } from '../lib/output.js';
import type { Command } from 'commander';
import { apiEndpoints } from '../lib/api/endpoints.js';

/** A single parameter descriptor in a command schema. */
interface SchemaParameter {
  type: string;
  required: boolean;
  description?: string;
  default?: unknown;
  positional?: boolean;
}

/** A command schema entry: invocation string + parameter map. */
interface CommandSchema {
  command: string;
  parameters: Record<string, SchemaParameter>;
}

const SCHEMAS: Record<string, CommandSchema> = {
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
      '--cohost': { type: 'string[]', required: false, description: 'Co-host names (resolved from contacts)' },
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
      '--cohost': { type: 'string[]', required: false, description: 'Co-host names (resolved from contacts)' },
    },
  },
  'events.cancel': {
    command: 'events cancel <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true },
    },
  },
  'events.rsvp': {
    command: 'events rsvp <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true, description: 'Event ID' },
      '--status': { type: 'string', required: false, default: 'going', description: 'RSVP status: going, maybe, declined' },
      '--name': { type: 'string', required: false, description: 'Display name to RSVP with (defaults to profile name)' },
      '--plus-one': { type: 'string[]', required: false, description: 'Plus-one name (repeatable)' },
      '--count': { type: 'integer', required: false, description: 'Total headcount including plus-ones' },
      '--message': { type: 'string', required: false, description: 'Optional public comment on the event' },
      '--password': { type: 'string', required: false, description: 'Event password (if password-gated)' },
      '--timezone': { type: 'string', required: false, description: 'IANA timezone for the RSVP' },
    },
  },
  'events.interested': {
    command: 'events interested <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true, description: 'Event ID' },
      '--remove': { type: 'boolean', required: false, description: 'Remove interest instead of adding it' },
    },
  },
  'explore.rsvp': {
    command: 'explore rsvp <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true, description: 'Event ID' },
      '--status': { type: 'string', required: false, default: 'going', description: 'RSVP status: going, maybe, declined' },
      '--name': { type: 'string', required: false, description: 'Display name to RSVP with (defaults to profile name)' },
      '--plus-one': { type: 'string[]', required: false, description: 'Plus-one name (repeatable)' },
      '--count': { type: 'integer', required: false, description: 'Total headcount including plus-ones' },
      '--message': { type: 'string', required: false, description: 'Optional public comment on the event' },
      '--password': { type: 'string', required: false, description: 'Event password (if password-gated)' },
      '--timezone': { type: 'string', required: false, description: 'IANA timezone for the RSVP' },
    },
  },
  'explore.interested': {
    command: 'explore interested <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true, description: 'Event ID' },
      '--remove': { type: 'boolean', required: false, description: 'Remove interest instead of adding it' },
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
  'cohosts.list': {
    command: 'cohosts list <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true, description: 'Event ID' },
    },
  },
  'cohosts.add': {
    command: 'cohosts add <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true, description: 'Event ID' },
      '--name': { type: 'string[]', required: false, description: 'Co-host names (must resolve uniquely from contacts)' },
      '--user-id': { type: 'string[]', required: false, description: 'Partiful user IDs' },
    },
  },
  'cohosts.link': {
    command: 'cohosts link <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true, description: 'Event ID' },
      '--enable': { type: 'boolean', required: false, description: 'Enable/create the co-host invite link' },
      '--disable': { type: 'boolean', required: false, description: 'Disable/revoke the co-host invite link' },
    },
  },
  'cohosts.remove': {
    command: 'cohosts remove <eventId>',
    parameters: {
      eventId: { type: 'string', required: true, positional: true, description: 'Event ID' },
      '--user-id': { type: 'string', required: true, description: 'User ID to remove' },
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

export function registerSchemaCommand(program: Command): void {
  program
    .command('schema [path]')
    .description('Introspect command parameters (e.g., events.create) or API endpoints (e.g., api.createEvent)')
    .action((path: string | undefined, _opts: unknown, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals();

      // ── API-endpoint introspection namespace: `schema api` / `schema api.<method>`
      // Reads from the T3 endpoint registry (api/endpoints.ts) — the spec-as-types
      // source of truth: host, method, transport, request params, response fields.
      if (path === 'api') {
        const methods = Object.keys(apiEndpoints);
        jsonOutput({ methods }, { count: methods.length }, globalOpts);
        return;
      }
      if (path && path.startsWith('api.')) {
        const method = path.slice('api.'.length);
        if (!Object.hasOwn(apiEndpoints, method)) {
          const available = Object.keys(apiEndpoints).join(', ');
          jsonError(`Unknown API method: ${method}. Available: ${available}`, EXIT.NOT_FOUND, 'not_found');
          return;
        }
        const meta = apiEndpoints[method as keyof typeof apiEndpoints];
        jsonOutput(
          {
            method: `api.${method}`,
            transport: meta.transport,
            host: meta.host,
            httpMethod: meta.method,
            path: meta.path,
            requestParams: meta.requestParams,
            responseFields: meta.responseFields,
          },
          {},
          globalOpts,
        );
        return;
      }

      // ── CLI-flag introspection (original behavior)
      if (!path) {
        jsonOutput(
          { commands: Object.keys(SCHEMAS), api: Object.keys(apiEndpoints).map((m) => `api.${m}`) },
          { count: Object.keys(SCHEMAS).length + Object.keys(apiEndpoints).length },
          globalOpts,
        );
        return;
      }
      if (!Object.hasOwn(SCHEMAS, path)) {
        const available = Object.keys(SCHEMAS).join(', ');
        jsonError(`Unknown schema path: ${path}. Available: ${available}`, EXIT.NOT_FOUND, 'not_found');
        return;
      }
      const schema = SCHEMAS[path];
      jsonOutput(schema, {}, globalOpts);
    });
}
