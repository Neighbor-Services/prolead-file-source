import 'dart:convert';

/// Represents a stored file object within Prolead File storage appliance.
class ProleadFileObject {
  final String id;
  final String bucket;
  final String path;
  final String name;
  final int size;
  final String contentType;
  final String sha256;
  final String downloadToken;
  final String downloadUrl;
  final bool isPublic;
  final DateTime? expiresAt;
  final DateTime? deletedAt;
  final Map<String, dynamic> metadata;
  final int version;
  final String? lqip;
  final bool isCompressed;
  final int? originalSize;
  final int? compressedSize;
  final double? compressionRatio;
  final double? savingsPercent;
  final DateTime createdAt;
  final DateTime updatedAt;

  ProleadFileObject({
    required this.id,
    required this.bucket,
    required this.path,
    required this.name,
    required this.size,
    required this.contentType,
    required this.sha256,
    required this.downloadToken,
    required this.downloadUrl,
    required this.isPublic,
    this.expiresAt,
    this.deletedAt,
    required this.metadata,
    required this.version,
    this.lqip,
    this.isCompressed = false,
    this.originalSize,
    this.compressedSize,
    this.compressionRatio,
    this.savingsPercent,
    required this.createdAt,
    required this.updatedAt,
  });

  ProleadFileObject copyWith({
    String? id,
    String? bucket,
    String? path,
    String? name,
    int? size,
    String? contentType,
    String? sha256,
    String? downloadToken,
    String? downloadUrl,
    bool? isPublic,
    DateTime? expiresAt,
    DateTime? deletedAt,
    Map<String, dynamic>? metadata,
    int? version,
    String? lqip,
    bool? isCompressed,
    int? originalSize,
    int? compressedSize,
    double? compressionRatio,
    double? savingsPercent,
    DateTime? createdAt,
    DateTime? updatedAt,
  }) {
    return ProleadFileObject(
      id: id ?? this.id,
      bucket: bucket ?? this.bucket,
      path: path ?? this.path,
      name: name ?? this.name,
      size: size ?? this.size,
      contentType: contentType ?? this.contentType,
      sha256: sha256 ?? this.sha256,
      downloadToken: downloadToken ?? this.downloadToken,
      downloadUrl: downloadUrl ?? this.downloadUrl,
      isPublic: isPublic ?? this.isPublic,
      expiresAt: expiresAt ?? this.expiresAt,
      deletedAt: deletedAt ?? this.deletedAt,
      metadata: metadata ?? this.metadata,
      version: version ?? this.version,
      lqip: lqip ?? this.lqip,
      isCompressed: isCompressed ?? this.isCompressed,
      originalSize: originalSize ?? this.originalSize,
      compressedSize: compressedSize ?? this.compressedSize,
      compressionRatio: compressionRatio ?? this.compressionRatio,
      savingsPercent: savingsPercent ?? this.savingsPercent,
      createdAt: createdAt ?? this.createdAt,
      updatedAt: updatedAt ?? this.updatedAt,
    );
  }

  bool get isImage => contentType.toLowerCase().startsWith('image/');
  bool get isVideo => contentType.toLowerCase().startsWith('video/');
  bool get isAudio => contentType.toLowerCase().startsWith('audio/');
  bool get isPdf => contentType.toLowerCase() == 'application/pdf';

  // Media characteristics
  double? get durationSeconds => double.tryParse(metadata['duration_seconds']?.toString() ?? '');
  int? get bitrate => int.tryParse(metadata['bitrate']?.toString() ?? '');
  int? get sampleRate => int.tryParse(metadata['sample_rate']?.toString() ?? '');
  int? get channels => int.tryParse(metadata['channels']?.toString() ?? '');
  String? get codec => metadata['codec']?.toString();

