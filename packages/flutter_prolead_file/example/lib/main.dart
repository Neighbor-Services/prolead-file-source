import 'dart:convert';
import 'dart:typed_data';
import 'package:flutter/material.dart';
import 'package:flutter_prolead_file/flutter_prolead_file.dart';

void main() {
  runApp(const ProleadFileExampleApp());
}

class ProleadFileExampleApp extends StatelessWidget {
  const ProleadFileExampleApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Prolead File Flutter Integration Demo',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF6366F1),
        brightness: Brightness.dark,
        useMaterial3: true,
      ),
      home: const HomeScreen(),
    );
  }
}

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> with SingleTickerProviderStateMixin {
  late final ProleadFileClient client;
  late final ProleadUploadQueue uploadQueue;
  late final TabController _tabController;

  List<ProleadFileObject> _selectedFiles = [];
  double _uploadProgress = 0.0;
  bool _isUploading = false;
  String _uploadStatus = '';

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 3, vsync: this);
    client = ProleadFileClient(
      baseUrl: 'http://localhost:8080',
      apiKey: '',
    );
    uploadQueue = ProleadUploadQueue(client: client, maxConcurrent: 3);
  }

  @override
  void dispose() {
    _tabController.dispose();
    uploadQueue.dispose();
    client.dispose();
    super.dispose();
  }

  // 1. Direct Multipart Stream Upload
  Future<void> _uploadSampleImage() async {
    setState(() {
      _isUploading = true;
      _uploadProgress = 0.0;
      _uploadStatus = 'Starting upload...';
    });

    final sampleBytes = Uint8List.fromList(
      utf8.encode('Simulated high-res image data for Flutter client upload demo.'),
    );

    try {
      final stream = client.uploadBytesWithProgress(
        bucket: 'default',
        path: 'uploads/flutter_sample_${DateTime.now().millisecondsSinceEpoch}.jpg',
        bytes: sampleBytes,
        filename: 'flutter_sample.jpg',
        contentType: 'image/jpeg',
      );

      await for (final event in stream) {
        setState(() {
          _uploadProgress = event.progressPercent / 100.0;
          _uploadStatus = 'Uploading: ${event.progressPercent.toStringAsFixed(1)}%';
        });

        if (event.isCompleted && event.result != null) {
          setState(() {
            _selectedFiles.insert(0, event.result!);
            _isUploading = false;
            _uploadStatus = 'Upload completed!';
          });
          if (mounted) {
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(content: Text('Uploaded ${event.result!.name} successfully!')),
            );
          }
        }
      }
    } catch (e) {
      setState(() {
        _isUploading = false;
        _uploadStatus = 'Upload failed: $e';
      });
    }
  }

  // 2. TUS 1.0.0 Resumable Upload
  Future<void> _uploadViaTUS() async {
    setState(() {
      _isUploading = true;
      _uploadProgress = 0.0;
      _uploadStatus = 'Initiating TUS 1.0.0 Resumable Session...';
    });

    final sampleBytes = Uint8List.fromList(
      List.generate(1024 * 500, (i) => i % 256), // 500KB chunked test
    );

    try {
      final res = await client.uploadTUS(
        bucket: 'default',
        path: 'videos/tus_resumable_${DateTime.now().millisecondsSinceEpoch}.bin',
        bytes: sampleBytes,
        chunkSize: 128 * 1024,
        onProgress: (p) {
          setState(() {
            _uploadProgress = p;
            _uploadStatus = 'TUS Streaming: ${(p * 100).toStringAsFixed(1)}%';
          });
        },
      );

      setState(() {
        _selectedFiles.insert(0, res);
        _isUploading = false;
        _uploadStatus = 'TUS Resumable Upload Complete!';
      });
    } catch (e) {
      setState(() {
        _isUploading = false;
        _uploadStatus = 'TUS Failed: $e';
      });
    }
  }

  // 3. Multi-file Batch Queue
  void _enqueueBatchUploads() {
    for (int i = 1; i <= 5; i++) {
      uploadQueue.enqueue(
        bucket: 'default',
        path: 'batch/item_$i.txt',
        bytes: Uint8List.fromList(utf8.encode('Content for batch file #$i')),
        filename: 'item_$i.txt',
        contentType: 'text/plain',
      );
    }
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Enqueued 5 files in background upload queue!')),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Prolead File Flutter Studio'),
        centerTitle: false,
        bottom: TabBar(
          controller: _tabController,
          tabs: const [
            Tab(icon: Icon(Icons.cloud_upload), text: 'Uploads & TUS'),
            Tab(icon: Icon(Icons.queue), text: 'Upload Queue'),
            Tab(icon: Icon(Icons.photo_library), text: 'LQIP & Explorer'),
          ],
        ),
      ),
      body: TabBarView(
        controller: _tabController,
        children: [
          _buildUploadTab(),
          _buildQueueTab(),
          _buildExplorerTab(),
        ],
      ),
    );
  }

  Widget _buildUploadTab() {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Card(
            child: Padding(
              padding: const EdgeInsets.all(20),
              child: Column(
                children: [
                  const Icon(Icons.cloud_sync, size: 48, color: Color(0xFF6366F1)),
                  const SizedBox(height: 12),
                  const Text(
                    'Direct Flutter File & Image Uploads',
                    style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
                  ),
                  const SizedBox(height: 8),
                  const Text(
                    'Upload files with live progress streaming, TUS 1.0.0 resumable chunks, and automatic LQIP blur generation.',
                    textAlign: TextAlign.center,
                    style: TextStyle(color: Colors.white70),
                  ),
                  const SizedBox(height: 20),
                  Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      ElevatedButton.icon(
                        icon: const Icon(Icons.upload_file),
                        label: const Text('Stream Upload'),
                        onPressed: _isUploading ? null : _uploadSampleImage,
                      ),
                      const SizedBox(width: 12),
                      OutlinedButton.icon(
                        icon: const Icon(Icons.bolt),
                        label: const Text('TUS Resumable'),
                        onPressed: _isUploading ? null : _uploadViaTUS,
                      ),
                    ],
                  ),
                  if (_isUploading || _uploadStatus.isNotEmpty) ...[
                    const SizedBox(height: 20),
                    LinearProgressIndicator(value: _isUploading ? _uploadProgress : 1.0),
                    const SizedBox(height: 8),
                    Text(_uploadStatus, style: const TextStyle(fontSize: 13, color: Colors.white70)),
                  ],
                ],
              ),
            ),
          ),
          const SizedBox(height: 24),
          const Text('Drag & Drop Target', style: TextStyle(fontSize: 16, fontWeight: FontWeight.bold)),
          const SizedBox(height: 12),
          ProleadDropZone(
            client: client,
            bucket: 'default',
            onUploadCompleted: (obj) {
              setState(() => _selectedFiles.insert(0, obj));
              ScaffoldMessenger.of(context).showSnackBar(
                SnackBar(content: Text('Uploaded ${obj.name} successfully!')),
              );
            },
          ),
        ],
      ),
    );
  }

  Widget _buildQueueTab() {
    return StreamBuilder<List<ProleadQueueItem>>(
      stream: uploadQueue.queueStream,
      initialData: uploadQueue.items,
      builder: (context, snapshot) {
        final items = snapshot.data ?? [];
        return Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Text('Queue Items (${items.length})', style: const TextStyle(fontSize: 18, fontWeight: FontWeight.bold)),
                  ElevatedButton.icon(
                    icon: const Icon(Icons.add),
                    label: const Text('Add 5 Items'),
                    onPressed: _enqueueBatchUploads,
                  ),
                ],
              ),
              const SizedBox(height: 16),
              if (items.isEmpty)
                const Expanded(
                  child: Center(
                    child: Text('Queue is empty. Click "Add 5 Items" to test background batch queue.'),
                  ),
                )
              else
                Expanded(
                  child: ListView.builder(
                    itemCount: items.length,
                    itemBuilder: (ctx, idx) {
                      final item = items[idx];
                      return Card(
                        margin: const EdgeInsets.only(bottom: 8),
                        child: ListTile(
                          leading: Icon(
                            item.status == ProleadUploadStatus.completed
                                ? Icons.check_circle
                                : (item.status == ProleadUploadStatus.uploading ? Icons.sync : Icons.hourglass_top),
                            color: item.status == ProleadUploadStatus.completed ? Colors.green : Colors.amber,
                          ),
                          title: Text(item.filename),
                          subtitle: Text('Status: ${item.status.name} | Path: ${item.path}'),
                          trailing: item.status == ProleadUploadStatus.uploading
                              ? const SizedBox(width: 24, height: 24, child: CircularProgressIndicator(strokeWidth: 2))
                              : null,
                        ),
                      );
                    },
                  ),
                ),
            ],
          ),
        );
      },
    );
  }

  Widget _buildExplorerTab() {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          ElevatedButton.icon(
            icon: const Icon(Icons.folder_open),
            label: const Text('Open Modal File Explorer'),
            onPressed: () async {
              final selected = await showProleadFilePicker(
                context,
                client: client,
                bucket: 'default',
                allowMultiple: true,
              );
              if (selected != null) {
                setState(() => _selectedFiles = selected);
              }
            },
          ),
          const SizedBox(height: 24),
          if (_selectedFiles.isNotEmpty) ...[
            const Text('Loaded Files Preview & LQIP', style: TextStyle(fontSize: 16, fontWeight: FontWeight.bold)),
            const SizedBox(height: 12),
            ..._selectedFiles.map((file) => Padding(
                  padding: const EdgeInsets.only(bottom: 12),
                  child: ProleadMediaPreview(
                    client: client,
                    file: file,
                  ),
                )),
          ],
        ],
      ),
    );
  }
}
