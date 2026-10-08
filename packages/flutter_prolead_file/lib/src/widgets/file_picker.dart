import 'package:flutter/material.dart';
import '../models.dart';
import '../client.dart';
import 'image_widget.dart';

/// Opens a rich modal bottom sheet to browse and select files from a Prolead File bucket.
Future<List<ProleadFileObject>?> showProleadFilePicker(
  BuildContext context, {
  required ProleadFileClient client,
  String bucket = 'default',
  bool allowMultiple = false,
  List<String>? allowedExtensions,
  String title = 'Select File',
}) {
  return showModalBottomSheet<List<ProleadFileObject>>(
    context: context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    builder: (ctx) => ProleadFilePickerSheet(
      client: client,
      bucket: bucket,
      allowMultiple: allowMultiple,
      allowedExtensions: allowedExtensions,
      title: title,
    ),
  );
}

class ProleadFilePickerSheet extends StatefulWidget {
  final ProleadFileClient client;
  final String bucket;
  final bool allowMultiple;
  final List<String>? allowedExtensions;
  final String title;

  const ProleadFilePickerSheet({
    super.key,
    required this.client,
    required this.bucket,
    this.allowMultiple = false,
    this.allowedExtensions,
    this.title = 'Select File',
  });

  @override
  State<ProleadFilePickerSheet> createState() => _ProleadFilePickerSheetState();
}

class _ProleadFilePickerSheetState extends State<ProleadFilePickerSheet> {
  String _currentPrefix = '';
  String _searchQuery = '';
  bool _isLoading = true;
  String? _error;
  List<ProleadFileObject> _files = [];
  List<String> _folders = [];
  final Set<String> _selectedIds = {};
  final Map<String, ProleadFileObject> _selectedObjects = {};

  @override
  void initState() {
    super.initState();
    _loadDirectory();
  }

  Future<void> _loadDirectory() async {
    setState(() {
      _isLoading = true;
      _error = null;
    });

    try {
      final res = await widget.client.listFiles(
        widget.bucket,
        prefix: _currentPrefix,
        delimiter: '/',
        search: _searchQuery,
        limit: 200,
      );

      final files = res.items.where((f) => !f.name.endsWith('.keep')).toList();
      if (widget.allowedExtensions != null && widget.allowedExtensions!.isNotEmpty) {
        final exts = widget.allowedExtensions!.map((e) => e.toLowerCase()).toList();
        files.removeWhere((f) {
          final ext = f.name.contains('.') ? f.name.split('.').last.toLowerCase() : '';
          return !exts.contains(ext);
        });
      }

      setState(() {
        _files = files;
        _folders = res.prefixes;
        _isLoading = false;
      });
    } catch (e) {
      setState(() {
        _error = e.toString();
        _isLoading = false;
      });
    }
  }

  void _navigateToFolder(String prefix) {
    setState(() {
      _currentPrefix = prefix;
    });
    _loadDirectory();
  }

  void _navigateUp() {
    if (_currentPrefix.isEmpty) return;
    final parts = _currentPrefix.replaceAll(RegExp(r'/+$'), '').split('/');
    if (parts.length <= 1) {
      _currentPrefix = '';
    } else {
      parts.removeLast();
      _currentPrefix = '${parts.join('/')}/';
    }
    _loadDirectory();
  }