  factory ProleadFileObject.fromJson(Map<String, dynamic> json) {
    Map<String, dynamic> parsedMeta = {};
    if (json['metadata'] != null) {
      if (json['metadata'] is Map) {
        parsedMeta = Map<String, dynamic>.from(json['metadata']);
      } else if (json['metadata'] is String && (json['metadata'] as String).isNotEmpty) {
        try {
          parsedMeta = Map<String, dynamic>.from(jsonDecode(json['metadata']));
        } catch (_) {}
      }
    }

    return ProleadFileObject(
      id: json['id'] ?? '',
      bucket: json['bucket'] ?? 'default',
      path: json['path'] ?? '',
      name: json['name'] ?? '',
      size: (json['size'] as num?)?.toInt() ?? 0,
      contentType: json['contentType'] ?? json['content_type'] ?? 'application/octet-stream',
      sha256: json['sha256'] ?? '',
      downloadToken: json['downloadToken'] ?? json['download_token'] ?? '',
      downloadUrl: json['downloadUrl'] ?? json['download_url'] ?? '',
      isPublic: json['isPublic'] ?? json['is_public'] ?? true,
      expiresAt: json['expiresAt'] != null || json['expires_at'] != null
          ? DateTime.tryParse(json['expiresAt'] ?? json['expires_at'])
          : null,
      deletedAt: json['deletedAt'] != null || json['deleted_at'] != null
          ? DateTime.tryParse(json['deletedAt'] ?? json['deleted_at'])
          : null,
      metadata: parsedMeta,
      version: (json['version'] as num?)?.toInt() ?? 1,
      lqip: json['lqip'],
      isCompressed: json['isCompressed'] ?? json['is_compressed'] ?? false,
      originalSize: (json['originalSize'] ?? json['original_size']) as int?,
      compressedSize: (json['compressedSize'] ?? json['compressed_size']) as int?,
      compressionRatio: (json['compressionRatio'] ?? json['compression_ratio'] as num?)?.toDouble(),
      savingsPercent: (json['savingsPercent'] ?? json['savings_percent'] as num?)?.toDouble(),
      createdAt: json['createdAt'] != null || json['created_at'] != null
          ? DateTime.tryParse(json['createdAt'] ?? json['created_at']) ?? DateTime.now()
          : DateTime.now(),
      updatedAt: json['updatedAt'] != null || json['updated_at'] != null
          ? DateTime.tryParse(json['updatedAt'] ?? json['updated_at']) ?? DateTime.now()
          : DateTime.now(),
    );
  }

  Map<String, dynamic> toJson() => {
    'id': id,
    'bucket': bucket,
    'path': path,
    'name': name,
    'size': size,
    'contentType': contentType,
    'sha256': sha256,
    'downloadToken': downloadToken,
    'downloadUrl': downloadUrl,
    'isPublic': isPublic,
    'expiresAt': expiresAt?.toIso8601String(),
    'deletedAt': deletedAt?.toIso8601String(),
    'metadata': metadata,
    'version': version,
    'lqip': lqip,
    'isCompressed': isCompressed,
    'originalSize': originalSize,
    'compressedSize': compressedSize,
    'compressionRatio': compressionRatio,
    'savingsPercent': savingsPercent,
    'createdAt': createdAt.toIso8601String(),
    'updatedAt': updatedAt.toIso8601String(),
  };
}

/// Public share link reference.
class ProleadShareLink {
  final String id;
  final String bucket;
  final String path;
  final String token;
  final bool requirePassword;
  final String? password;
  final int maxDownloads;
  final int downloadCount;
  final DateTime? expiresAt;
  final String downloadUrl;
  final DateTime createdAt;

  ProleadShareLink({
    required this.id,
    required this.bucket,
    required this.path,
    required this.token,
    required this.requirePassword,
    this.password,
    required this.maxDownloads,
    required this.downloadCount,
    this.expiresAt,
    required this.downloadUrl,
    required this.createdAt,
  });

  factory ProleadShareLink.fromJson(Map<String, dynamic> json) {
    return ProleadShareLink(
      id: json['id'] ?? '',
      bucket: json['bucket'] ?? '',
      path: json['path'] ?? '',
      token: json['token'] ?? '',
      requirePassword: json['requirePassword'] ?? json['require_password'] ?? false,
      password: json['password'],
      maxDownloads: (json['maxDownloads'] ?? json['max_downloads'] ?? 0) as int,
      downloadCount: (json['downloadCount'] ?? json['download_count'] ?? 0) as int,
      expiresAt: json['expiresAt'] != null ? DateTime.tryParse(json['expiresAt']) : null,
      downloadUrl: json['downloadUrl'] ?? json['download_url'] ?? '',
      createdAt: json['createdAt'] != null ? DateTime.tryParse(json['createdAt']) ?? DateTime.now() : DateTime.now(),
    );
  }
}

