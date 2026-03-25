/**
 * Template commands — save, list, show, edit, delete event templates.
 */

import { loadTemplates, saveTemplates, extractTemplate, applyVariables } from '../lib/templates.js';
import { jsonOutput, jsonError } from '../lib/output.js';

export function registerTemplateCommands(program) {
  const template = program.command('template').description('Manage event templates');

  template
    .command('list')
    .description('List saved templates')
    .action((opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      const templates = loadTemplates();
      const names = Object.keys(templates);
      if (names.length === 0) {
        jsonOutput([], { total: 0, hint: 'Save a template with: partiful template save <eventId> --name <name>' }, globalOpts);
        return;
      }
      const items = names.map(name => ({
        name,
        title: templates[name].title || '(no title)',
        location: templates[name].location || '',
        fields: Object.keys(templates[name]).length,
      }));
      jsonOutput(items, { total: items.length }, globalOpts);
    });

  template
    .command('show <name>')
    .description('Show template details')
    .action((name, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
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
    .action((opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      const templates = loadTemplates();
      const name = opts.name;

      if (templates[name] && !opts.force && !globalOpts.force) {
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
    .action((name, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      const templates = loadTemplates();

      if (!templates[name]) {
        jsonError(`Template "${name}" not found.`, 4, 'not_found');
        return;
      }

      // Apply edits
      const edits = extractTemplate(opts);
      const updated = { ...templates[name], ...edits };

      if (opts.rename) {
        delete templates[name];
        templates[opts.rename] = updated;
        saveTemplates(templates);
        jsonOutput(updated, { name: opts.rename, renamedFrom: name, action: 'edited' }, globalOpts);
      } else {
        templates[name] = updated;
        saveTemplates(templates);
        jsonOutput(updated, { name, action: 'edited' }, globalOpts);
      }
    });

  template
    .command('delete <name>')
    .description('Delete a saved template')
    .action((name, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
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
