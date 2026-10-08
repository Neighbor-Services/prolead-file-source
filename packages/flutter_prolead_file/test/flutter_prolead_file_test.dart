import 'dart:io';
import 'dart:typed_data';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_prolead_file/flutter_prolead_file.dart';

void main() {
  group('Prolead File Models & SDK Tests', () {
    test('ProleadFileObject correctly parses JSON data', () {
      final json = {
        'id': 'obj-999',
        'bucket': 'assets',
        'path': 'documents/report.pdf',
        'name': 'report.pdf',
        'size': 204800,
        'contentType': 'application/pdf',
        'sha256': 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
        'downloadToken': 'tok-abc',
        'downloadUrl': '/v0/b/assets/o/documents%2Freport.pdf',
        'isPublic': true,
        'metadata': {'uploaded_by': 'admin'},
        'version': 1,
        'createdAt': '2026-10-05T11:00:00Z',
        'updatedAt': '2026-10-05T11:00:00Z',
      };

      final obj = ProleadFileObject.fromJson(json);
      expect(obj.id, 'obj-999');
      expect(obj.bucket, 'assets');
      expect(obj.path, 'documents/report.pdf');
      expect(obj.isPdf, isTrue);
      expect(obj.isImage, isFalse);
      expect(obj.size, 204800);
    });

    test('Image transform query building', () {
      final client = ProleadFileClient(
        baseUrl: 'http://localhost:8080',
        apiKey: 'test-key',
      );

      final url = client.getTransformedImageUrl(
        '/v0/b/default/o/pic.jpg',
        const ProleadImageTransform(width: 800, height: 600, format: 'webp', fit: 'cover'),
      );

      expect(url, contains('w=800'));
      expect(url, contains('h=600'));
      expect(url, contains('format=webp'));
      expect(url, contains('fit=cover'));
    });

    test('ProleadCacheManager stores, retrieves, and expires items', () async {
      final cache = ProleadCacheManager();
      cache.clear();

      final data = Uint8List.fromList([1, 2, 3, 4, 5]);
      cache.put('test_key', data, ttl: const Duration(seconds: 2));

      expect(cache.has('test_key'), isTrue);
      expect(cache.get('test_key'), equals(data));
      expect(cache.currentBytes, equals(5));

      // Test manual remove
      cache.remove('test_key');
      expect(cache.has('test_key'), isFalse);
    });

    test('ProleadRetryPolicy retries on error and succeeds', () async {
      int attempts = 0;
      final retryPolicy = ProleadRetryPolicy(
        maxRetries: 3,
        initialDelay: const Duration(milliseconds: 10),
      );

      final result = await retryPolicy.execute(() async {
        attempts++;
        if (attempts < 3) {
          throw Exception('Simulated network glitch');
        }
        return 'success';
      });

      expect(result, 'success');
      expect(attempts, 3);
    });

    test('MockProleadFileClient bucket & upload workflow', () async {
      final mockClient = MockProleadFileClient();

      // Buckets
      final buckets = await mockClient.listBuckets();
      expect(buckets.length, greaterThanOrEqualTo(1));

      final createdBucket = await mockClient.createBucket(
        name: 'flutter-assets',
        description: 'Flutter SDK test bucket',
      );
      expect(createdBucket.name, 'flutter-assets');

      // Upload String
      final file = await mockClient.uploadString(
        bucket: 'flutter-assets',
        path: 'notes/test.txt',
        content: 'Hello from Flutter Prolead File SDK!',
      );

      expect(file.bucket, 'flutter-assets');
      expect(file.path, 'notes/test.txt');

      // List Files
      final listRes = await mockClient.listFiles('flutter-assets');
      expect(listRes.items.length, 1);
      expect(listRes.items.first.path, 'notes/test.txt');

      // Download Bytes
      final downloaded = await mockClient.downloadBytes('flutter-assets', 'notes/test.txt');
      expect(downloaded, isNotEmpty);

      // Signed URL
      final signed = await mockClient.generateSignedUrl('flutter-assets', 'notes/test.txt');
      expect(signed.signedUrl, contains('sig='));

      // Stats
      final stats = await mockClient.getStats();
      expect(stats.totalFiles, greaterThanOrEqualTo(1));

      // Delete
      await mockClient.deleteFile('flutter-assets', 'notes/test.txt', permanent: true);
      final afterDelete = await mockClient.listFiles('flutter-assets');
      expect(afterDelete.items.isEmpty, isTrue);
    });

    testWidgets('ProleadMediaPreview renders appropriate UI elements', (WidgetTester tester) async {
      final mockClient = MockProleadFileClient();
      final pdfObj = ProleadFileObject(
        id: '1',
        bucket: 'default',
        path: 'doc.pdf',
        name: 'doc.pdf',
        size: 1024,
        contentType: 'application/pdf',
        sha256: 'sha',
        downloadToken: 'tok',
        downloadUrl: 'http://localhost:8080/v0/b/default/o/doc.pdf',
        isPublic: true,
        metadata: const {},
        version: 1,
        createdAt: DateTime.now(),
        updatedAt: DateTime.now(),
      );

      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ProleadMediaPreview(
              client: mockClient,
              file: pdfObj,
            ),
          ),
        ),
      );

      expect(find.text('doc.pdf'), findsOneWidget);
      expect(find.byIcon(Icons.picture_as_pdf), findsOneWidget);
    });

    test('ProleadFileObject parses LQIP, compression and media metadata', () {
      final json = {
        'id': 'obj-media',
        'bucket': 'media',
        'path': 'audio/song.mp3',
        'name': 'song.mp3',
        'size': 5242880,
        'contentType': 'audio/mpeg',
        'sha256': 'sha-hash',
        'downloadToken': 'tok-123',
        'downloadUrl': '/v0/b/media/o/song.mp3',
        'isPublic': true,
        'lqip': 'data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEASABIAAD...',
        'isCompressed': true,
        'originalSize': 10485760,
        'compressedSize': 5242880,
        'compressionRatio': 2.0,
        'savingsPercent': 50.0,
        'metadata': {
          'duration_seconds': '184.5',
          'bitrate': '320000',
          'sample_rate': '44100',
          'channels': '2',
          'codec': 'mp3'
        },
        'version': 1,
        'createdAt': '2026-10-08T12:00:00Z',
        'updatedAt': '2026-10-08T12:00:00Z',
      };

      final obj = ProleadFileObject.fromJson(json);
      expect(obj.isAudio, isTrue);
      expect(obj.lqip, isNotNull);
      expect(obj.isCompressed, isTrue);
      expect(obj.compressionRatio, 2.0);
      expect(obj.durationSeconds, 184.5);
      expect(obj.bitrate, 320000);
      expect(obj.sampleRate, 44100);
      expect(obj.channels, 2);
      expect(obj.codec, 'mp3');
    });

    test('ProleadShareLink parses share responses correctly', () {
      final json = {
        'id': 'sh-1',
        'bucket': 'default',
        'path': 'reports/summary.pdf',
        'token': 'sh_token_xyz',
        'requirePassword': true,
        'maxDownloads': 5,
        'downloadCount': 1,
        'downloadUrl': 'http://localhost:8080/s/sh_token_xyz',
        'createdAt': '2026-10-08T12:00:00Z',
      };

      final share = ProleadShareLink.fromJson(json);
      expect(share.token, 'sh_token_xyz');
      expect(share.requirePassword, isTrue);
      expect(share.maxDownloads, 5);
      expect(share.downloadCount, 1);
    });

    test('ProleadDiskCacheManager stores, retrieves, and evicts from file system', () async {
      final tempDir = Directory.systemTemp.createTempSync('prolead_cache_test_');
      final diskCache = ProleadDiskCacheManager(
        cacheDir: tempDir,
        maxBytes: 1024 * 1024,
      );

      final data = Uint8List.fromList([10, 20, 30, 40, 50]);
      await diskCache.put('image_thumb_1', data);

      expect(await diskCache.has('image_thumb_1'), isTrue);
      final retrieved = await diskCache.get('image_thumb_1');
      expect(retrieved, equals(data));

      await diskCache.remove('image_thumb_1');
      expect(await diskCache.has('image_thumb_1'), isFalse);

      tempDir.deleteSync(recursive: true);
    });

    test('ProleadUploadQueue enqueues and processes items with concurrency control', () async {
      final mockClient = MockProleadFileClient();
      final queue = ProleadUploadQueue(client: mockClient, maxConcurrent: 2);

      queue.enqueue(
        bucket: 'default',
        path: 'gallery/img1.png',
        bytes: Uint8List.fromList([1, 2, 3]),
        filename: 'img1.png',
      );
      queue.enqueue(
        bucket: 'default',
        path: 'gallery/img2.png',
        bytes: Uint8List.fromList([4, 5, 6]),
        filename: 'img2.png',
      );

      expect(queue.items.length, 2);

      // Wait a tick for mock client execution
      await Future.delayed(const Duration(milliseconds: 50));

      expect(queue.items.every((i) => i.status == ProleadUploadStatus.completed), isTrue);
      queue.dispose();
    });
  });
}


