# Event Images CLI — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add poster browsing (`posters list/search`) and image support (`--poster`, `--image`) to `events create` and `events update`, closing issue #5.

**Architecture:** New `posters.js` command module fetches the public poster catalog (`https://assets.getpartiful.com/posters.json`). Event commands get `--poster <id>` and `--image <path|url>` flags. Poster selection builds the `event.image` object with `source: "partiful_posters"`. Custom upload uses `POST /uploadPhoto` with `uploadType: "event_poster"` via multipart FormData. Update uses `POST /updateEvent` (callable function) instead of Firestore PATCH for image fields since the image object is complex/nested.

**Tech Stack:** Node.js, Commander, vitest, native fetch + FormData

**Design Spec:** `docs/research/2026-03-24-event-image-schema.md`

**Closes:** #5

---

## API Research Summary

### Image Source Types (from app bundle module 90126)

```text
GIPHY = "giphy"       — GIF search (Giphy API, skip for v1)
LOCAL = "local"        — client-only, not persisted
UNSPLASH = "unsplash"  — stock photos (skip for v1)
UPLOAD = "upload"      — custom image upload
PARTIFUL_POSTERS = "partiful_posters" — built-in poster library
```

### Poster Catalog

- **URL:** `https://assets.getpartiful.com/posters.json` (public, no auth)
- **Count:** ~1,979 posters
- **Content types:** PNG, JPEG, AVIF, GIF (1 GIF)
- **Categories:** Trending, Birthday, Elegant, Minimal, Dinner Party, Themed, Community Made, Chill, Not Chill, Holiday, College, Watch Party, Wedding, Outdoors, etc.

### Image Object in createEvent Payload

**Poster:**
```json
{
  "image": {
    "source": "partiful_posters",
    "poster": { "id": "...", "name": "...", "contentType": "...", "url": "...", "width": 1600, "height": 1600, ... },
    "url": "https://assets.getpartiful.com/posters/<id>",
    "blurHash": "...",
    "contentType": "image/png",
    "name": "<id>",
    "height": 1600,
    "width": 1600
  }
}
```

**Custom Upload:**
```json
{
  "image": {
    "source": "upload",
    "type": "image",
    "upload": { "path": "<storage-path>", "url": "<cdn-url>", "contentType": "...", "size": 12345, "width": 800, "height": 600 },
    "url": "<cdn-url>",
    "contentType": "...",
    "name": "<filename>",
    "height": 600,
    "width": 800
  }
}
```

### Upload Endpoint

```text
POST https://api.partiful.com/uploadPhoto
Content-Type: multipart/form-data

FormData:
  - file: <binary>
  - (params are URL-encoded in the callable function URL)

Callable function pattern: POST to `${baseUrl}/uploadPhoto` with params: { uploadType: "event_poster" }
Returns: { uploadData: { path, url, contentType, size, width, height } }
```

---

## Phase 1: Poster Browsing (`posters list`, `posters search`)

### Task 1.1: Create `src/commands/posters.js` — list command

**Files:**
- Create: `src/commands/posters.js`
- Modify: `src/cli.js` (register new command)

**Step 1: Write the failing test**

Create `tests/posters-integration.test.js`:

```javascript
import { describe, it, expect } from 'vitest';
import { run } from './helpers.js';

describe('posters integration', () => {
  describe('posters list', () => {
    it('lists posters with default limit', () => {
      const out = run(['posters', 'list']);
      expect(out.status).toBe('success');
      expect(out.data).toBeInstanceOf(Array);
      expect(out.data.length).toBeGreaterThan(0);
      expect(out.data.length).toBeLessThanOrEqual(20);
      expect(out.data[0]).toHaveProperty('id');
      expect(out.data[0]).toHaveProperty('url');
      expect(out.data[0]).toHaveProperty('categories');
    });

    it('respects --limit', () => {
      const out = run(['posters', 'list', '--limit', '5']);
      expect(out.data.length).toBeLessThanOrEqual(5);
    });

    it('filters by --category', () => {
      const out = run(['posters', 'list', '--category', 'Birthday']);
      expect(out.data.length).toBeGreaterThan(0);
      out.data.forEach(p => {
        expect(p.categories).toContain('Birthday');
      });
    });

    it('returns metadata count', () => {
      const out = run(['posters', 'list', '--limit', '3']);
      expect(out.metadata.count).toBeDefined();
      expect(out.metadata.totalAvailable).toBeDefined();
    });
  });
});
```

