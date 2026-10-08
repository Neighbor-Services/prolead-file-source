import 'package:flutter/material.dart';
import '../models.dart';
import '../client.dart';
import 'image_widget.dart';

/// Unified previewer widget that dynamically renders images, videos, audio, PDF markers, and text.
class ProleadMediaPreview extends StatelessWidget {
  final ProleadFileClient client;
  final ProleadFileObject file;
  final double? width;
  final double? height;

  const ProleadMediaPreview({
    super.key,
    required this.client,
    required this.file,
    this.width,
    this.height,
  });

  @override
  Widget build(BuildContext context) {
    final mime = file.contentType.toLowerCase();
    final theme = Theme.of(context);

    if (mime.startsWith('image/')) {
      return ProleadFileImage(
        client: client,
        rawDownloadUrl: file.downloadUrl,
        width: width,
        height: height,
        fit: BoxFit.contain,
      );
    }

    if (mime.startsWith('audio/')) {
      return Container(
        width: width,
        height: height ?? 140,
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: theme.colorScheme.surfaceVariant.withOpacity(0.3),
          borderRadius: BorderRadius.circular(12),
        ),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(Icons.audiotrack, size: 40, color: theme.colorScheme.primary),
            const SizedBox(height: 8),
            Text(file.name, maxLines: 1, overflow: TextOverflow.ellipsis, style: theme.textTheme.titleSmall),
            const SizedBox(height: 4),
            Text('${_formatBytes(file.size)} • ${file.contentType}', style: theme.textTheme.bodySmall),
          ],
        ),
      );
    }

    if (mime.startsWith('video/')) {
      return Container(
        width: width,
        height: height ?? 220,
        decoration: BoxDecoration(
          color: Colors.black87,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Stack(
          alignment: Alignment.center,
          children: [
            Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                const Icon(Icons.play_circle_fill, size: 56, color: Colors.white70),
                const SizedBox(height: 8),
                Text(file.name, style: const TextStyle(color: Colors.white, fontWeight: FontWeight.bold)),
                Text(_formatBytes(file.size), style: const TextStyle(color: Colors.white60, fontSize: 12)),
              ],
            ),
          ],
        ),
      );
    }

    if (mime.contains('pdf')) {
      return Container(
        width: width,
        height: height ?? 160,
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: Colors.red.withOpacity(0.08),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: Colors.red.withOpacity(0.3)),
        ),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(Icons.picture_as_pdf, size: 48, color: Colors.redAccent),
            const SizedBox(height: 8),
            Text(file.name, maxLines: 1, overflow: TextOverflow.ellipsis, style: theme.textTheme.titleSmall),
            const SizedBox(height: 4),
            Text(_formatBytes(file.size), style: theme.textTheme.bodySmall),
          ],
        ),
      );
    }

    // Generic file fallback
    return Container(
      width: width,
      height: height ?? 120,
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceVariant.withOpacity(0.2),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(Icons.insert_drive_file, size: 40, color: theme.colorScheme.secondary),
          const SizedBox(height: 8),
          Text(file.name, maxLines: 1, overflow: TextOverflow.ellipsis, style: theme.textTheme.titleSmall),
          const SizedBox(height: 4),
          Text(_formatBytes(file.size), style: theme.textTheme.bodySmall),
        ],
      ),
    );
  }

  String _formatBytes(int bytes) {
    if (bytes < 1024) return '$bytes B';
    if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
    if (bytes < 1024 * 1024 * 1024) return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
    return '${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(2)} GB';
  }
}
