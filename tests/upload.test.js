import { describe, it, expect } from 'vitest';
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
    const result = buildUploadImage(uploadData, 'poster.png');
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
