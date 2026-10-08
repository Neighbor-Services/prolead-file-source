import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';
import 'models.dart';
import 'http_client.dart';
import 'sse_client.dart';

/// The official Prolead File client SDK for Flutter and Dart.
class ProleadFileClient {
  final String baseUrl;
  final String? apiKey;
  final ProleadHttp _http;

  ProleadFileClient({
    required this.baseUrl,
    this.apiKey,
  }) : _http = ProleadHttp(baseUrl: baseUrl, apiKey: apiKey);

  // ---------------------------------------------------------------------------
  // BUCKET OPERATIONS
  // ---------------------------------------------------------------------------

  /// List all storage buckets.
  Future<List<ProleadBucket>> listBuckets() async {
    final resp = await _http.get('/api/v1/buckets');
    if (resp is List) {
      return resp.map((e) => ProleadBucket.fromJson(e)).toList();
    }
    return [];
  }

  /// Get details and quotas of a specific bucket.
  Future<ProleadBucket> getBucket(String bucketName) async {
    final resp = await _http.get('/api/v1/buckets/$bucketName');
    return ProleadBucket.fromJson(resp);
  }

  /// Create a new bucket.
  Future<ProleadBucket> createBucket({
    required String name,
    String description = '',
    bool isPublic = true,
    int maxFileSize = 0,
    List<String>? allowedMimes,
  }) async {
    final body = {
      'name': name.trim().toLowerCase(),
      'description': description,
      'isPublic': isPublic,
      'maxFileSize': maxFileSize,
      if (allowedMimes != null) 'allowedMimes': allowedMimes,
    };
    final resp = await _http.post('/api/v1/buckets', body: body);
    return ProleadBucket.fromJson(resp);
  }

  /// Update bucket settings & quotas.
  Future<ProleadBucket> updateBucket(
    String bucketName, {
    String? description,
    bool? isPublic,
    int? maxFileSize,
    List<String>? allowedMimes,
  }) async {
    final body = <String, dynamic>{};
    if (description != null) body['description'] = description;
    if (isPublic != null) body['isPublic'] = isPublic;
    if (maxFileSize != null) body['maxFileSize'] = maxFileSize;
    if (allowedMimes != null) body['allowedMimes'] = allowedMimes;

    final resp = await _http.put('/api/v1/buckets/$bucketName', body: body);
    return ProleadBucket.fromJson(resp);
  }

  /// Delete a bucket.
  Future<void> deleteBucket(String bucketName) async {
    await _http.delete('/api/v1/buckets/$bucketName');
  }

  // ---------------------------------------------------------------------------
  // FILE & OBJECT OPERATIONS
  // ---------------------------------------------------------------------------

  /// Upload raw bytes with live streaming progress updates.
  Stream<ProleadUploadProgress> uploadBytesWithProgress({
    String bucket = 'default',
    required String path,
    required Uint8List bytes,
    required String filename,
    String? contentType,
    bool isPublic = true,
    int? expiresInSeconds,
    Map<String, String>? metadata,
  }) {
    return _http.uploadBytesStream(
      bucket: bucket,
      path: path,
      bytes: bytes,
      filename: filename,
      contentType: contentType,
      isPublic: isPublic,
      expiresInSeconds: expiresInSeconds,
      metadata: metadata,
    );
  }

