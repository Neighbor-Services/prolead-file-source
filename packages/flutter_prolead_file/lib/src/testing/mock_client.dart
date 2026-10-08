import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';
import '../models.dart';
import '../client.dart';

/// In-memory mock implementation of [ProleadFileClient] for unit and widget tests.
class MockProleadFileClient implements ProleadFileClient {
  @override
  final String baseUrl;
  @override
  final String? apiKey;

  final Map<String, ProleadBucket> _buckets = {};
  final Map<String, Map<String, _MockFileRecord>> _storage = {};
  final StreamController<ProleadEvent> _eventController = StreamController<ProleadEvent>.broadcast();

  Duration simulatedDelay;

  MockProleadFileClient({
    this.baseUrl = 'http://localhost:8080',
    this.apiKey,
    this.simulatedDelay = Duration.zero,
  }) {
    // Seed a default bucket
    _buckets['default'] = ProleadBucket(
      id: 'b-default',
      name: 'default',
      description: 'Default Bucket',
      isPublic: true,
      maxFileSize: 104857600,
      allowedMimes: const [],
      createdAt: DateTime.now(),
      updatedAt: DateTime.now(),
    );
    _storage['default'] = {};
  }

  Future<void> _delay() async {
    if (simulatedDelay > Duration.zero) {
      await Future.delayed(simulatedDelay);
    }
  }

  @override
  Future<List<ProleadBucket>> listBuckets() async {
    await _delay();
    return _buckets.values.toList();
  }

  @override
  Future<ProleadBucket> getBucket(String bucketName) async {
    await _delay();
    final b = _buckets[bucketName];
    if (b == null) throw Exception('Bucket not found: $bucketName');
    return b;
  }

  @override
  Future<ProleadBucket> createBucket({
    required String name,
    String description = '',
    bool isPublic = true,
    int maxFileSize = 0,
    List<String>? allowedMimes,
  }) async {
    await _delay();
    final normalized = name.trim().toLowerCase();
    if (_buckets.containsKey(normalized)) {
      throw Exception('Bucket already exists: $normalized');
    }
    final bucket = ProleadBucket(
      id: 'b-$normalized',
      name: normalized,
      description: description,
      isPublic: isPublic,
      maxFileSize: maxFileSize,
      allowedMimes: allowedMimes ?? const [],
      createdAt: DateTime.now(),
      updatedAt: DateTime.now(),
    );
    _buckets[normalized] = bucket;
    _storage[normalized] = {};
    return bucket;
  }

  @override
  Future<ProleadBucket> updateBucket(
    String bucketName, {
    String? description,
    bool? isPublic,
    int? maxFileSize,
    List<String>? allowedMimes,
  }) async {
    await _delay();
    final existing = _buckets[bucketName];
    if (existing == null) throw Exception('Bucket not found: $bucketName');
    final updated = ProleadBucket(
      id: existing.id,
      name: existing.name,
      description: description ?? existing.description,
      isPublic: isPublic ?? existing.isPublic,
      maxFileSize: maxFileSize ?? existing.maxFileSize,
      allowedMimes: allowedMimes ?? existing.allowedMimes,
      createdAt: existing.createdAt,
      updatedAt: DateTime.now(),
    );
    _buckets[bucketName] = updated;
    return updated;
  }

  @override
  Future<void> deleteBucket(String bucketName) async {
    await _delay();
    _buckets.remove(bucketName);
    _storage.remove(bucketName);
  }

  @override
  Stream<ProleadUploadProgress> uploadBytesWithProgress({
    String bucket = 'default',
    required String path,
    required Uint8List bytes,
    required String filename,
    String? contentType,
    bool isPublic = true,
    int? expiresInSeconds,
    Map<String, String>? metadata,
  }) async* {
    final total = bytes.lengthInBytes;
    yield ProleadUploadProgress(
      bytesSent: 0,
      totalBytes: total,
      progressPercent: 0.0,
      isCompleted: false,
    );
    await _delay();

    final cleanPath = path.replaceAll(RegExp(r'^/+'), '');
    final fileObj = ProleadFileObject(
      id: 'obj-${DateTime.now().millisecondsSinceEpoch}',
      bucket: bucket,
      path: cleanPath,
      name: filename,
      size: total,
      contentType: contentType ?? 'application/octet-stream',
      sha256: 'mocksha256-${bytes.hashCode}',
      downloadToken: 'mock-token-${DateTime.now().millisecondsSinceEpoch}',
      downloadUrl: '$baseUrl/v0/b/$bucket/o/${Uri.encodeComponent(cleanPath)}?alt=media',
      isPublic: isPublic,
      metadata: metadata ?? {},
      version: 1,
      createdAt: DateTime.now(),
      updatedAt: DateTime.now(),
    );

    if (!_storage.containsKey(bucket)) {
      _storage[bucket] = {};
    }
    _storage[bucket]![cleanPath] = _MockFileRecord(obj: fileObj, data: bytes);

    yield ProleadUploadProgress(
      bytesSent: total,
      totalBytes: total,
      progressPercent: 1.0,
      isCompleted: true,
      result: fileObj,
    );

    _eventController.add(ProleadEvent(
      eventType: 'OBJECT_CREATED',
      bucket: bucket,
      path: cleanPath,
      size: total,
      contentType: fileObj.contentType,
      timestamp: DateTime.now(),
    ));
  }

