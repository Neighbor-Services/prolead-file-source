import 'dart:async';
import 'dart:typed_data';
import 'models.dart';
import 'client.dart';

enum ProleadUploadStatus { queued, uploading, completed, failed, cancelled }

class ProleadQueueItem {
  final String id;
  final String bucket;
  final String path;
  final Uint8List bytes;
  final String filename;
  final String? contentType;
  final Map<String, String>? metadata;

  ProleadUploadStatus status;
  double progress;
  String? errorMessage;
  ProleadFileObject? result;

  ProleadQueueItem({
    required this.id,
    required this.bucket,
    required this.path,
    required this.bytes,
    required this.filename,
    this.contentType,
    this.metadata,
    this.status = ProleadUploadStatus.queued,
    this.progress = 0.0,
  });
}

/// Stateful multi-file upload queue manager with concurrency control for mobile apps.
class ProleadUploadQueue {
  final ProleadFileClient client;
  final int maxConcurrent;
  final List<ProleadQueueItem> _items = [];
  final StreamController<List<ProleadQueueItem>> _queueController = StreamController<List<ProleadQueueItem>>.broadcast();

  int _activeUploads = 0;
  bool _isPaused = false;

  ProleadUploadQueue({
    required this.client,
    this.maxConcurrent = 3,
  });

  Stream<List<ProleadQueueItem>> get queueStream => _queueController.stream;
  List<ProleadQueueItem> get items => List.unmodifiable(_items);

  void enqueue({
    String bucket = 'default',
    required String path,
    required Uint8List bytes,
    required String filename,
    String? contentType,
    Map<String, String>? metadata,
  }) {
    final item = ProleadQueueItem(
      id: 'q_${DateTime.now().millisecondsSinceEpoch}_${_items.length}',
      bucket: bucket,
      path: path,
      bytes: bytes,
      filename: filename,
      contentType: contentType,
      metadata: metadata,
    );

    _items.add(item);
    _notify();
    _processNext();
  }

  void pause() {
    _isPaused = true;
  }

  void resume() {
    _isPaused = false;
    _processNext();
  }

  void cancel(String id) {
    final item = _items.firstWhere((i) => i.id == id, orElse: () => throw Exception('Item not found'));
    if (item.status == ProleadUploadStatus.queued || item.status == ProleadUploadStatus.uploading) {
      item.status = ProleadUploadStatus.cancelled;
      _notify();
    }
  }

  void clearCompleted() {
    _items.removeWhere((i) => i.status == ProleadUploadStatus.completed || i.status == ProleadUploadStatus.cancelled);
    _notify();
  }

  void _processNext() async {
    if (_isPaused || _activeUploads >= maxConcurrent) return;

    final nextItem = _items.firstWhere(
      (i) => i.status == ProleadUploadStatus.queued,
      orElse: () => ProleadQueueItem(id: '', bucket: '', path: '', bytes: Uint8List(0), filename: ''),
    );

    if (nextItem.id.isEmpty) return;

    nextItem.status = ProleadUploadStatus.uploading;
    _activeUploads++;
    _notify();

    try {
      final res = await client.uploadBytes(
        bucket: nextItem.bucket,
        path: nextItem.path,
        bytes: nextItem.bytes,
        filename: nextItem.filename,
        contentType: nextItem.contentType,
        metadata: nextItem.metadata,
      );

      nextItem.status = ProleadUploadStatus.completed;
      nextItem.progress = 1.0;
      nextItem.result = res;
    } catch (e) {
      nextItem.status = ProleadUploadStatus.failed;
      nextItem.errorMessage = e.toString();
    } finally {
      _activeUploads--;
      _notify();
      _processNext();
    }
  }

  void _notify() {
    if (!_queueController.isClosed) {
      _queueController.add(List.unmodifiable(_items));
    }
  }

  void dispose() {
    _queueController.close();
  }
}
