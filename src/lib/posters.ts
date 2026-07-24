/**
 * Shared poster catalog helpers.
 */

/** A poster entry from the Partiful catalog (broad shape, extra fields allowed). */
export interface Poster {
  id?: string;
  name?: string;
  url?: string;
  blurHash?: string;
  contentType?: string;
  height?: number;
  width?: number;
  tags?: string[];
  categories?: string[];
  [extra: string]: unknown;
}

/** A poster augmented with its search relevance score. */
export interface ScoredPoster extends Poster {
  score: number;
}

/** The image object we attach to an event when using a catalog poster. */
export interface PosterImage {
  source: 'partiful_posters';
  poster: Poster;
  url?: string;
  blurHash?: string;
  contentType?: string;
  name?: string;
  height?: number;
  width?: number;
}

let _catalogCache: Poster[] | null = null;

export async function fetchCatalog(): Promise<Poster[]> {
  if (_catalogCache) return _catalogCache;

  // Support local fixture for testing
  const localFile = process.env.PARTIFUL_POSTER_CATALOG_FILE;
  if (localFile) {
    const { readFileSync } = await import('fs');
    _catalogCache = JSON.parse(readFileSync(localFile, 'utf-8'));
    return _catalogCache!;
  }

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 10000);
  try {
    const res = await fetch('https://assets.getpartiful.com/posters.json', { signal: controller.signal });
    if (!res.ok) throw new Error(`Failed to fetch poster catalog: ${res.status}`);
    _catalogCache = (await res.json()) as Poster[];
    return _catalogCache!;
  } catch (err) {
    if ((err as Error).name === 'AbortError') throw new Error('Poster catalog fetch timed out (10s)');
    throw err;
  } finally {
    clearTimeout(timeout);
  }
}

export function posterThumbnail(posterId: string): string {
  return `https://partiful-posters.imgix.net/${encodeURIComponent(posterId)}?fit=max&w=400`;
}

export function searchPosters(catalog: Poster[], query: string): ScoredPoster[] {
  const q = query.toLowerCase();
  const results: ScoredPoster[] = [];
  for (const poster of catalog) {
    let score = 0;
    // Tag exact match
    if (poster.tags) {
      for (const tag of poster.tags) {
        if (tag.toLowerCase() === q) score += 10;
        else if (tag.toLowerCase().includes(q)) score += 5;
      }
    }
    // Name match
    if (poster.name && poster.name.toLowerCase().includes(q)) score += 3;
    // Category match
    if (poster.categories) {
      for (const cat of poster.categories) {
        if (cat.toLowerCase().includes(q)) score += 2;
      }
    }
    if (score > 0) results.push({ ...poster, score });
  }
  results.sort((a, b) => b.score - a.score);
  return results;
}

export function buildPosterImage(poster: Poster): PosterImage {
  return {
    source: 'partiful_posters',
    poster,
    url: poster.url,
    blurHash: poster.blurHash,
    contentType: poster.contentType,
    name: poster.name,
    height: poster.height,
    width: poster.width,
  };
}