**Step 2: Run test to verify it fails**

```bash
npx vitest run tests/posters-integration.test.js
```

Expected: FAIL — `posters` command not found.

**Step 3: Implement `src/commands/posters.js`**

```javascript
/**
 * Posters commands: list, search
 * Browses Partiful's public poster catalog.
 */

import { jsonOutput, jsonError } from '../lib/output.js';

const CATALOG_URL = 'https://assets.getpartiful.com/posters.json';
let _cache = null;

async function fetchCatalog() {
  if (_cache) return _cache;
  const res = await fetch(CATALOG_URL);
  if (!res.ok) throw new Error(`Failed to fetch poster catalog: ${res.status}`);
  _cache = await res.json();
  return _cache;
}

export function registerPosterCommands(program) {
  const posters = program.command('posters').description('Browse Partiful poster library');

  posters
    .command('list')
    .description('List available posters')
    .option('--category <category>', 'Filter by category (e.g. Birthday, Trending)')
    .option('--type <type>', 'Filter by content type (png, gif, jpeg)', '')
    .option('--limit <n>', 'Max results', parseInt, 20)
    .action(async (opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        let catalog = await fetchCatalog();

        if (opts.category) {
          const cat = opts.category.toLowerCase();
          catalog = catalog.filter(p =>
            p.categories.some(c => c.toLowerCase() === cat)
          );
        }

        if (opts.type) {
          const t = opts.type.toLowerCase();
          catalog = catalog.filter(p =>
            p.contentType.includes(t)
          );
        }

        const totalAvailable = catalog.length;
        const limited = catalog.slice(0, opts.limit);

        const data = limited.map(p => ({
          id: p.id,
          name: p.name,
          contentType: p.contentType,
          categories: p.categories,
          tags: p.tags,
          width: p.width,
          height: p.height,
          url: p.url,
          thumbnail: `https://partiful-posters.imgix.net/${encodeURIComponent(p.id)}?fit=max&w=400`,
          bgColor: p.bgColor,
        }));

        jsonOutput(data, { count: data.length, totalAvailable });
      } catch (e) {
        jsonError(e.message);
      }
    });

  posters
    .command('search')
    .description('Search posters by keyword')
    .argument('<query>', 'Search query (matches tags, name, categories)')
    .option('--limit <n>', 'Max results', parseInt, 10)
    .action(async (query, opts, cmd) => {
      const globalOpts = cmd.optsWithGlobals();
      try {
        const catalog = await fetchCatalog();
        const q = query.toLowerCase();

        const scored = catalog.map(p => {
          let score = 0;
          // Tag exact match = highest
          if (p.tags.some(t => t.toLowerCase() === q)) score += 10;
          // Tag partial match
          if (p.tags.some(t => t.toLowerCase().includes(q))) score += 5;
          // Name match
          if (p.name.toLowerCase().includes(q)) score += 3;
          // Category match
          if (p.categories.some(c => c.toLowerCase().includes(q))) score += 2;
          return { poster: p, score };
        })
          .filter(s => s.score > 0)
          .sort((a, b) => b.score - a.score)
          .slice(0, opts.limit);

        const data = scored.map(s => ({
          id: s.poster.id,
          name: s.poster.name,
          contentType: s.poster.contentType,
          categories: s.poster.categories,
          tags: s.poster.tags,
          url: s.poster.url,
          thumbnail: `https://partiful-posters.imgix.net/${encodeURIComponent(s.poster.id)}?fit=max&w=400`,
          score: s.score,
        }));

        jsonOutput(data, { count: data.length, query });
      } catch (e) {
        jsonError(e.message);
      }
    });

  posters
    .command('get')
    .description('Get full poster details by ID')
    .argument('<posterId>', 'Poster ID')
    .action(async (posterId, opts, cmd) => {
      try {
        const catalog = await fetchCatalog();
        const poster = catalog.find(p => p.id === posterId);
        if (!poster) {
          jsonError(`Poster not found: ${posterId}`, 4, 'not_found');
          return;
        }
        jsonOutput(poster);
      } catch (e) {
        jsonError(e.message);
      }
    });
}
```

**Step 4: Register in `src/cli.js`**

Add import and registration:

```javascript
import { registerPosterCommands } from './commands/posters.js';
// ... in run():
registerPosterCommands(program);
```

**Step 5: Run tests to verify they pass**

```bash
npx vitest run tests/posters-integration.test.js
```

Expected: PASS (all tests). Note: these tests make a real HTTP call to the public catalog URL. If that's a concern for CI, we can mock later.

**Step 6: Commit**

```bash
git add src/commands/posters.js src/cli.js tests/posters-integration.test.js
git commit -m "feat(#5): add posters list/search/get commands"
```

---

### Task 1.2: Add search tests

**Files:**
- Modify: `tests/posters-integration.test.js`

**Step 1: Add search tests to existing file**

```javascript
  describe('posters search', () => {
    it('finds posters by tag', () => {
      const out = run(['posters', 'search', 'birthday']);
      expect(out.status).toBe('success');
      expect(out.data.length).toBeGreaterThan(0);
      expect(out.data[0]).toHaveProperty('score');
    });

    it('respects --limit', () => {
      const out = run(['posters', 'search', 'party', '--limit', '3']);
      expect(out.data.length).toBeLessThanOrEqual(3);
    });

    it('returns empty for nonsense query', () => {
      const out = run(['posters', 'search', 'xyzzyflurble123']);
      expect(out.data).toEqual([]);
      expect(out.metadata.count).toBe(0);
    });
  });

  describe('posters get', () => {
    it('returns poster by exact ID', () => {
      const out = run(['posters', 'get', 'piscesairbrush.png']);
      expect(out.status).toBe('success');
      expect(out.data.id).toBe('piscesairbrush.png');
      expect(out.data.url).toContain('assets.getpartiful.com');
    });

    it('errors on unknown poster', () => {
      const out = run(['posters', 'get', 'does-not-exist-xyz']);
      expect(out.status).toBe('error');
      expect(out.error.type).toBe('not_found');
    });
  });
