import 'dart:async';
import 'dart:convert';
import 'package:http/http.dart' as http;
import 'models.dart';

/// Server-Sent Events listener for real-time Prolead File storage notifications with auto-reconnection.
class ProleadSSEClient {
  final String baseUrl;
  final String? apiKey;
  StreamController<ProleadEvent>? _controller;
  http.Client? _activeClient;
  bool _isDisposed = false;
  int _reconnectAttempts = 0;

  ProleadSSEClient({required this.baseUrl, this.apiKey});

  Stream<ProleadEvent> subscribe() {
    _isDisposed = false;
    _controller = StreamController<ProleadEvent>.broadcast(
      onListen: _connect,
      onCancel: _disconnect,
    );
    return _controller!.stream;
  }

  void _connect() async {
    if (_isDisposed) return;

    _activeClient?.close();
    final client = http.Client();
    _activeClient = client;

    final uri = Uri.parse('${baseUrl.replaceAll(RegExp(r'/+$'), '')}/api/v1/events/stream');
    final req = http.Request('GET', uri);
    req.headers['Accept'] = 'text/event-stream';
    req.headers['Cache-Control'] = 'no-cache';
    if (apiKey != null && apiKey!.isNotEmpty) {
      req.headers['Authorization'] = 'Bearer $apiKey';
    }

    try {
      final streamedResponse = await client.send(req);
      _reconnectAttempts = 0; // Reset on successful handshake

      streamedResponse.stream
          .transform(utf8.decoder)
          .transform(const LineSplitter())
          .listen(
        (line) {
          if (line.startsWith('data:')) {
            final dataStr = line.substring(5).trim();
            if (dataStr.isNotEmpty && dataStr != ':keepalive') {
              try {
                final json = jsonDecode(dataStr);
                final event = ProleadEvent.fromJson(json);
                _controller?.add(event);
              } catch (_) {}
            }
          }
        },
        onError: (err) {
          if (!_isDisposed) {
            _scheduleReconnect();
          }
        },
        onDone: () {
          if (!_isDisposed) {
            _scheduleReconnect();
          }
        },
        cancelOnError: true,
      );
    } catch (e) {
      if (!_isDisposed) {
        _scheduleReconnect();
      }
    }
  }

  void _scheduleReconnect() {
    if (_isDisposed) return;
    _reconnectAttempts++;
    final delaySeconds = _reconnectAttempts > 5 ? 10 : (_reconnectAttempts * 2);
    Future.delayed(Duration(seconds: delaySeconds), () {
      if (!_isDisposed) {
        _connect();
      }
    });
  }

  void _disconnect() {
    _isDisposed = true;
    _activeClient?.close();
    _activeClient = null;
  }
}

