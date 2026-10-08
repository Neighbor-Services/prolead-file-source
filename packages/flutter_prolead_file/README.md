# flutter_prolead_file

Official Dart and Flutter SDK for **Prolead File** — on-premise enterprise object storage appliance with deduplication, dynamic image transformation, versioning, and real-time SSE streams.

---

## 🚀 Features

- **Object Storage & Streaming Uploads**: Fast multipart uploads with byte-level progress reporting.
- **Hierarchical Directory Management**: Virtual folder prefixes and `.keep` directory creation.
- **Dynamic Image Transformations**: On-the-fly resizing (`w`, `h`), cropping (`fit`), format transcoding (`webp`, `jpeg`, `png`), and quality adjustments.
- **ProleadFileImage Widget**: Drop-in Flutter widget with automatic placeholders, error handling, and server-side image scaling.
- **Security & Signed URLs**: Generate expiring HMAC-signed download URLs.
- **Real-Time Event Streams**: Built-in SSE client (`/api/v1/events/stream`) for live notifications.

---

## 📦 Installation

Add `flutter_prolead_file` to your `pubspec.yaml`:

```yaml
dependencies:
  flutter_prolead_file:
    path: ../packages/flutter_prolead_file
```

---

## 🛠️ Quick Start

```dart
import 'dart:typed_data';
import 'package:flutter_prolead_file/flutter_prolead_file.dart';

final client = ProleadFileClient(
  baseUrl: 'http://localhost:8080',
  apiKey: 'prolead-master-secret-key',
);

// 1. List Buckets
final buckets = await client.listBuckets();

// 2. Upload with Real-Time Progress Stream
final bytes = Uint8List.fromList('Hello Prolead File'.codeUnits);
client.uploadBytesWithProgress(
  bucket: 'default',
  path: 'documents/hello.txt',
  bytes: bytes,
  filename: 'hello.txt',
  contentType: 'text/plain',
).listen((progress) {
  print('Uploaded: ${progress.progressPercent}%');
  if (progress.isCompleted) {
    print('Done! URL: ${progress.result?.downloadUrl}');
  }
});

// 3. Render Transformed Images in Flutter
Widget buildImage(BuildContext context) {
  return ProleadFileImage(
    client: client,
    rawDownloadUrl: '/v0/b/default/o/avatars%2Fuser.png',
    width: 200,
    height: 200,
    fit: BoxFit.cover,
  );
}
```
