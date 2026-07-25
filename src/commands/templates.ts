/**
 * Template commands — save, list, show, edit, delete event templates.
 */

import { Command } from 'commander';
import { loadTemplates, saveTemplates, extractTemplate } from '../lib/templates.js';
import { jsonOutput, jsonError } from '../lib/output.js';

export function registerTemplateCommands(program: Command): void {
  const template = program.command('template').description('Manage event templates');

  template
    .command('list')
    .description('List saved templates')
    .action((_opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      const templates = loadTemplates();
      const names = Object.keys(templates);
      if (names.length === 0) {
        jsonOutput([], { total: 0, hint: 'Save a template with: partiful template save <eventId> --name <name>' }, globalOpts);
        return;
      }
      const items = names.map(name => {
        const tpl = templates[name]!;
        return {
          name,
          title: (tpl['title'] as string | undefined) ?? '(no title)',
          location: (tpl['location'] as string | undefined) ?? '',
          fields: Object.keys(tpl).length,
        };
      });
      jsonOutput(items, { total: items.length }, globalOpts);
    });

  template
    .command('show <name>')
    .description('Show template details')
    .action((name: string, _opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      const templates = loadTemplates();
      if (!templates[name]) {
        jsonError(`Template "${name}" not found. Use "partiful template list" to see available templates.`, 4, 'not_found');
        return;
      }
      jsonOutput(templates[name], { name }, globalOpts);
    });

  template
    .command('save')
    .description('Save current CLI options as a template (or extract from an existing event)')
    .requiredOption('--name <name>', 'Template name')
    .option('--title <title>', 'Event title')
    .option('--location <location>', 'Location name')
    .option('--address <address>', 'Street address')
    .option('--description <desc>', 'Event description')
    .option('--capacity <n>', 'Guest limit', parseInt)
    .option('--private', 'Make event private')
    .option('--timezone <tz>', 'Timezone')
    .option('--theme <theme>', 'Color theme')
    .option('--effect <effect>', 'Visual effect')
    .option('--poster <posterId>', 'Built-in poster ID')
    .option('--poster-search <query>', 'Poster search query')
    .option('--link <url...>', 'Link URL (repeatable)')
    .option('--link-text <text...>', 'Display text for link')
    .option('--force', 'Overwrite existing template')
    .action((opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      const templates = loadTemplates();
      const name = opts['name'] as string;

      if (templates[name] && !opts['force'] && !globalOpts['force']) {
        jsonError(`Template "${name}" already exists. Use --force to overwrite.`, 3, 'validation_error');
        return;
      }

      const tpl = extractTemplate(opts);
      if (Object.keys(tpl).length === 0) {
        jsonError('No template fields provided. Use --title, --location, etc.', 3, 'validation_error');
        return;
      }

      templates[name] = tpl;
      saveTemplates(templates);
      jsonOutput(tpl, { name, action: 'saved' }, globalOpts);
    });

  template
    .command('edit <name>')
    .description('Edit a saved template')
    .option('--title <title>', 'Event title')
    .option('--location <location>', 'Location name')
    .option('--address <address>', 'Street address')
    .option('--description <desc>', 'Event description')
    .option('--capacity <n>', 'Guest limit', parseInt)
    .option('--private', 'Make event private')
    .option('--timezone <tz>', 'Timezone')
    .option('--theme <theme>', 'Color theme')
    .option('--effect <effect>', 'Visual effect')
    .option('--poster <posterId>', 'Built-in poster ID')
    .option('--poster-search <query>', 'Poster search query')
    .option('--link <url...>', 'Link URL (repeatable)')
    .option('--link-text <text...>', 'Display text for link')
    .option('--rename <newName>', 'Rename template')
    .action((name: string, opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      const templates = loadTemplates();

      if (!templates[name]) {
        jsonError(`Template "${name}" not found.`, 4, 'not_found');
        return;
      }

      const edits = extractTemplate(opts);
      const updated = { ...templates[name], ...edits };

      if (opts['rename']) {
        const newName = opts['rename'] as string;
        delete templates[name];
        templates[newName] = updated;
        saveTemplates(templates);
        jsonOutput(updated, { name: newName, renamedFrom: name, action: 'edited' }, globalOpts);
      } else {
        templates[name] = updated;
        saveTemplates(templates);
        jsonOutput(updated, { name, action: 'edited' }, globalOpts);
      }
    });

  template
    .command('delete <name>')
    .description('Delete a saved template')
    .action((name: string, _opts: Record<string, unknown>, cmd: Command) => {
      const globalOpts = cmd.optsWithGlobals<Record<string, unknown>>();
      const templates = loadTemplates();

      if (!templates[name]) {
        jsonError(`Template "${name}" not found.`, 4, 'not_found');
        return;
      }

      const deleted = templates[name];
      delete templates[name];
      saveTemplates(templates);
      jsonOutput(deleted, { name, action: 'deleted' }, globalOpts);
    });
}