  /// Upload raw bytes returning a Future of the stored [ProleadFileObject].
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
    final completer = Completer<ProleadFileObject>();
    uploadBytesWithProgress(
      bucket: bucket,
      path: path,
      bytes: bytes,
      filename: filename,
      contentType: contentType,
      isPublic: isPublic,
      expiresInSeconds: expiresInSeconds,
      metadata: metadata,
    ).listen(
      (event) {
        if (event.isCompleted && event.result != null) {
          completer.complete(event.result!);
        }
      },
      onError: (err) => completer.completeError(err),
    );
    return completer.future;
  }

  /// Upload a File, raw bytes, or binary payload with optional progress callback.
  Future<ProleadFileObject> upload({
    String bucket = 'default',
    required String path,
    required dynamic file, // File (dart:io) or Uint8List
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
      final completer = Completer<ProleadFileObject>();
      uploadBytesWithProgress(
        bucket: bucket,
        path: path,
        bytes: bytes,
        filename: filename,
        contentType: contentType,
        isPublic: isPublic,
      ).listen(
        (event) {
          onProgress(event.bytesSent, event.totalBytes);
          if (event.isCompleted && event.result != null) {
            completer.complete(event.result!);
          }
        },
        onError: (err) => completer.completeError(err),
      );
      return completer.future;
    } else {
      return uploadBytes(
        bucket: bucket,
        path: path,
        bytes: bytes,
        filename: filename,
        contentType: contentType,
        isPublic: isPublic,
      );
    }
  }

  /// Convenience method to upload a UTF-8 string as a file.
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

  /// Creates a directory marker (.keep) to materialize a folder prefix in the bucket.
  Future<ProleadFileObject> createFolder({
    String bucket = 'default',
    required String folderPath,
  }) {
    final cleanPath = folderPath.replaceAll(RegExp(r'^/+|/+$'), '');
    final markerPath = '$cleanPath/.keep';
    final emptyBytes = Uint8List(0);
    return uploadBytes(
      bucket: bucket,
      path: markerPath,
      bytes: emptyBytes,
      filename: '.keep',
      contentType: 'application/octet-stream',
      isPublic: true,
    );
  }

  /// List files in a bucket with hierarchical prefix filtering and search.
  Future<ProleadFileListResult> listFiles(
    String bucket, {
    String prefix = '',
    String delimiter = '/',
    int limit = 100,
    int offset = 0,
    String search = '',
    bool trashOnly = false,
  }) async {
    final query = <String, String>{
      if (prefix.isNotEmpty) 'prefix': prefix,
      if (delimiter.isNotEmpty) 'delimiter': delimiter,
      'limit': limit.toString(),
      'offset': offset.toString(),
      if (search.isNotEmpty) 'search': search,
      if (trashOnly) 'trash': 'true',
    };

    final resp = await _http.get('/api/v1/b/$bucket/o', queryParams: query);
    return ProleadFileListResult.fromJson(resp);
  }

  /// Get metadata for a specific file object.
  Future<ProleadFileObject> getFileMetadata(String bucket, String path) async {
    final cleanPath = Uri.encodeComponent(path);
    final resp = await _http.get('/api/v1/b/$bucket/o/$cleanPath');
    return ProleadFileObject.fromJson(resp);
  }

  /// Download file bytes directly.
  Future<Uint8List> downloadBytes(String bucket, String path) async {
    final cleanPath = Uri.encodeComponent(path);
    return _http.getBytes('/v0/b/$bucket/o/$cleanPath?alt=media');
  }

  /// Delete or move file to trash.
  Future<void> deleteFile(String bucket, String path, {bool permanent = false}) async {
    final cleanPath = Uri.encodeComponent(path);
    final query = permanent ? {'permanent': 'true'} : null;
    await _http.delete('/api/v1/b/$bucket/o/$cleanPath', queryParams: query);
  }

  /// Restore a deleted file from trash.
  Future<ProleadFileObject> restoreFile(String bucket, String path) async {
    final cleanPath = Uri.encodeComponent(path);
    final resp = await _http.post('/api/v1/b/$bucket/o/$cleanPath/restore');
    return ProleadFileObject.fromJson(resp);
  }

  /// Rotate security download token.
  Future<ProleadFileObject> rotateToken(String bucket, String path) async {
    final cleanPath = Uri.encodeComponent(path);
    final resp = await _http.post('/api/v1/b/$bucket/o/$cleanPath/rotate-token');
    return ProleadFileObject.fromJson(resp);
  }

  /// List all historical revisions of a file.
  Future<List<ProleadFileVersion>> listVersions(String bucket, String path) async {
    final cleanPath = Uri.encodeComponent(path);
    final resp = await _http.get('/api/v1/b/$bucket/o/$cleanPath/versions');
    if (resp is List) {
      return resp.map((e) => ProleadFileVersion.fromJson(e)).toList();
    }
    return [];
  }

  /// Generate a time-limited HMAC-signed URL for direct secure download.
  Future<ProleadSignedUrl> generateSignedUrl(
    String bucket,
    String path, {
    int durationSeconds = 3600,
  }) async {
    final resp = await _http.post(
      '/api/v1/b/$bucket/sign-url',
      body: {'path': path, 'durationSeconds': durationSeconds},
    );
    return ProleadSignedUrl.fromJson(resp);
  }

  /// Download multiple files combined into a ZIP archive.
  Future<Uint8List> downloadZip(String bucket, List<String> paths) async {
    return _http.getBytes('/api/v1/b/$bucket/download-zip');
  }

  // ---------------------------------------------------------------------------
  // RESUMABLE TUS 1.0.0 UPLOAD
  // ---------------------------------------------------------------------------

  /// Upload large files via resumable TUS 1.0.0 protocol.
  Future<ProleadFileObject> uploadTUS({
    String bucket = 'default',
    required String path,
    required Uint8List bytes,
    int chunkSize = 4 * 1024 * 1024,
    Map<String, String>? metadata,
    void Function(double progress)? onProgress,
  }) async {
    final filename = path.split('/').last;
    final metaParts = <String>[
      'bucket ${base64Url.encode(utf8.encode(bucket))}',
      'path ${base64Url.encode(utf8.encode(path))}',
      'filename ${base64Url.encode(utf8.encode(filename))}',
    ];
    if (metadata != null) {
      metadata.forEach((k, v) {
        metaParts.add('$k ${base64Url.encode(utf8.encode(v))}');
      });
    }

    // 1. Create TUS upload
    final createResp = await _http.customPost(
      '/api/v1/tus/upload',
      headers: {
        'Tus-Resumable': '1.0.0',
        'Upload-Length': bytes.length.toString(),
        'Upload-Metadata': metaParts.join(','),
      },
    );

    final location = createResp.headers['location'] ?? createResp.headers['Location'];
    if (location == null) {
      throw Exception('TUS server did not return Location header');
    }

    final uploadUrl = location.startsWith('http')
        ? location
        : '${baseUrl.replaceAll(RegExp(r'/+$'), '')}${location.startsWith('/') ? '' : '/'}$location';

    // 2. Stream Chunks with PATCH
    int offset = 0;
    while (offset < bytes.length) {
      final end = (offset + chunkSize < bytes.length) ? offset + chunkSize : bytes.length;
      final chunk = bytes.sublist(offset, end);

      await _http.patchBytes(
        uploadUrl,
        bytes: chunk,
        headers: {
          'Tus-Resumable': '1.0.0',
          'Upload-Offset': offset.toString(),
          'Content-Type': 'application/offset+octet-stream',
        },
      );

      offset = end;
      onProgress?.call(offset / bytes.length);
    }

    return getFileMetadata(bucket, path);
  }

  // ---------------------------------------------------------------------------
  // SHARE LINKS
  // ---------------------------------------------------------------------------

  /// Create a public share link.
  Future<ProleadShareLink> createShareLink({
    required String bucket,
    required String path,
    int durationHours = 24,
    String? password,
    int maxDownloads = 0,
  }) async {
    final resp = await _http.post(
      '/api/v1/shares',
      body: {
        'bucket': bucket,
        'path': path,
        'durationHours': durationHours,
        if (password != null) 'password': password,
        'maxDownloads': maxDownloads,
      },
    );
    return ProleadShareLink.fromJson(resp);
  }

  /// List public share links for a bucket.
  Future<List<ProleadShareLink>> listShares({String? bucket}) async {
    final query = bucket != null ? {'bucket': bucket} : null;
    final resp = await _http.get('/api/v1/shares', queryParams: query);
    if (resp is List) {
      return resp.map((e) => ProleadShareLink.fromJson(e)).toList();
    }
    return [];
  }

  /// Revoke a public share link.
  Future<void> revokeShare(String token) async {
    await _http.delete('/api/v1/shares/$token');
  }

  // ---------------------------------------------------------------------------
  // IMAGE TRANSFORMATION ENGINE
  // ---------------------------------------------------------------------------

  /// Build transformed image URL with dynamic resizing, format transcoding and quality parameters.
  String getTransformedImageUrl(String rawDownloadUrl, ProleadImageTransform options) {
    final uri = Uri.parse(rawDownloadUrl.startsWith('http')
        ? rawDownloadUrl
        : '${baseUrl.replaceAll(RegExp(r'/+$'), '')}$rawDownloadUrl');
    
    final newParams = Map<String, String>.from(uri.queryParameters);
    newParams.addAll(options.toQueryParams());
    if (apiKey != null && apiKey!.isNotEmpty && !newParams.containsKey('key')) {
      newParams['key'] = apiKey!;
    }
    return uri.replace(queryParameters: newParams).toString();
  }

  // ---------------------------------------------------------------------------
  // TELEMETRY & SSE
  // ---------------------------------------------------------------------------

  /// Fetch overall storage telemetry & deduplication stats.
  Future<ProleadStats> getStats() async {
    final resp = await _http.get('/api/v1/stats');
    return ProleadStats.fromJson(resp);
  }

  /// Subscribe to real-time storage events via Server-Sent Events (SSE).
  Stream<ProleadEvent> listenToEvents() {
    final sse = ProleadSSEClient(baseUrl: baseUrl, apiKey: apiKey);
    return sse.subscribe();
  }

  /// Check server health status.
  Future<bool> isHealthy() async {
    try {
      final resp = await _http.get('/api/v1/health');
      return resp != null && (resp['status'] == 'ok' || resp['status'] == 'healthy');
    } catch (_) {
      return false;
    }
  }

  void dispose() {
    _http.close();
  }
}