  @override
  Future<ProleadFileObject> uploadBytes({
    String bucket = 'default',
    required String path,
    required Uint8List bytes,
    required String filename,
    String? contentType,
    bool isPublic = true,
    int? expiresInSeconds,
    Map<String, String>? metadata,
  }) async {
    final progressList = await uploadBytesWithProgress(
      bucket: bucket,
      path: path,
      bytes: bytes,
      filename: filename,
      contentType: contentType,
      isPublic: isPublic,
      expiresInSeconds: expiresInSeconds,
      metadata: metadata,
    ).toList();
    return progressList.last.result!;
  }

  @override
  Future<ProleadFileObject> upload({
    String bucket = 'default',
    required String path,
    required dynamic file,
    String? contentType,
    bool isPublic = true,
    void Function(int sent, int total)? onProgress,
  }) async {
    Uint8List bytes;
    String filename;

    if (file is Uint8List) {
      bytes = file;
      filename = path.split('/').last;
    } else {
      try {
        bytes = (file as dynamic).readAsBytesSync();
        filename = (file.path as String).split(RegExp(r'[/\\]')).last;
      } catch (_) {
        bytes = await (file as dynamic).readAsBytes();
        filename = (file.path as String).split(RegExp(r'[/\\]')).last;
      }
    }

    if (onProgress != null) {
      onProgress(bytes.lengthInBytes, bytes.lengthInBytes);
    }

    return uploadBytes(
      bucket: bucket,
      path: path,
      bytes: bytes,
      filename: filename,
      contentType: contentType,
      isPublic: isPublic,
    );
  }

  @override
  Future<ProleadFileObject> uploadString({
    String bucket = 'default',
    required String path,
    required String content,
    String filename = 'file.txt',
    String contentType = 'text/plain; charset=utf-8',
    bool isPublic = true,
    Map<String, String>? metadata,
  }) {
    final bytes = Uint8List.fromList(utf8.encode(content));
    return uploadBytes(
      bucket: bucket,
      path: path,
      bytes: bytes,
      filename: filename,
      contentType: contentType,
      isPublic: isPublic,
      metadata: metadata,
    );
  }

  @override
  Future<ProleadFileObject> createFolder({
    String bucket = 'default',
    required String folderPath,
  }) {
    final cleanPath = folderPath.replaceAll(RegExp(r'^/+|/+$'), '');
    final markerPath = '$cleanPath/.keep';
    return uploadBytes(
      bucket: bucket,
      path: markerPath,
      bytes: Uint8List(0),
      filename: '.keep',
      contentType: 'application/octet-stream',
      isPublic: true,
    );
  }

  @override
  Future<ProleadFileListResult> listFiles(
    String bucket, {
    String prefix = '',
    String delimiter = '/',
    int limit = 100,
    int offset = 0,
    String search = '',
    bool trashOnly = false,
  }) async {
    await _delay();
    final bucketFiles = _storage[bucket]?.values.map((r) => r.obj).toList() ?? [];
    var filtered = bucketFiles.where((f) {
      if (trashOnly) return f.deletedAt != null;
      if (f.deletedAt != null) return false;
      if (prefix.isNotEmpty && !f.path.startsWith(prefix)) return false;
      if (search.isNotEmpty && !f.path.toLowerCase().contains(search.toLowerCase()) && !f.name.toLowerCase().contains(search.toLowerCase())) {
        return false;
      }
      return true;
    }).toList();

    return ProleadFileListResult(
      items: filtered.skip(offset).take(limit).toList(),
      prefixes: [],
      total: filtered.length,
    );
  }

  @override
  Future<ProleadFileObject> getFileMetadata(String bucket, String path) async {
    await _delay();
    final cleanPath = path.replaceAll(RegExp(r'^/+'), '');
    final record = _storage[bucket]?[cleanPath];
    if (record == null) throw Exception('File not found: $path');
    return record.obj;
  }

  @override
  Future<Uint8List> downloadBytes(String bucket, String path) async {
    await _delay();
    final cleanPath = path.replaceAll(RegExp(r'^/+'), '');
    final record = _storage[bucket]?[cleanPath];
    if (record == null) throw Exception('File not found: $path');
    return record.data;
  }

  @override
  Future<void> deleteFile(String bucket, String path, {bool permanent = false}) async {
    await _delay();
    final cleanPath = path.replaceAll(RegExp(r'^/+'), '');
    if (permanent) {
      _storage[bucket]?.remove(cleanPath);
    } else {
      final record = _storage[bucket]?[cleanPath];
      if (record != null) {
        record.obj = record.obj.copyWith(deletedAt: DateTime.now());
      }
    }
  }