/// Bucket configuration & quotas.
class ProleadBucket {
  final String id;
  final String name;
  final String description;
  final bool isPublic;
  final int maxFileSize;
  final List<String> allowedMimes;
  final DateTime createdAt;
  final DateTime updatedAt;

  ProleadBucket({
    required this.id,
    required this.name,
    required this.description,
    required this.isPublic,
    required this.maxFileSize,
    required this.allowedMimes,
    required this.createdAt,
    required this.updatedAt,
  });

  factory ProleadBucket.fromJson(Map<String, dynamic> json) {
    List<String> mimes = [];
    if (json['allowedMimes'] is List) {
      mimes = List<String>.from(json['allowedMimes']);
    } else if (json['allowed_mimes'] is List) {
      mimes = List<String>.from(json['allowed_mimes']);
    }

    return ProleadBucket(
      id: json['id'] ?? '',
      name: json['name'] ?? '',
      description: json['description'] ?? '',
      isPublic: json['isPublic'] ?? json['is_public'] ?? true,
      maxFileSize: (json['maxFileSize'] ?? json['max_file_size'] as num?)?.toInt() ?? 0,
      allowedMimes: mimes,
      createdAt: json['createdAt'] != null || json['created_at'] != null
          ? DateTime.tryParse(json['createdAt'] ?? json['created_at']) ?? DateTime.now()
          : DateTime.now(),
      updatedAt: json['updatedAt'] != null || json['updated_at'] != null
          ? DateTime.tryParse(json['updatedAt'] ?? json['updated_at']) ?? DateTime.now()
          : DateTime.now(),
    );
  }
}

/// File revision snapshot.
class ProleadFileVersion {
  final String id;
  final String bucket;
  final String path;
  final int version;
  final int size;
  final String sha256;
  final String downloadToken;
  final DateTime createdAt;

  ProleadFileVersion({
    required this.id,
    required this.bucket,
    required this.path,
    required this.version,
    required this.size,
    required this.sha256,
    required this.downloadToken,
    required this.createdAt,
  });

  factory ProleadFileVersion.fromJson(Map<String, dynamic> json) {
    return ProleadFileVersion(
      id: json['id']?.toString() ?? '',
      bucket: json['bucket'] ?? '',
      path: json['path'] ?? '',
      version: (json['version'] as num?)?.toInt() ?? 1,
      size: (json['size'] as num?)?.toInt() ?? 0,
      sha256: json['sha256'] ?? '',
      downloadToken: json['downloadToken'] ?? json['download_token'] ?? '',
      createdAt: json['createdAt'] != null || json['created_at'] != null
          ? DateTime.tryParse(json['createdAt'] ?? json['created_at']) ?? DateTime.now()
          : DateTime.now(),
    );
  }
}

/// Storage appliance telemetry and deduplication metrics.
class ProleadStats {
  final int totalFiles;
  final int totalLogicalBytes;
  final int totalPhysicalBytes;
  final int totalSavedBytes;
  final double dedupRatio;
  final int activeBuckets;
  final int activeVersions;
  final int trashFiles;

  ProleadStats({
    required this.totalFiles,
    required this.totalLogicalBytes,
    required this.totalPhysicalBytes,
    required this.totalSavedBytes,
    required this.dedupRatio,
    required this.activeBuckets,
    required this.activeVersions,
    required this.trashFiles,
  });

