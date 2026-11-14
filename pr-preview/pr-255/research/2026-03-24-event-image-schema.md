# Partiful Event Image — API Research

**Date:** 2026-03-24
**Method:** Browser interception on partiful.com/create with poster selection

---

## Image Field in createEvent Payload

The `event.image` object is passed inside the `createEvent` request body alongside title, date, displaySettings, etc.

### Poster (Built-in Library)

```json
{
  "image": {
    "source": "partiful_posters",
    "poster": {
      "id": "piscesairbrush.png",
      "name": "piscesairbrush.png",
      "contentType": "image/png",
      "createdAt": "2026-02-20T23:00:36.000Z",
      "version": 1771628441,
      "tags": [],
      "size": 5170580,
      "width": 1600,
      "height": 1600,
      "categories": ["Trending", "Birthday"],
      "url": "https://assets.getpartiful.com/posters/piscesairbrush.png",
      "ordersMap": { "default": 99, "us": 99 },
      "cardOrdersMap": { "default": 99, "us": 99 },
      "bgColor": "#635ba3",
      "blurHash": "eQIr2qaeC6kUgOM#W-ocsEt6LNj]y1WBnfohn+afWUWA6?j?=zahsE"
    },
    "url": "https://assets.getpartiful.com/posters/piscesairbrush.png",
    "blurHash": "eQIr2qaeC6kUgOM#W-ocsEt6LNj]y1WBnfohn+afWUWA6?j?=zahsE",
    "contentType": "image/png",
    "name": "piscesairbrush.png",
    "height": 1600,
    "width": 1600
  }
}
```

### Key Observations

1. **`image.source`** — `"partiful_posters"` for built-in. Likely `"upload"` or `"custom"` for user uploads.
2. **`image.poster`** — Full poster object when using built-in. Contains metadata (tags, categories, dimensions, bgColor).
3. **`image.url`** — Duplicated at top level and inside `poster` object. CDN URL format: `https://assets.getpartiful.com/posters/<id>`
4. **`image.blurHash`** — Used for placeholder rendering. Duplicated at top level.
5. **Poster IDs** — filename-style: `piscesairbrush.png`, `oscars.png`, `st-pat-day`, `movie-awards-spotlight`
6. **Posters hosted at** `https://partiful-posters.imgix.net/<id>?fit=max&w=<width>` (thumbnails) and `https://assets.getpartiful.com/posters/<id>` (full res)

## Poster Library Structure

Posters are fetched client-side and have these fields:

```typescript
interface Poster {
  id: string;           // e.g. "piscesairbrush.png"
  name: string;         // same as id
  contentType: string;  // "image/png"
  createdAt: string;    // ISO timestamp
  version: number;      // unix timestamp
  tags: string[];       // search tags: ["academy awards", "oscar awards", ...]
  size: number;         // bytes
  width: number;        // pixels (usually 1600 or 2160)
  height: number;       // pixels
  categories: string[]; // ["Trending", "Birthday", "Watch Party", "Holiday", ...]
  url: string;          // full-res CDN URL
  ordersMap: { default: number; us: number };     // sort order
  cardOrdersMap: { default: number; us: number }; // card sort order
  bgColor: string;      // hex color for loading state
  blurHash: string;     // blurhash string
}
```

### Categories (from web UI)

- Trending, Birthday, Elegant, Minimal, Dinner Party, Themed, Community Made, Chill, Not Chill, Holiday, College

### Tabs

- **Posters** — static images (PNG)
- **GIFs** — animated (likely same structure but contentType: image/gif)
- **Photos** — stock photos (unknown source, possibly Unsplash integration)

## Custom Image Upload (TODO — needs interception)

The web UI has an "Upload" button and "Upload image" at bottom of picker.
Custom uploads likely:
1. Upload file to Firebase Storage or Partiful's upload endpoint
2. Get back a URL
3. Set `image.source` to something like `"upload"` or `"custom"`
4. Include `image.url`, dimensions, contentType

## Poster Library Source

Posters are likely fetched from Firestore directly (the web app uses Firebase). The collection is probably `posters` in the `getpartiful` Firestore database. This means we could query it via the Firestore REST API with the same auth token.

Alternatively, we could hardcode a poster list endpoint or scrape the gallery.
