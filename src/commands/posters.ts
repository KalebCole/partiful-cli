/**
 * Poster browsing commands: list, search, get
 */

import { Command } from 'commander';
import { fetchCatalog, searchPosters, posterThumbnail } from '../lib/posters.js';
import type { Poster, ScoredPoster } from '../lib/posters.js';
import { jsonOutput, jsonError } from '../lib/output.js';

function summarizePoster(p: Poster) {
  return {
    id: p.id,
    name: p.name,
    contentType: p.contentType,
    categories: p.categories,
    tags: p.tags,
    width: p.width,
    height: p.height,
    url: p.url,
    thumbnail: p.id ? posterThumbnail(p.id) : null,
    bgColor: p.bgColor,
  };
}

export function registerPosterCommands(program: Command): void {
  const posters = program.command('posters').description('Browse poster catalog');

  posters
    .command('list')
    .description('List available posters')
    .option('--category <category>', 'Filter by category')
    .option('--type <type>', 'Filter by content type (png, gif, jpeg)')
    .option('--limit <n>', 'Max results', '20')
    .action(async (opts: Record<string, unknown>) => {
      try {
        const catalog = await fetchCatalog();
        let filtered = catalog;
        if (opts['category']) {
          const cat = (opts['category'] as string).toLowerCase();
          filtered = filtered.filter((p) =>
            p.categories && p.categories.some((c) => c.toLowerCase() === cat)
          );
        }
        if (opts['type']) {
          const t = (opts['type'] as string).toLowerCase();
          filtered = filtered.filter((p) =>
            p.contentType && p.contentType.toLowerCase().includes(t)
          );
        }
        const limit = parseInt(opts['limit'] as string, 10);
        if (isNaN(limit) || limit < 1) {
          jsonError('--limit must be a positive integer', 3, 'validation_error');
          return;
        }
        const results = filtered.slice(0, limit).map(summarizePoster);
        jsonOutput(results, { count: results.length, totalAvailable: filtered.length });
      } catch (err) {
        jsonError(err instanceof Error ? err.message : String(err), 5, 'internal_error');
      }
    });

  posters
    .command('search <query>')
    .description('Search posters by keyword')
    .option('--limit <n>', 'Max results', '10')
    .action(async (query: string, opts: Record<string, unknown>) => {
      try {
        const catalog = await fetchCatalog();
        const results: ScoredPoster[] = searchPosters(catalog, query);
        const limit = parseInt(opts['limit'] as string, 10);
        if (isNaN(limit) || limit < 1) {
          jsonError('--limit must be a positive integer', 3, 'validation_error');
          return;
        }
        const limited = results.slice(0, limit).map((p) => ({
          ...summarizePoster(p),
          score: p.score,
        }));
        jsonOutput(limited, { count: limited.length, totalMatches: results.length });
      } catch (err) {
        jsonError(err instanceof Error ? err.message : String(err), 5, 'internal_error');
      }
    });

  posters
    .command('get <posterId>')
    .description('Get full poster details by ID')
    .action(async (posterId: string) => {
      try {
        const catalog = await fetchCatalog();
        const poster = catalog.find((p) => p.id === posterId);
        if (!poster) {
          jsonError(`Poster not found: ${posterId}`, 4, 'not_found');
          return;
        }
        jsonOutput(poster);
      } catch (err) {
        jsonError(err instanceof Error ? err.message : String(err), 5, 'internal_error');
      }
    });
}
