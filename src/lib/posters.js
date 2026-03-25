/**
 * Shared poster catalog helpers.
 */

let _catalogCache = null;

export async function fetchCatalog() {
  if (_catalogCache) return _catalogCache;
  const res = await fetch('https://assets.getpartiful.com/posters.json');
  if (!res.ok) throw new Error(`Failed to fetch poster catalog: ${res.status}`);
  _catalogCache = await res.json();
  return _catalogCache;
}

export function posterThumbnail(posterId) {
  return `https://partiful-posters.imgix.net/${encodeURIComponent(posterId)}?fit=max&w=400`;
}

export function searchPosters(catalog, query) {
  const q = query.toLowerCase();
  const results = [];
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

export function buildPosterImage(poster) {
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