  void _toggleSelection(ProleadFileObject file) {
    setState(() {
      if (widget.allowMultiple) {
        if (_selectedIds.contains(file.id)) {
          _selectedIds.remove(file.id);
          _selectedObjects.remove(file.id);
        } else {
          _selectedIds.add(file.id);
          _selectedObjects[file.id] = file;
        }
      } else {
        _selectedIds.clear();
        _selectedObjects.clear();
        _selectedIds.add(file.id);
        _selectedObjects[file.id] = file;
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Container(
      height: MediaQuery.of(context).size.height * 0.85,
      decoration: BoxDecoration(
        color: theme.colorScheme.surface,
        borderRadius: const BorderRadius.vertical(top: Radius.circular(20)),
        boxShadow: const [
          BoxShadow(color: Colors.black26, blurRadius: 20, offset: Offset(0, -4)),
        ],
      ),
      child: Column(
        children: [
          // Header
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            child: Row(
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(widget.title, style: theme.textTheme.titleLarge?.copyWith(fontWeight: FontWeight.bold)),
                      const SizedBox(height: 2),
                      Text('Bucket: ${widget.bucket}${_currentPrefix.isNotEmpty ? "/$_currentPrefix" : ""}',
                          style: theme.textTheme.bodySmall?.copyWith(color: theme.hintColor)),
                    ],
                  ),
                ),
                if (_currentPrefix.isNotEmpty)
                  IconButton(
                    icon: const Icon(Icons.arrow_upward),
                    tooltip: 'Up one directory',
                    onPressed: _navigateUp,
                  ),
                IconButton(
                  icon: const Icon(Icons.close),
                  onPressed: () => Navigator.of(context).pop(),
                ),
              ],
            ),
          ),
          const Divider(height: 1),

          // Search Bar
          Padding(
            padding: const EdgeInsets.all(12),
            child: TextField(
              decoration: InputDecoration(
                hintText: 'Search files in ${widget.bucket}...',
                prefixIcon: const Icon(Icons.search),
                filled: true,
                contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
                border: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: BorderSide.none),
              ),
              onChanged: (val) {
                _searchQuery = val;
                _loadDirectory();
              },
            ),
          ),

          // File List
          Expanded(
            child: _isLoading
                ? const Center(child: CircularProgressIndicator())
                : _error != null
                    ? Center(child: Text('Error: $_error', style: const TextStyle(color: Colors.red)))
                    : (_files.isEmpty && _folders.isEmpty)
                        ? Center(child: Text('No files found', style: TextStyle(color: theme.hintColor)))
                        : ListView(
                            padding: const EdgeInsets.symmetric(horizontal: 8),
                            children: [
                              ..._folders.map((folder) {
                                final folderName = folder.replaceAll(RegExp(r'/+$'), '').split('/').last;
                                return ListTile(
                                  leading: const Icon(Icons.folder, color: Colors.amber),
                                  title: Text(folderName),
                                  trailing: const Icon(Icons.chevron_right),
                                  onTap: () => _navigateToFolder(folder),
                                );
                              }),
                              ..._files.map((file) {
                                final isSelected = _selectedIds.contains(file.id);
                                final isImage = file.contentType.startsWith('image/');

                                return ListTile(
                                  selected: isSelected,
                                  leading: ClipRRect(
                                    borderRadius: BorderRadius.circular(6),
                                    child: isImage
                                        ? SizedBox(
                                            width: 40,
                                            height: 40,
                                            child: ProleadFileImage(
                                              client: widget.client,
                                              rawDownloadUrl: file.downloadUrl,
                                              width: 40,
                                              height: 40,
                                            ),
                                          )
                                        : Container(
                                            width: 40,
                                            height: 40,
                                            color: theme.colorScheme.primaryContainer,
                                            child: Icon(_getFileIcon(file.contentType), color: theme.colorScheme.onPrimaryContainer),
                                          ),
                                  ),
                                  title: Text(file.name, maxLines: 1, overflow: TextOverflow.ellipsis),
                                  subtitle: Text(_formatBytes(file.size), style: theme.textTheme.bodySmall),
                                  trailing: isSelected
                                      ? Icon(Icons.check_circle, color: theme.colorScheme.primary)
                                      : const Icon(Icons.radio_button_unchecked, color: Colors.grey),
                                  onTap: () => _toggleSelection(file),
                                );
                              }),
                            ],
                          ),
          ),

          // Bottom Action Bar
          Container(
            padding: const EdgeInsets.all(16),
            decoration: BoxDecoration(
              color: theme.colorScheme.surfaceContainerHighest.withValues(alpha: 0.3),
              border: Border(top: BorderSide(color: theme.dividerColor.withValues(alpha: 0.1))),
            ),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text('${_selectedIds.length} item(s) selected', style: theme.textTheme.bodyMedium),
                ElevatedButton(
                  onPressed: _selectedIds.isEmpty
                      ? null
                      : () {
                          Navigator.of(context).pop(_selectedObjects.values.toList());
                        },
                  style: ElevatedButton.styleFrom(
                    padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(10)),
                  ),
                  child: const Text('Confirm Selection'),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  IconData _getFileIcon(String mime) {
    if (mime.startsWith('image/')) return Icons.image;
    if (mime.startsWith('video/')) return Icons.movie;
    if (mime.startsWith('audio/')) return Icons.audiotrack;
    if (mime.contains('pdf')) return Icons.picture_as_pdf;
    if (mime.contains('zip') || mime.contains('compressed')) return Icons.archive;
    if (mime.startsWith('text/')) return Icons.description;
    return Icons.insert_drive_file;
  }

  String _formatBytes(int bytes) {
    if (bytes < 1024) return '$bytes B';
    if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
    if (bytes < 1024 * 1024 * 1024) return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
    return '${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(2)} GB';
  }
}
