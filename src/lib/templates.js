/**
 * Template storage — saves/loads event templates from ~/.config/partiful/templates.json
 */

import fs from 'fs';
import path from 'path';

function templatesPath() {
  return process.env.PARTIFUL_TEMPLATES_FILE
    || path.join(process.env.HOME, '.config/partiful/templates.json');
}

export function loadTemplates() {
  const p = templatesPath();
  if (!fs.existsSync(p)) return {};
  try {
    return JSON.parse(fs.readFileSync(p, 'utf8'));
  } catch {
    return {};
  }
}

export function saveTemplates(templates) {
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
export function extractTemplate(source) {
  const tpl = {};
  for (const key of TEMPLATE_FIELDS) {
    if (source[key] !== undefined && source[key] !== null) {
      tpl[key] = source[key];
    }
  }
  // Map API event fields to CLI option names
  if (source.guestLimit && !tpl.capacity) tpl.capacity = source.guestLimit;
  if (source.visibility === 'private' && !tpl.private) tpl.private = true;
  if (source.displaySettings) {
    if (source.displaySettings.theme && !tpl.theme) tpl.theme = source.displaySettings.theme;
    if (source.displaySettings.effect && !tpl.effect) tpl.effect = source.displaySettings.effect;
  }
  if (source.links && !tpl.link) {
    tpl.link = source.links.map(l => l.url);
    tpl.linkText = source.links.map(l => l.text || l.url);
  }
  return tpl;
}

/**
 * Apply variable substitution: {{varName}} → value
 */
export function applyVariables(template, vars) {
  if (!vars || Object.keys(vars).length === 0) return { ...template };
  const result = {};
  for (const [key, value] of Object.entries(template)) {
    if (typeof value === 'string') {
      result[key] = value.replace(/\{\{(\w+)\}\}/g, (match, name) => {
        return vars[name] !== undefined ? vars[name] : match;
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
export function mergeTemplateOpts(template, opts) {
  const merged = { ...template };
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
