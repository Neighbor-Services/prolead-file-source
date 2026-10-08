import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';
import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart';
import 'models.dart';

class ProleadException implements Exception {
  final String message;
  final int statusCode;
  final String? code;

  ProleadException(this.message, {this.statusCode = 500, this.code});

  @override
  String toString() => 'ProleadException($statusCode): $message ${code != null ? '[$code]' : ''}';
}

class ProgressMultipartRequest extends http.MultipartRequest {
  final void Function(int bytesSent, int totalBytes)? onProgress;

  ProgressMultipartRequest(super.method, super.url, {this.onProgress});

  @override
  http.ByteStream finalize() {
    final byteStream = super.finalize();
    if (onProgress == null) return byteStream;

    final total = contentLength;
    int bytesSent = 0;

    final controller = StreamController<List<int>>(sync: true);
    byteStream.listen(
      (chunk) {
        bytesSent += chunk.length;
        onProgress!(bytesSent, total);
        controller.add(chunk);
      },
      onDone: () => controller.close(),
      onError: (err, stack) => controller.addError(err, stack),
      cancelOnError: true,
    );

    return http.ByteStream(controller.stream);
  }
}

class ProleadHttp {
  final String baseUrl;
  final String? apiKey;
  final http.Client _httpClient;

  ProleadHttp({
    required String baseUrl,
    this.apiKey,
    http.Client? httpClient,
  })  : baseUrl = baseUrl.replaceAll(RegExp(r'/+$'), ''),
        _httpClient = httpClient ?? http.Client();

  Map<String, String> get _headers {
    final map = <String, String>{
      'Accept': 'application/json',
      'X-Prolead-Client': 'flutter-sdk-1.0.0',
    };
    if (apiKey != null && apiKey!.isNotEmpty) {
      map['Authorization'] = 'Bearer $apiKey';
    }
    return map;
  }

  Future<dynamic> get(String path, {Map<String, String>? queryParams}) async {
    final uri = Uri.parse('$baseUrl$path').replace(queryParameters: queryParams);
    final resp = await _httpClient.get(uri, headers: _headers);
    return _handleResponse(resp);
  }

  Future<dynamic> post(String path, {Map<String, dynamic>? body, Map<String, String>? queryParams}) async {
    final uri = Uri.parse('$baseUrl$path').replace(queryParameters: queryParams);
    final headers = Map<String, String>.from(_headers);
    headers['Content-Type'] = 'application/json';

    final resp = await _httpClient.post(
      uri,
      headers: headers,
      body: body != null ? jsonEncode(body) : null,
    );
    return _handleResponse(resp);
  }

  Future<dynamic> put(String path, {Map<String, dynamic>? body, Map<String, String>? queryParams}) async {
    final uri = Uri.parse('$baseUrl$path').replace(queryParameters: queryParams);
    final headers = Map<String, String>.from(_headers);
    headers['Content-Type'] = 'application/json';

    final resp = await _httpClient.put(
      uri,
      headers: headers,
      body: body != null ? jsonEncode(body) : null,
    );
    return _handleResponse(resp);
  }

  Future<dynamic> delete(String path, {Map<String, String>? queryParams}) async {
    final uri = Uri.parse('$baseUrl$path').replace(queryParameters: queryParams);
    final resp = await _httpClient.delete(uri, headers: _headers);
    return _handleResponse(resp);
  }

  Future<http.Response> customPost(String path, {Map<String, String>? headers, Object? body}) async {
    final uri = Uri.parse(path.startsWith('http') ? path : '$baseUrl$path');
    final reqHeaders = Map<String, String>.from(_headers);
    if (headers != null) reqHeaders.addAll(headers);
    return _httpClient.post(uri, headers: reqHeaders, body: body);
  }

  Future<http.Response> patchBytes(String url, {required Uint8List bytes, Map<String, String>? headers}) async {
    final uri = Uri.parse(url.startsWith('http') ? url : '$baseUrl$url');
    final reqHeaders = Map<String, String>.from(_headers);
    if (headers != null) reqHeaders.addAll(headers);
    return _httpClient.patch(uri, headers: reqHeaders, body: bytes);
  }

