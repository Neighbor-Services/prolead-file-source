import 'package:flutter/material.dart';
import '../models.dart';
import '../client.dart';

/// A reactive drag-and-drop file upload target widget with progress animation.
class ProleadDropZone extends StatefulWidget {
  final ProleadFileClient client;
  final String bucket;
  final String pathPrefix;
  final ValueChanged<ProleadFileObject>? onUploadCompleted;
  final ValueChanged<Object>? onUploadError;
  final Widget? child;

  const ProleadDropZone({
    super.key,
    required this.client,
    this.bucket = 'default',
    this.pathPrefix = '',
    this.onUploadCompleted,
    this.onUploadError,
    this.child,
  });

  @override
  State<ProleadDropZone> createState() => _ProleadDropZoneState();
}

class _ProleadDropZoneState extends State<ProleadDropZone> {
  bool _isHovering = false;
  bool _isUploading = false;
  double _uploadProgress = 0.0;
  String _uploadStatus = '';

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return DragTarget<Object>(
      onWillAcceptWithDetails: (details) {
        setState(() => _isHovering = true);
        return true;
      },
      onLeave: (data) {
        setState(() => _isHovering = false);
      },
      onAcceptWithDetails: (details) {
        setState(() => _isHovering = false);
        // On web/desktop flutter file drop handling
      },
      builder: (context, candidateData, rejectedData) {
        return AnimatedContainer(
          duration: const Duration(milliseconds: 200),
          decoration: BoxDecoration(
            color: _isHovering
                ? theme.colorScheme.primary.withOpacity(0.08)
                : theme.colorScheme.surfaceVariant.withOpacity(0.2),
            borderRadius: BorderRadius.circular(16),
            border: Border.all(
              color: _isHovering ? theme.colorScheme.primary : theme.dividerColor.withOpacity(0.5),
              width: _isHovering ? 2.0 : 1.0,
              strokeAlign: BorderSide.strokeAlignCenter,
            ),
          ),
          child: widget.child ??
              Padding(
                padding: const EdgeInsets.all(32.0),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(
                      _isUploading ? Icons.cloud_upload : Icons.file_upload_outlined,
                      size: 48,
                      color: _isHovering ? theme.colorScheme.primary : theme.hintColor,
                    ),
                    const SizedBox(height: 12),
                    Text(
                      _isUploading ? _uploadStatus : 'Drag & drop files here to upload',
                      style: theme.textTheme.titleMedium?.copyWith(
                        fontWeight: FontWeight.w600,
                        color: _isHovering ? theme.colorScheme.primary : theme.textTheme.titleMedium?.color,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      'Directly stored to bucket: ${widget.bucket}',
                      style: theme.textTheme.bodySmall?.copyWith(color: theme.hintColor),
                    ),
                    if (_isUploading) ...[
                      const SizedBox(height: 16),
                      ClipRRect(
                        borderRadius: BorderRadius.circular(8),
                        child: LinearProgressIndicator(
                          value: _uploadProgress,
                          minHeight: 6,
                        ),
                      ),
                      const SizedBox(height: 8),
                      Text('${(_uploadProgress * 100).toStringAsFixed(0)}% uploaded',
                          style: theme.textTheme.labelSmall),
                    ],
                  ],
                ),
              ),
        );
      },
    );
  }
}
