/**
 * Template storage — saves/loads event templates from ~/.config/partiful/templates.json
 */

import fs from 'fs';
import path from 'path';

/** A saved template is a bag of CLI-option-shaped fields. */
export type Template = Record<string, unknown>;
/** A map of template name -> template. */
export type TemplateStore = Record<string, Template>;

function templatesPath(): string {
  return (
    process.env.PARTIFUL_TEMPLATES_FILE ||
    path.join(process.env.HOME as string, '.config/partiful/templates.json')
  );
}

export function loadTemplates(): TemplateStore {
  const p = templatesPath();
  if (!fs.existsSync(p)) return {};
  try {
    return JSON.parse(fs.readFileSync(p, 'utf8'));
  } catch {
    return {};
  }
}

export function saveTemplates(templates: TemplateStore): void {
  const p = templatesPath();
  const dir = path.dirname(p);
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(p, JSON.stringify(templates, null, 2));
}

/** Fields we save from an event into a template */
const TEMPLATE_FIELDS = [
  'title', 'location', 'address', 'description', 'timezone',
  'capacity', 'private', 'theme', 'effect', 'poster', 'posterSearch',
  'link', 'linkText',
];

/**
 * Extract template-worthy fields from CLI opts or an API event object.
 */
export function extractTemplate(source: Record<string, unknown>): Template {
  const tpl: Template = {};
  for (const key of TEMPLATE_FIELDS) {
    if (source[key] !== undefined && source[key] !== null) {
      tpl[key] = source[key];
    }
  }
  // Map API event fields to CLI option names
  if (source.guestLimit && !tpl.capacity) tpl.capacity = source.guestLimit;
  if (source.visibility === 'private' && !tpl.private) tpl.private = true;
  const displaySettings = source.displaySettings as
    | { theme?: unknown; effect?: unknown }
    | undefined;
  if (displaySettings) {
    if (displaySettings.theme && !tpl.theme) tpl.theme = displaySettings.theme;
    if (displaySettings.effect && !tpl.effect) tpl.effect = displaySettings.effect;
  }
  const links = source.links as Array<{ url: string; text?: string }> | undefined;
  if (links && !tpl.link) {
    tpl.link = links.map((l) => l.url);
    tpl.linkText = links.map((l) => l.text || l.url);
  }
  return tpl;
}

/**
 * Apply variable substitution: {{varName}} → value
 */
export function applyVariables(
  template: Template,
  vars: Record<string, string> | null | undefined,
): Template {
  if (!vars || Object.keys(vars).length === 0) return { ...template };
  const result: Template = {};
  for (const [key, value] of Object.entries(template)) {
    if (typeof value === 'string') {
      result[key] = value.replace(/\{\{(\w+)\}\}/g, (match, name) => {
        return vars[name] !== undefined ? vars[name]! : match;
      });
    } else {
      result[key] = value;
    }
  }
  return result;
}

/**
 * Merge template with CLI overrides. CLI opts win.
 */
export function mergeTemplateOpts(
  template: Template,
  opts: Record<string, unknown>,
): Template {
  const merged: Template = { ...template };
  for (const key of TEMPLATE_FIELDS) {
    if (opts[key] !== undefined && opts[key] !== null) {
      merged[key] = opts[key];
    }
  }
  // Date is always from CLI
  if (opts.date) merged.date = opts.date;
  if (opts.endDate) merged.endDate = opts.endDate;
  return merged;
}