```

**Step 2: Run tests**

```bash
npx vitest run tests/posters-integration.test.js
```

Expected: PASS (all).

**Step 3: Commit**

```bash
git add tests/posters-integration.test.js
git commit -m "test(#5): add search and get poster tests"
```

---

## Phase 2: Poster Support in `events create`

### Task 2.1: Add `--poster` flag to `events create`

**Files:**
- Modify: `src/commands/events.js`
- Modify: `tests/events-integration.test.js`

**Step 1: Write the failing test**

Add to `tests/events-integration.test.js`:

```javascript
    it('events create --poster includes image in payload', () => {
      const out = run([
        'events', 'create',
        '--title', 'Poster Test',
        '--date', '2026-06-01 7pm',
        '--poster', 'piscesairbrush.png',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      const event = out.data.payload.data.params.event;
      expect(event.image).toBeDefined();
      expect(event.image.source).toBe('partiful_posters');
      expect(event.image.poster.id).toBe('piscesairbrush.png');
      expect(event.image.url).toContain('assets.getpartiful.com');
    });

    it('events create --poster errors on unknown poster', () => {
      const out = run([
        'events', 'create',
        '--title', 'Bad Poster',
        '--date', '2026-06-01 7pm',
        '--poster', 'nonexistent-poster-xyz',
        '--dry-run',
      ]);
      expect(out.status).toBe('error');
      expect(out.error.type).toBe('not_found');
    });
```

**Step 2: Run test to verify it fails**

```bash
npx vitest run tests/events-integration.test.js -t "poster"
```

Expected: FAIL — `--poster` option not recognized.

**Step 3: Implement `--poster` in events create**

In `src/commands/events.js`, add to the `create` command:

1. Add option: `.option('--poster <posterId>', 'Built-in poster ID (use "posters search" to find)')`
2. After building the `event` object, before building `payload`:

```javascript
        // Handle poster image
        if (opts.poster) {
          const posterCatalogUrl = 'https://assets.getpartiful.com/posters.json';
          const catalogRes = await fetch(posterCatalogUrl);
          if (!catalogRes.ok) throw new PartifulError('Failed to fetch poster catalog', 1, 'fetch_error');
          const catalog = await catalogRes.json();
          const poster = catalog.find(p => p.id === opts.poster);
          if (!poster) {
            jsonError(`Poster not found: "${opts.poster}". Use "partiful posters search <term>" to find posters.`, 4, 'not_found');
            return;
          }
          event.image = {
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
```

**Step 4: Run tests**

```bash
npx vitest run tests/events-integration.test.js
```

Expected: PASS.

**Step 5: Commit**

```bash
git add src/commands/events.js tests/events-integration.test.js
git commit -m "feat(#5): add --poster flag to events create"
```

---

### Task 2.2: Add `--poster-search` convenience flag

**Files:**
- Modify: `src/commands/events.js`
- Modify: `tests/events-integration.test.js`

**Step 1: Write failing test**

```javascript
    it('events create --poster-search finds and uses best match', () => {
      const out = run([
        'events', 'create',
        '--title', 'Search Test',
        '--date', '2026-06-01 7pm',
        '--poster-search', 'birthday',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      const event = out.data.payload.data.params.event;
      expect(event.image).toBeDefined();
      expect(event.image.source).toBe('partiful_posters');
    });

    it('events create errors when both --poster and --poster-search given', () => {
      const out = run([
        'events', 'create',
        '--title', 'Conflict',
        '--date', '2026-06-01 7pm',
        '--poster', 'piscesairbrush.png',
        '--poster-search', 'birthday',
        '--dry-run',
      ]);
      expect(out.status).toBe('error');
    });
```

**Step 2: Implement**

Add option: `.option('--poster-search <query>', 'Search poster library and use best match')`

Add validation + logic:

```javascript
        if (opts.poster && opts.posterSearch) {
          jsonError('Cannot use both --poster and --poster-search. Pick one.', 3, 'validation_error');
          return;
        }

        if (opts.posterSearch) {
          const catalogRes = await fetch('https://assets.getpartiful.com/posters.json');
          if (!catalogRes.ok) throw new PartifulError('Failed to fetch poster catalog', 1, 'fetch_error');
          const catalog = await catalogRes.json();
          const q = opts.posterSearch.toLowerCase();
          const match = catalog
            .map(p => {
              let score = 0;
              if (p.tags.some(t => t.toLowerCase() === q)) score += 10;
              if (p.tags.some(t => t.toLowerCase().includes(q))) score += 5;
              if (p.name.toLowerCase().includes(q)) score += 3;
              if (p.categories.some(c => c.toLowerCase().includes(q))) score += 2;
              return { poster: p, score };
            })
            .filter(s => s.score > 0)
            .sort((a, b) => b.score - a.score)[0];

          if (!match) {
            jsonError(`No posters found matching "${opts.posterSearch}". Try "partiful posters search <term>".`, 4, 'not_found');
            return;
          }

          const poster = match.poster;
          event.image = {
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
```

**Step 3: Run tests, commit**

```bash
npx vitest run tests/events-integration.test.js
git add src/commands/events.js tests/events-integration.test.js
git commit -m "feat(#5): add --poster-search flag to events create"
```

---

### Task 2.3: Extract shared poster helpers to `src/lib/posters.js`

The poster catalog fetch and search logic is duplicated between `commands/posters.js` and `commands/events.js`. Extract to a shared module.

**Files:**
- Create: `src/lib/posters.js`
- Modify: `src/commands/posters.js` (use shared helpers)
- Modify: `src/commands/events.js` (use shared helpers)

**Step 1: Create `src/lib/posters.js`**

```javascript
/**
 * Poster catalog helpers — shared between posters commands and events create.
 */

const CATALOG_URL = 'https://assets.getpartiful.com/posters.json';
let _cache = null;

export async function fetchCatalog() {
  if (_cache) return _cache;
  const res = await fetch(CATALOG_URL);
  if (!res.ok) throw new Error(`Failed to fetch poster catalog: HTTP ${res.status}`);
  _cache = await res.json();
  return _cache;
}

export function searchPosters(catalog, query) {
  const q = query.toLowerCase();
  return catalog
    .map(p => {
      let score = 0;
      if (p.tags.some(t => t.toLowerCase() === q)) score += 10;
      if (p.tags.some(t => t.toLowerCase().includes(q))) score += 5;
      if (p.name.toLowerCase().includes(q)) score += 3;
      if (p.categories.some(c => c.toLowerCase().includes(q))) score += 2;
      return { poster: p, score };
    })
    .filter(s => s.score > 0)
    .sort((a, b) => b.score - a.score);
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

export function posterThumbnail(posterId) {
  return `https://partiful-posters.imgix.net/${encodeURIComponent(posterId)}?fit=max&w=400`;
}
```

**Step 2: Update `commands/posters.js` and `commands/events.js` to use shared helpers**

Replace inline catalog fetch/search/build with imports from `../lib/posters.js`.

**Step 3: Run all tests**

```bash
npx vitest run
```

Expected: all pass, no behavior change.

**Step 4: Commit**

```bash
git add src/lib/posters.js src/commands/posters.js src/commands/events.js
git commit -m "refactor(#5): extract shared poster helpers to lib/posters.js"
```

---

## Phase 3: Custom Image Upload

### Task 3.1: Add upload helper to `src/lib/upload.js`

**Files:**
- Create: `src/lib/upload.js`
- Create: `tests/upload.test.js`

**Step 1: Write the test (unit test with mock)**

```javascript
import { describe, it, expect, vi } from 'vitest';
import { buildUploadImage } from '../src/lib/upload.js';

describe('buildUploadImage', () => {
  it('builds correct image object from upload data', () => {
    const uploadData = {
      path: 'eventImages/abc123/poster.png',
      url: 'https://firebasestorage.googleapis.com/v0/b/getpartiful.appspot.com/o/eventImages%2Fabc123%2Fposter.png?alt=media',
      contentType: 'image/png',
      size: 123456,
      width: 800,
      height: 600,
    };
    const filename = 'poster.png';
    const result = buildUploadImage(uploadData, filename);
    expect(result.source).toBe('upload');
    expect(result.type).toBe('image');
    expect(result.upload).toEqual(uploadData);
    expect(result.url).toBe(uploadData.url);
    expect(result.contentType).toBe('image/png');
    expect(result.name).toBe('poster.png');
    expect(result.width).toBe(800);
    expect(result.height).toBe(600);
  });
});
```

**Step 2: Implement `src/lib/upload.js`**

```javascript
/**
 * Image upload helper for Partiful CLI.
 * Handles uploading local files or URLs to Partiful's storage.
 */

import { readFileSync, existsSync } from 'fs';
import { basename, extname } from 'path';
import { getValidToken, loadConfig, wrapPayload } from './auth.js';

const UPLOAD_TYPES = {
  EVENT_POSTER: 'event_poster',
};

const ALLOWED_TYPES = {
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.webp': 'image/webp',
  '.avif': 'image/avif',
};

const MAX_SIZE = 10 * 1024 * 1024; // 10MB

export async function uploadEventImage(filePath, token, config, verbose = false) {
  if (!existsSync(filePath)) {
    throw new Error(`File not found: ${filePath}`);
  }

  const ext = extname(filePath).toLowerCase();
  const contentType = ALLOWED_TYPES[ext];
  if (!contentType) {
    throw new Error(`Unsupported image type: ${ext}. Allowed: ${Object.keys(ALLOWED_TYPES).join(', ')}`);
  }

  const fileBuffer = readFileSync(filePath);
  if (fileBuffer.length > MAX_SIZE) {
    throw new Error(`File too large: ${(fileBuffer.length / 1024 / 1024).toFixed(1)}MB. Max: ${MAX_SIZE / 1024 / 1024}MB`);
  }

  const fileName = basename(filePath);
  const blob = new Blob([fileBuffer], { type: contentType });
  const file = new File([blob], fileName, { type: contentType });

  const formData = new FormData();
  formData.append('file', file);

  // Build the callable function URL with params
  const baseUrl = 'https://us-central1-getpartiful.cloudfunctions.net';
  const params = new URLSearchParams({
    uploadType: UPLOAD_TYPES.EVENT_POSTER,
  });

  const res = await fetch(`${baseUrl}/uploadPhoto?${params}`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
    },
    body: formData,
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Upload failed (${res.status}): ${text}`);
  }

  const result = await res.json();
  return result.uploadData || result.result?.uploadData || result;
}

export function buildUploadImage(uploadData, filename) {
  return {
    source: 'upload',
    type: 'image',
    upload: uploadData,
    url: uploadData.url,
    contentType: uploadData.contentType,
    name: filename,
    height: uploadData.height,
    width: uploadData.width,
  };
}
```

**Step 3: Run test, commit**

```bash
npx vitest run tests/upload.test.js
git add src/lib/upload.js tests/upload.test.js
git commit -m "feat(#5): add image upload helper"
```

---

### Task 3.2: Add `--image` flag to `events create`

**Files:**
- Modify: `src/commands/events.js`
- Modify: `tests/events-integration.test.js`

**Step 1: Write failing test (dry-run only — can't test real upload without API)**

```javascript
    it('events create --image validates file extension', () => {
      const out = run([
        'events', 'create',
        '--title', 'Upload Test',
        '--date', '2026-06-01 7pm',
        '--image', '/tmp/not-an-image.txt',
        '--dry-run',
      ]);
      expect(out.status).toBe('error');
      expect(out.error.message).toContain('Unsupported');
    });

    it('events create errors when --poster and --image used together', () => {
      const out = run([
        'events', 'create',
        '--title', 'Conflict',
        '--date', '2026-06-01 7pm',
        '--poster', 'piscesairbrush.png',
        '--image', '/tmp/test.png',
        '--dry-run',
      ]);
      expect(out.status).toBe('error');
    });
```

**Step 2: Implement**

Add option: `.option('--image <path>', 'Custom image file path to upload')`

Add validation:

```javascript
        const imageOpts = [opts.poster, opts.posterSearch, opts.image].filter(Boolean).length;
        if (imageOpts > 1) {
          jsonError('Use only one of --poster, --poster-search, or --image.', 3, 'validation_error');
          return;
        }

        if (opts.image) {
          const { uploadEventImage, buildUploadImage } = await import('../lib/upload.js');
          const ext = opts.image.split('.').pop().toLowerCase();
          const allowed = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'avif'];
          if (!allowed.includes(ext)) {
            jsonError(`Unsupported image type: .${ext}. Allowed: ${allowed.map(e => '.' + e).join(', ')}`, 3, 'validation_error');
            return;
          }

          if (globalOpts.dryRun) {
            event.image = { source: 'upload', file: opts.image, note: 'File will be uploaded on real run' };
          } else {
            const uploadData = await uploadEventImage(opts.image, token, config, globalOpts.verbose);
            event.image = buildUploadImage(uploadData, opts.image.split('/').pop());
          }
        }
```

**Step 3: Run tests, commit**

```bash
npx vitest run tests/events-integration.test.js
git add src/commands/events.js tests/events-integration.test.js
git commit -m "feat(#5): add --image flag to events create"
```

---

## Phase 4: Image Support in `events update`

### Task 4.1: Add `--poster` and `--image` to `events update`

**Files:**
- Modify: `src/commands/events.js`
- Modify: `tests/events-integration.test.js`

**Note:** The current `update` uses Firestore PATCH which works for flat fields but the `image` object is complex. For image updates, we'll use the `updateEvent` callable function (same pattern as `createEvent`). If that endpoint doesn't exist, we fall back to `createEvent`-style Firestore write on the `image` field.

**Step 1: Write failing test**

```javascript
    it('events update --poster in dry-run', () => {
      const out = run([
        'events', 'update', 'test-event-123',
        '--poster', 'piscesairbrush.png',
        '--dry-run',
      ]);
      expect(out.status).toBe('success');
      expect(out.data.dryRun).toBe(true);
      expect(out.data.fields).toContain('image');
    });
```

**Step 2: Implement**

Add options to update command:
```javascript
    .option('--poster <posterId>', 'Set poster by ID')
    .option('--poster-search <query>', 'Search and set best matching poster')
    .option('--image <path>', 'Upload and set custom image')
```

Add image handling logic similar to create. For the Firestore PATCH, the image field needs to be serialized as a Firestore map value:

```javascript
        // Handle image options
        if (opts.poster || opts.posterSearch || opts.image) {
          const { fetchCatalog, searchPosters, buildPosterImage } = await import('../lib/posters.js');
          let imageObj = null;

          if (opts.poster) {
            const catalog = await fetchCatalog();
            const poster = catalog.find(p => p.id === opts.poster);
            if (!poster) { jsonError(`Poster not found: ${opts.poster}`, 4, 'not_found'); return; }
            imageObj = buildPosterImage(poster);
          } else if (opts.posterSearch) {
            const catalog = await fetchCatalog();
            const results = searchPosters(catalog, opts.posterSearch);
            if (results.length === 0) { jsonError(`No posters matching "${opts.posterSearch}"`, 4, 'not_found'); return; }
            imageObj = buildPosterImage(results[0].poster);
          } else if (opts.image) {
            if (!globalOpts.dryRun) {
              const { uploadEventImage, buildUploadImage } = await import('../lib/upload.js');
              const uploadData = await uploadEventImage(opts.image, token, config, globalOpts.verbose);
              imageObj = buildUploadImage(uploadData, opts.image.split('/').pop());
            } else {
              imageObj = { source: 'upload', file: opts.image, note: 'File will be uploaded on real run' };
            }
          }

          // Serialize image as Firestore map
          fields.image = { mapValue: { fields: firestoreSerialize(imageObj) } };
          updateFields.push('image');
        }
```

**Note:** Firestore map serialization is complex for nested objects. We may need a helper function `firestoreSerialize` that converts a JS object to Firestore value types. If this gets too complex, use `apiRequest('POST', '/updateEvent', ...)` instead.

**Step 3: Run tests, commit**

```bash
npx vitest run
git add src/commands/events.js tests/events-integration.test.js
git commit -m "feat(#5): add --poster/--image to events update"
```

---

## Phase 5: Schema & Documentation

### Task 5.1: Update schema definitions

**Files:**
- Modify: `src/commands/schema.js`

Add poster schemas and update event schemas:

```javascript
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
```

Update `events.create` schema to include:

```javascript
      '--poster': { type: 'string', required: false, description: 'Built-in poster ID' },
      '--poster-search': { type: 'string', required: false, description: 'Search poster library, use best match' },
      '--image': { type: 'string', required: false, description: 'Custom image file path to upload' },
```

Same for `events.update`.

**Commit:**

```bash
git add src/commands/schema.js
git commit -m "docs(#5): update schema with poster and image commands"
```

---

### Task 5.2: Update research docs and issue

**Files:**
- Already created: `docs/research/2026-03-24-event-image-schema.md`

**Step 1: Close issue #5 via PR description**

```bash
git push -u origin sasha/event-images
gh pr create --title "feat(#5): Event images — poster library + custom upload" \
  --body "Closes #5

## What
- New \`posters list/search/get\` commands to browse Partiful's poster library (1,979 posters, no auth needed)
- \`--poster <id>\` flag on \`events create/update\` to use a built-in poster
- \`--poster-search <query>\` flag for natural language poster matching
- \`--image <path>\` flag for custom image upload via \`uploadPhoto\` endpoint
- Updated schema introspection for all new commands
- Research doc: \`docs/research/2026-03-24-event-image-schema.md\`

## Image Source Types
| Source | Flag | Auth |
|--------|------|------|
| Built-in posters | \`--poster\` / \`--poster-search\` | None (public catalog) |
| Custom upload | \`--image\` | Firebase token |
| Giphy GIFs | Future | API key |
| Unsplash photos | Future | API key |

## Testing
- Poster list/search/get: integration tests (real HTTP to public catalog)
- Poster flag in events create: dry-run tests
- Upload helper: unit tests with mock
- Image validation: extension checks, size limits, mutual exclusion"
```

---

## Summary

| Phase | Tasks | What it delivers |
|-------|-------|-----------------|
| 1 | 1.1-1.2 | `posters list/search/get` commands |
| 2 | 2.1-2.3 | `--poster` and `--poster-search` on events create |
| 3 | 3.1-3.2 | `--image` custom upload on events create |
| 4 | 4.1 | `--poster`/`--image` on events update |
| 5 | 5.1-5.2 | Schema updates, docs, PR |

**Estimated time:** 2-3 hours
**Branch:** `sasha/event-images`
**Closes:** Issue #5
