import fs from 'fs';

/** Structured JSON output envelope emitted on success. */
export interface OutputOptions {
  output?: string;
  [key: string]: unknown;
}

export function jsonOutput(
  data: unknown,
  metadata: Record<string, unknown> = {},
  opts: OutputOptions = {},
): void {
  const envelope = { status: 'success', data, metadata };
  const json = JSON.stringify(envelope);
  if (opts.output) {
    fs.writeFileSync(opts.output, json + '\n');
  } else {
    process.stdout.write(json + '\n');
  }
}

export function jsonError(
  message: string,
  code = 5,
  type = 'internal_error',
  details: unknown = null,
): never {
  const envelope = {
    status: 'error',
    error: { code, type, message, ...(details ? { details } : {}) },
  };
  process.stdout.write(JSON.stringify(envelope) + '\n');
  process.exit(code);
}

/** A row is a record of column name -> printable cell value. */
export type TableRow = Record<string, unknown>;

export function formatTable(rows: TableRow[], columns: string[]): string {
  if (!rows || rows.length === 0) return '(no results)';
  const widths = columns.map((col) =>
    Math.max(col.length, ...rows.map((r) => String(r[col] ?? '').length)),
  );
  const header = columns.map((col, i) => col.padEnd(widths[i]!)).join('  ');
  const sep = widths.map((w) => '─'.repeat(w)).join('──');
  const body = rows
    .map((r) =>
      columns.map((col, i) => String(r[col] ?? '').padEnd(widths[i]!)).join('  '),
    )
    .join('\n');
  return `${header}\n${sep}\n${body}`;
}

export function formatCsv(rows: TableRow[], columns: string[]): string {
  const escape = (v: unknown): string => {
    const s = String(v ?? '');
    return s.includes(',') || s.includes('"') || s.includes('\n')
      ? `"${s.replace(/"/g, '""')}"`
      : s;
  };
  const header = columns.map(escape).join(',');
  const body = rows.map((r) => columns.map((col) => escape(r[col])).join(',')).join('\n');
  return `${header}\n${body}`;
}

export const EXIT = {
  SUCCESS: 0,
  API_ERROR: 1,
  AUTH_ERROR: 2,
  VALIDATION_ERROR: 3,
  NOT_FOUND: 4,
  INTERNAL_ERROR: 5,
} as const;