  Future<Uint8List> getBytes(String path, {Map<String, String>? queryParams}) async {
    final uri = Uri.parse(path.startsWith('http') ? path : '$baseUrl$path').replace(queryParameters: queryParams);
    final resp = await _httpClient.get(uri, headers: _headers);
    if (resp.statusCode >= 400) {
      throw ProleadException('Download failed with status ${resp.statusCode}', statusCode: resp.statusCode);
    }
    return resp.bodyBytes;
  }

  Stream<ProleadUploadProgress> uploadBytesStream({
    required String bucket,
    required String path,
    required Uint8List bytes,
    required String filename,
    String? contentType,
    bool isPublic = true,
    int? expiresInSeconds,
    Map<String, String>? metadata,
  }) {
    final streamController = StreamController<ProleadUploadProgress>();

    final uri = Uri.parse('$baseUrl/v0/b/$bucket/o');
    final request = ProgressMultipartRequest('POST', uri, onProgress: (sent, total) {
      final percent = total > 0 ? (sent / total) * 100.0 : 0.0;
      streamController.add(ProleadUploadProgress(
        bytesSent: sent,
        totalBytes: total,
        progressPercent: percent,
        isCompleted: false,
      ));
    });

    if (apiKey != null && apiKey!.isNotEmpty) {
      request.headers['Authorization'] = 'Bearer $apiKey';
    }

    request.fields['path'] = path;
    request.fields['isPublic'] = isPublic.toString();
    if (expiresInSeconds != null) {
      request.fields['expiresInSeconds'] = expiresInSeconds.toString();
    }
    if (metadata != null && metadata.isNotEmpty) {
      request.fields['metadata'] = jsonEncode(metadata);
    }

    MediaType? mediaType;
    if (contentType != null && contentType.contains('/')) {
      final parts = contentType.split('/');
      mediaType = MediaType(parts[0], parts[1]);
    }

    request.files.add(http.MultipartFile.fromBytes(
      'file',
      bytes,
      filename: filename,
      contentType: mediaType,
    ));

    _httpClient.send(request).then((streamedResponse) async {
      final resp = await http.Response.fromStream(streamedResponse);
      if (resp.statusCode >= 400) {
        streamController.addError(ProleadException(
          'Upload failed: ${resp.body}',
          statusCode: resp.statusCode,
        ));
        await streamController.close();
        return;
      }

      final json = jsonDecode(resp.body);
      final obj = ProleadFileObject.fromJson(json);

      streamController.add(ProleadUploadProgress(
        bytesSent: bytes.length,
        totalBytes: bytes.length,
        progressPercent: 100.0,
        isCompleted: true,
        result: obj,
      ));
      await streamController.close();
    }).catchError((err, stack) {
      streamController.addError(err, stack);
      streamController.close();
    });

    return streamController.stream;
  }

  dynamic _handleResponse(http.Response resp) {
    if (resp.statusCode >= 200 && resp.statusCode < 300) {
      if (resp.body.isEmpty) return null;
      try {
        return jsonDecode(resp.body);
      } catch (_) {
        return resp.body;
      }
    }

    String message = 'HTTP ${resp.statusCode}';
    String? code;
    try {
      final decoded = jsonDecode(resp.body);
      if (decoded is Map) {
        if (decoded['error'] is Map) {
          message = decoded['error']['message'] ?? message;
          code = decoded['error']['code'];
        } else if (decoded['error'] is String) {
          message = decoded['error'];
        } else if (decoded['message'] != null) {
          message = decoded['message'];
        }
      }
    } catch (_) {
      if (resp.body.isNotEmpty) {
        message = resp.body;
      }
    }

    throw ProleadException(message, statusCode: resp.statusCode, code: code);
  }

  void close() {
    _httpClient.close();
  }
}
