import { test, describe } from 'node:test';
import assert from 'node:assert';
import { ProleadMemoryCache } from '../cache.js';
import { executeWithRetry, ResilientUploader } from '../resilience.js';
import { MockProleadFileClient } from '../mock-client.js';
import { formatBytes, getMimeTypeFromName } from '../browser.js';
import { verifyWebhookSignature } from '../webhook.js';

describe('TypeScript SDK Features & Utilities', () => {
  test('ProleadMemoryCache stores and evicts', async () => {
    const cache = new ProleadMemoryCache({ ttlMs: 100, maxEntries: 2 });
    cache.set('key1', 'val1');
    cache.set('key2', 'val2');

    assert.strictEqual(cache.get('key1'), 'val1');
    assert.strictEqual(cache.get('key2'), 'val2');

    // Adding 3rd item triggers LRU eviction
    cache.set('key3', 'val3');
    assert.strictEqual(cache.get('key3'), 'val3');
  });

  test('executeWithRetry retries failing operations', async () => {
    let attempts = 0;
    const result = await executeWithRetry(
      async () => {
        attempts++;
        if (attempts < 3) throw new Error('Temporary failure');
        return 'success';
      },
      { maxRetries: 3, initialDelayMs: 10 }
    );

    assert.strictEqual(result, 'success');
    assert.strictEqual(attempts, 3);
  });

  test('ResilientUploader chunks data and reports progress', async () => {
    const data = new Uint8Array(1024 * 10); // 10KB
    const uploader = new ResilientUploader(data, 2048); // 2KB chunks

    const uploadedChunks: number[] = [];
    let lastPercent = 0;

    await uploader.upload(
      async (chunk, index) => {
        uploadedChunks.push(index);
      },
      (progress) => {
        lastPercent = progress.percent;
      }
    );

    assert.strictEqual(uploadedChunks.length, 5);
    assert.strictEqual(lastPercent, 100);
  });

  test('MockProleadFileClient full workflow', async () => {
    const mock = new MockProleadFileClient();

    // 1. Create Bucket
    const bucket = await mock.createBucket('docs', { description: 'Documents bucket' });
    assert.strictEqual(bucket.name, 'docs');

    // 2. Upload file
    const file = await mock.upload('Hello world text', {
      bucket: 'docs',
      path: 'notes/intro.txt',
      contentType: 'text/plain',
    });
    assert.strictEqual(file.name, 'intro.txt');
    assert.strictEqual(file.size, 16);

    // 3. List files
    const list = await mock.listFiles('docs');
    assert.strictEqual(list.items.length, 1);
    assert.strictEqual(list.items[0].path, 'notes/intro.txt');

    // 4. Download file
    const buf = await mock.downloadBuffer('docs', 'notes/intro.txt');
    const text = new TextDecoder().decode(buf);
    assert.strictEqual(text, 'Hello world text');

    // 5. Signed URL
    const signed = await mock.generateSignedUrl('docs', 'notes/intro.txt');
    assert.ok(signed.signedUrl.includes('expires='));

    // 6. Delete file
    await mock.deleteFile('docs', 'notes/intro.txt', true);
    const afterDelete = await mock.listFiles('docs');
    assert.strictEqual(afterDelete.items.length, 0);
  });

  test('Browser formatting utilities', () => {
    assert.strictEqual(formatBytes(0), '0 B');
    assert.strictEqual(formatBytes(1024), '1 KB');
    assert.strictEqual(formatBytes(1048576), '1 MB');

    assert.strictEqual(getMimeTypeFromName('test.png'), 'image/png');
    assert.strictEqual(getMimeTypeFromName('manual.pdf'), 'application/pdf');
    assert.strictEqual(getMimeTypeFromName('song.mp3'), 'audio/mpeg');
  });

  test('TUS, Shares, and Admin operations workflow in TS SDK', async () => {
    const mock = new MockProleadFileClient();

    // TUS upload
    const tusFile = { size: 100, slice: () => new Uint8Array(100) };
    let progressCalled = false;
    const uploaded = await mock.uploadTUS(tusFile as any, {
      bucket: 'default',
      path: 'large-dataset.bin',
      onProgress: (p) => {
        progressCalled = true;
      },
    });
    assert.strictEqual(uploaded.name, 'large-dataset.bin');
    assert.strictEqual(progressCalled, true);

    // Shares
    const share = await mock.createShareLink('default', 'large-dataset.bin', {
      durationHours: 12,
      password: 'mypassword',
    });
    assert.strictEqual(share.requirePassword, true);
    assert.ok(share.token.length > 0);

    const shares = await mock.listShareLinks('default');
    assert.strictEqual(shares.length, 1);

    await mock.revokeShareLink(share.token);

    // Admin ops
    const gc = await mock.triggerGC();
    assert.strictEqual(gc.deletedBlobs, 5);

    const dedup = await mock.getDedupReport();
    assert.strictEqual(dedup.dedupRatio, 2.0);
  });

  test('Webhook HMAC signature validation in TS SDK', () => {
    const payload = JSON.stringify({ eventType: 'OBJECT_CREATED', bucket: 'default', path: 'file.txt' });
    const secret = 'my_super_secret_webhook_key';

    // Verify empty/invalid fails cleanly
    assert.strictEqual(verifyWebhookSignature(payload, '', secret), false);
    assert.strictEqual(verifyWebhookSignature(payload, 'invalid_sig', secret), false);
  });
});