  factory ProleadStats.fromJson(Map<String, dynamic> json) {
    return ProleadStats(
      totalFiles: (json['totalFiles'] ?? json['total_files'] as num?)?.toInt() ?? 0,
      totalLogicalBytes: (json['totalLogicalBytes'] ?? json['total_logical_bytes'] as num?)?.toInt() ?? 0,
      totalPhysicalBytes: (json['totalPhysicalBytes'] ?? json['total_physical_bytes'] as num?)?.toInt() ?? 0,
      totalSavedBytes: (json['totalSavedBytes'] ?? json['total_saved_bytes'] as num?)?.toInt() ?? 0,
      dedupRatio: (json['dedupRatio'] ?? json['dedup_ratio'] as num?)?.toDouble() ?? 1.0,
      activeBuckets: (json['activeBuckets'] ?? json['active_buckets'] as num?)?.toInt() ?? 0,
      activeVersions: (json['activeVersions'] ?? json['active_versions'] as num?)?.toInt() ?? 0,
      trashFiles: (json['trashFiles'] ?? json['trash_files'] as num?)?.toInt() ?? 0,
    );
  }
}

/// List files and directory query response.
class ProleadFileListResult {
  final List<ProleadFileObject> items;
  final List<String> prefixes;
  final int total;

  ProleadFileListResult({
    required this.items,
    required this.prefixes,
    required this.total,
  });

  factory ProleadFileListResult.fromJson(Map<String, dynamic> json) {
    var rawItems = json['items'] as List? ?? [];
    var list = rawItems.map((e) => ProleadFileObject.fromJson(e)).toList();
    var prefixes = (json['prefixes'] as List?)?.map((e) => e.toString()).toList() ?? [];
    return ProleadFileListResult(
      items: list,
      prefixes: prefixes,
      total: (json['total'] as num?)?.toInt() ?? list.length,
    );
  }
}

/// Image transformation request parameters.
class ProleadImageTransform {
  final int? width;
  final int? height;
  final String? fit; // cover, contain, fill, scale
  final String? format; // webp, jpeg, png, gif
  final int? quality; // 1-100

  const ProleadImageTransform({
    this.width,
    this.height,
    this.fit,
    this.format,
    this.quality,
  });

  Map<String, String> toQueryParams() {
    final params = <String, String>{};
    if (width != null && width! > 0) params['w'] = width.toString();
    if (height != null && height! > 0) params['h'] = height.toString();
    if (fit != null && fit!.isNotEmpty) params['fit'] = fit!;
    if (format != null && format!.isNotEmpty) params['format'] = format!;
    if (quality != null && quality! > 0) params['q'] = quality.toString();
    return params;
  }
}

/// Upload progress report.
class ProleadUploadProgress {
  final int bytesSent;
  final int totalBytes;
  final double progressPercent;
  final bool isCompleted;
  final ProleadFileObject? result;

  ProleadUploadProgress({
    required this.bytesSent,
    required this.totalBytes,
    required this.progressPercent,
    required this.isCompleted,
    this.result,
  });
}

/// Signed URL generated response.
class ProleadSignedUrl {
  final String signedUrl;
  final DateTime expiresAt;

  ProleadSignedUrl({
    required this.signedUrl,
    required this.expiresAt,
  });

  factory ProleadSignedUrl.fromJson(Map<String, dynamic> json) {
    return ProleadSignedUrl(
      signedUrl: json['signedUrl'] ?? json['signed_url'] ?? '',
      expiresAt: DateTime.tryParse(json['expiresAt'] ?? json['expires_at'] ?? '') ?? DateTime.now(),
    );
  }
}

/// Real-time Server-Sent Event notification.
class ProleadEvent {
  final String eventType;
  final String bucket;
  final String path;
  final int size;
  final String contentType;
  final DateTime timestamp;

  ProleadEvent({
    required this.eventType,
    required this.bucket,
    required this.path,
    required this.size,
    required this.contentType,
    required this.timestamp,
  });

  factory ProleadEvent.fromJson(Map<String, dynamic> json) {
    return ProleadEvent(
      eventType: json['eventType'] ?? json['event_type'] ?? 'object.created',
      bucket: json['bucket'] ?? '',
      path: json['path'] ?? '',
      size: (json['size'] as num?)?.toInt() ?? 0,
      contentType: json['contentType'] ?? json['content_type'] ?? '',
      timestamp: DateTime.tryParse(json['timestamp'] ?? '') ?? DateTime.now(),
    );
  }
}
