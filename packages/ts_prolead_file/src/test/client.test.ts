import { test, describe } from 'node:test';
import assert from 'node:assert';
import { ProleadFileClient } from '../client.js';

describe('ProleadFileClient TypeScript SDK Tests', () => {
  const client = new ProleadFileClient({
    baseUrl: 'http://localhost:8080',
    apiKey: 'prolead-test-key'
  });

  test('Image transform URL formatting', () => {
    const url = client.getTransformedImageUrl('/v0/b/default/o/avatars%2Fuser.png', {
      width: 600,
      height: 400,
      fit: 'cover',
      format: 'webp',
      quality: 90
    });

    assert.ok(url.includes('w=600'));
    assert.ok(url.includes('h=400'));
    assert.ok(url.includes('fit=cover'));
    assert.ok(url.includes('format=webp'));
    assert.ok(url.includes('q=90'));
  });
});
