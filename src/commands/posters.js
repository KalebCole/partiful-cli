/**
 * Poster browsing commands: list, search, get
 */

import { fetchCatalog, searchPosters, posterThumbnail } from '../lib/posters.js';
import { jsonOutput, jsonError } from '../lib/output.js';

function summarizePoster(p) {
  return {
    id: p.id,
    name: p.name,
    contentType: p.contentType,
    categories: p.categories,
    tags: p.tags,
    width: p.width,
    height: p.height,
    url: p.url,
    thumbnail: posterThumbnail(p.id),
    bgColor: p.bgColor,
  };
}

export function registerPosterCommands(program) {
  const posters = program.command('posters').description('Browse poster catalog');

  posters
    .command('list')
    .description('List available posters')
    .option('--category <category>', 'Filter by category')
    .option('--type <type>', 'Filter by content type (png, gif, jpeg)')
    .option('--limit <n>', 'Max results', '20')
    .action(async (opts) => {
      try {
        const catalog = await fetchCatalog();
        let filtered = catalog;
        if (opts.category) {
          const cat = opts.category.toLowerCase();
          filtered = filtered.filter(p =>
            p.categories && p.categories.some(c => c.toLowerCase() === cat)
          );
        }
        if (opts.type) {
          const t = opts.type.toLowerCase();
          filtered = filtered.filter(p =>
            p.contentType && p.contentType.toLowerCase().includes(t)
          );
        }
        const limit = parseInt(opts.limit, 10) || 20;
        const results = filtered.slice(0, limit).map(summarizePoster);
        jsonOutput(results, { count: results.length, totalAvailable: filtered.length });
      } catch (err) {
        jsonError(err.message, 5, 'internal_error');
      }
    });

  posters
    .command('search <query>')
    .description('Search posters by keyword')
    .option('--limit <n>', 'Max results', '10')
    .action(async (query, opts) => {
      try {
        const catalog = await fetchCatalog();
        const results = searchPosters(catalog, query);
        const limit = parseInt(opts.limit, 10) || 10;
        const limited = results.slice(0, limit).map(p => ({
          ...summarizePoster(p),
          score: p.score,
        }));
        jsonOutput(limited, { count: limited.length, totalMatches: results.length });
      } catch (err) {
        jsonError(err.message, 5, 'internal_error');
      }
    });

  posters
    .command('get <posterId>')
    .description('Get full poster details by ID')
    .action(async (posterId) => {
      try {
        const catalog = await fetchCatalog();
        const poster = catalog.find(p => p.id === posterId);
        if (!poster) {
          jsonError(`Poster not found: ${posterId}`, 4, 'not_found');
          return;
        }
        jsonOutput(poster);
      } catch (err) {
        jsonError(err.message, 5, 'internal_error');
      }
    });
}
