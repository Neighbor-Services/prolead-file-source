import 'dart:typed_data';

/// In-memory LRU cache manager for Prolead File with TTL and memory limits.
class ProleadCacheManager {
  static final ProleadCacheManager _instance = ProleadCacheManager._internal();
  factory ProleadCacheManager() => _instance;
  ProleadCacheManager._internal();

  final Map<String, _CacheEntry> _cache = {};
  int _maxBytes = 50 * 1024 * 1024; // 50MB default limit
  int _currentBytes = 0;
  Duration defaultTtl = const Duration(minutes: 30);

  void setMaxBytes(int bytes) {
    _maxBytes = bytes;
    _evictIfNeeded();
  }

  void put(String key, Uint8List data, {Duration? ttl}) {
    final entrySize = data.lengthInBytes;
    if (entrySize > _maxBytes) return; // Cannot fit single item

    // Remove existing if any
    remove(key);

    _cache[key] = _CacheEntry(
      data: data,
      expiresAt: DateTime.now().add(ttl ?? defaultTtl),
      lastAccessed: DateTime.now(),
    );
    _currentBytes += entrySize;
    _evictIfNeeded();
  }

  Uint8List? get(String key) {
    final entry = _cache[key];
    if (entry == null) return null;

    if (DateTime.now().isAfter(entry.expiresAt)) {
      remove(key);
      return null;
    }

    entry.lastAccessed = DateTime.now();
    return entry.data;
  }

  bool has(String key) {
    return get(key) != null;
  }

  void remove(String key) {
    final entry = _cache.remove(key);
    if (entry != null) {
      _currentBytes -= entry.data.lengthInBytes;
    }
  }

  void clear() {
    _cache.clear();
    _currentBytes = 0;
  }

  int get count => _cache.length;
  int get currentBytes => _currentBytes;

  void _evictIfNeeded() {
    if (_currentBytes <= _maxBytes) return;

    // Evict expired first
    final now = DateTime.now();
    final expiredKeys = _cache.entries
        .where((e) => now.isAfter(e.value.expiresAt))
        .map((e) => e.key)
        .toList();

    for (final key in expiredKeys) {
      remove(key);
    }

    // If still exceeds, evict LRU
    if (_currentBytes > _maxBytes) {
      final sortedEntries = _cache.entries.toList()
        ..sort((a, b) => a.value.lastAccessed.compareTo(b.value.lastAccessed));

      for (final entry in sortedEntries) {
        remove(entry.key);
        if (_currentBytes <= _maxBytes) break;
      }
    }
  }
}

class _CacheEntry {
  final Uint8List data;
  final DateTime expiresAt;
  DateTime lastAccessed;

  _CacheEntry({
    required this.data,
    required this.expiresAt,
    required this.lastAccessed,
  });
}