  @override
  Future<ProleadFileObject> restoreFile(String bucket, String path) async {
    await _delay();
    final cleanPath = path.replaceAll(RegExp(r'^/+'), '');
    final record = _storage[bucket]?[cleanPath];
    if (record == null) throw Exception('File not found: $path');
    record.obj = record.obj.copyWith(deletedAt: null);
    return record.obj;
  }

  @override
  Future<ProleadFileObject> rotateToken(String bucket, String path) async {
    await _delay();
    final cleanPath = path.replaceAll(RegExp(r'^/+'), '');
    final record = _storage[bucket]?[cleanPath];
    if (record == null) throw Exception('File not found: $path');
    final updated = record.obj.copyWith(downloadToken: 'token-${DateTime.now().millisecondsSinceEpoch}');
    record.obj = updated;
    return updated;
  }

  @override
  Future<List<ProleadFileVersion>> listVersions(String bucket, String path) async {
    await _delay();
    return [
      ProleadFileVersion(
        id: '00000000-0000-0000-0000-000000000001',
        bucket: bucket,
        path: path,
        version: 1,
        size: 1024,
        sha256: 'mocksha',
        downloadToken: 'token',
        createdAt: DateTime.now(),
      )
    ];
  }

  @override
  Future<ProleadSignedUrl> generateSignedUrl(
    String bucket,
    String path, {
    int durationSeconds = 3600,
  }) async {
    await _delay();
    return ProleadSignedUrl(
      signedUrl: '$baseUrl/v0/b/$bucket/o/${Uri.encodeComponent(path)}?sig=mockHmacSignature&expires=${DateTime.now().add(Duration(seconds: durationSeconds)).millisecondsSinceEpoch}',
      expiresAt: DateTime.now().add(Duration(seconds: durationSeconds)),
    );
  }

  @override
  Future<Uint8List> downloadZip(String bucket, List<String> paths) async {
    await _delay();
    return Uint8List.fromList([80, 75, 3, 4]); // Zip magic header
  }

  @override
  String getTransformedImageUrl(String rawDownloadUrl, ProleadImageTransform options) {
    final uri = Uri.parse(rawDownloadUrl);
    final newParams = Map<String, String>.from(uri.queryParameters);
    newParams.addAll(options.toQueryParams());
    return uri.replace(queryParameters: newParams).toString();
  }

  @override
  Future<ProleadStats> getStats() async {
    await _delay();
    int totalBytes = 0;
    int totalFiles = 0;
    for (final bucketMap in _storage.values) {
      for (final rec in bucketMap.values) {
        totalBytes += rec.data.lengthInBytes;
        totalFiles++;
      }
    }
    return ProleadStats(
      totalFiles: totalFiles,
      totalLogicalBytes: totalBytes,
      totalPhysicalBytes: (totalBytes * 0.75).toInt(),
      totalSavedBytes: (totalBytes * 0.25).toInt(),
      dedupRatio: 1.33,
      activeBuckets: _buckets.length,
      activeVersions: 1,
      trashFiles: 0,
    );
  }

  @override
  Stream<ProleadEvent> listenToEvents() {
    return _eventController.stream;
  }

  @override
  Future<ProleadFileObject> uploadTUS({
    String bucket = 'default',
    required String path,
    required Uint8List bytes,
    int chunkSize = 4 * 1024 * 1024,
    Map<String, String>? metadata,
    void Function(double progress)? onProgress,
  }) async {
    onProgress?.call(1.0);
    return uploadBytes(
      bucket: bucket,
      path: path,
      bytes: bytes,
      filename: path.split('/').last,
      metadata: metadata,
    );
  }

  @override
  Future<ProleadShareLink> createShareLink({
    required String bucket,
    required String path,
    int durationHours = 24,
    String? password,
    int maxDownloads = 0,
  }) async {
    await _delay();
    return ProleadShareLink(
      id: 'mock-share-1',
      bucket: bucket,
      path: path,
      token: 'mock_share_token_123',
      requirePassword: password != null && password.isNotEmpty,
      password: password,
      maxDownloads: maxDownloads,
      downloadCount: 0,
      downloadUrl: '$baseUrl/s/mock_share_token_123',
      createdAt: DateTime.now(),
    );
  }

  @override
  Future<List<ProleadShareLink>> listShares({String? bucket}) async {
    await _delay();
    return [
      ProleadShareLink(
        id: 'mock-share-1',
        bucket: bucket ?? 'default',
        path: 'test.png',
        token: 'mock_share_token_123',
        requirePassword: false,
        maxDownloads: 0,
        downloadCount: 0,
        downloadUrl: '$baseUrl/s/mock_share_token_123',
        createdAt: DateTime.now(),
      ),
    ];
  }

  @override
  Future<void> revokeShare(String token) async {
    await _delay();
  }

  @override
  Future<bool> isHealthy() async {
    await _delay();
    return true;
  }

  @override
  void dispose() {
    _eventController.close();
  }
}

class _MockFileRecord {
  ProleadFileObject obj;
  final Uint8List data;
  _MockFileRecord({required this.obj, required this.data});
}
