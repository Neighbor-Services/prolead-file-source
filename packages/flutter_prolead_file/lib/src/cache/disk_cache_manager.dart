import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

/// Persistent file-system LRU cache manager for mobile and desktop Flutter apps.
class ProleadDiskCacheManager {
  final Directory cacheDir;
  final int maxBytes;
  final Duration defaultTtl;

  ProleadDiskCacheManager({
    required this.cacheDir,
    this.maxBytes = 250 * 1024 * 1024, // 250MB default
    this.defaultTtl = const Duration(days: 7),
  }) {
    if (!cacheDir.existsSync()) {
      cacheDir.createSync(recursive: true);
    }
  }

  String _hashKey(String key) {
    return base64Url.encode(utf8.encode(key)).replaceAll('=', '');
  }

  File _getFile(String key) {
    return File('${cacheDir.path}/${_hashKey(key)}.cache');
  }

  Future<void> put(String key, Uint8List bytes) async {
    try {
      final file = _getFile(key);
      await file.writeAsBytes(bytes);
      await _evictIfNeeded();
    } catch (_) {}
  }

  Future<Uint8List?> get(String key) async {
    try {
      final file = _getFile(key);
      if (!file.existsSync()) return null;

      final lastModified = file.lastModifiedSync();
      if (DateTime.now().difference(lastModified) > defaultTtl) {
        file.deleteSync();
        return null;
      }

      // Update accessed timestamp
      file.setLastModifiedSync(DateTime.now());
      return await file.readAsBytes();
    } catch (_) {
      return null;
    }
  }

  Future<bool> has(String key) async {
    final file = _getFile(key);
    return file.existsSync();
  }

  Future<void> remove(String key) async {
    try {
      final file = _getFile(key);
      if (file.existsSync()) {
        await file.delete();
      }
    } catch (_) {}
  }

  Future<void> clear() async {
    try {
      if (cacheDir.existsSync()) {
        final files = cacheDir.listSync();
        for (final f in files) {
          if (f is File) f.deleteSync();
        }
      }
    } catch (_) {}
  }

  Future<void> _evictIfNeeded() async {
    try {
      if (!cacheDir.existsSync()) return;

      final entities = cacheDir.listSync().whereType<File>().toList();
      int totalSize = 0;
      for (final f in entities) {
        totalSize += f.lengthSync();
      }

      if (totalSize <= maxBytes) return;

      // Sort by oldest last modified
      entities.sort((a, b) => a.lastModifiedSync().compareTo(b.lastModifiedSync()));

      for (final f in entities) {
        final size = f.lengthSync();
        f.deleteSync();
        totalSize -= size;
        if (totalSize <= maxBytes) break;
      }
    } catch (_) {}
  }
}
