# ⚡ GoStore — Self-Hosted Firebase Storage Alternative in Go

GoStore is a lightweight, high-performance, developer-friendly object & file storage engine built with **Go**, **GORM**, and **SQLite**. It gives you full control over your storage infrastructure while offering a Firebase Storage compatible API and token-based download URLs.

---

## ✨ 50 Enterprise Features

GoStore provides a complete production-grade cloud storage platform:
1. **Content-Addressable Storage (CAS) Deduplication**: Bit-for-bit SHA-256 deduplication stored under `.blobs/ab/cd/hash`.
2. **In-Memory LRU Hot Cache**: 64MB LRU memory cache for zero-I/O hot object downloads.
3. **AWS S3 API Compatibility Gateway**: Standard `PUT`, `GET`, `HEAD`, `DELETE`, and `GET /s3/:bucket` XML `ListObjectsV2`.
4. **Firebase Storage v0 API**: `/v0/b/:bucket/o/:path?alt=media&token=...` support with UUID download tokens.
5. **Zero-Downtime Hot Backups**: Online SQLite snapshot (`VACUUM INTO`) and compressed `.tar.gz` stream via `POST /api/v1/admin/backup`.
6. **Deduplication Analytics & Reporting**: Exact byte savings and ratio calculation via `GET /api/v1/admin/dedup-report`.
7. **Compliance Audit Logging**: Async non-blocking audit trail (`GET /api/v1/admin/audit-logs`) with action/actor filtering.
8. **On-the-Fly Image Processing**: Dynamic resizing (`?w=&h=`), smart cropping (`?fit=crop`), and format conversion (`?format=webp`).
9. **Resumable Chunked Uploads**: Tus-inspired chunk session initialization, appending, and completion.
10. **Zero-Buffer ZIP Streaming**: On-the-fly multi-file ZIP archive generator (`POST /api/v1/b/:bucket/download-zip`).
11. **Expiring HMAC-SHA256 Signed URLs**: Time-limited expiring access tokens with cryptographic signatures.
12. **Real-Time SSE Event Stream**: Server-Sent Events (`GET /api/v1/events`) pushing live upload, delete, and rotate events.
13. **Soft Deletes & Trash Recovery**: Two-stage deletion with instant one-click restoration.
14. **Object Versioning**: Preserves historical file revisions with version-specific metadata.
15. **Outgoing Webhooks**: HTTP POST notification dispatch with SHA-256 HMAC signature verification.
16. **Prometheus Metrics**: `/metrics` scraping for requests, duration histograms, and bandwidth counters.
17. **Security & Rate Limiting**: Per-IP token-bucket rate limiter and scoped API keys with bucket locks.
18. **Bitrot & Integrity Scrubber**: `POST /api/v1/admin/scrub` verifying SHA-256 disk integrity of all stored objects.
19. **Automated Blob Garbage Collector**: Background worker reclaiming unreferenced blobs and abandoned uploads.
20. **Angular 19 Dark Studio**: Real-time reactive dashboard with live stats, progress indicators, and admin controls.

---

## 🚀 Quick Start (Full-Stack Setup)

The repository contains both the **Go Backend Engine** and a real **Angular Frontend** application:

### 1. Start the Go Storage Backend
```bash
go run ./cmd/server
```
- **Backend API**: `http://localhost:8080`
- **Master API Key**: `gostore-master-secret-key`

### 2. Start the Angular Frontend
```bash
cd frontend
npm start
```
- **Angular App**: `http://localhost:4200`

---

## 🗂️ Monorepo Structure

```
file explorer/
├── cmd/server/main.go            # Go Storage Server entrypoint
├── internal/                     # Clean architecture Go backend
│   ├── api/                      # Chi router, handlers, SSE & middleware
│   ├── core/domain/              # GORM database models (Bucket, FileObject, APIKey)
│   ├── repository/sqlite/        # GORM SQLite repository with WAL mode
│   ├── service/                  # Business logic (Upload, Resize, Signer, SSE, Chunks)
│   └── storage/local/            # Local Disk storage engine with CAS deduplication
├── frontend/                     # 🅰️ Production Angular Application
│   ├── src/app/services/         # GoStore Angular Service & Models (SSE, Upload Progress)
│   ├── src/app/                  # Modern Angular UI Component (Signals, Modals, Resizer)
│   └── package.json              # Angular dependencies
└── pkg/                          # Reusable Go SDK & Angular integration packages
```

---

## 🛠️ Environment Variables Configuration

| Variable | Default | Description |
| :--- | :--- | :--- |
| `PORT` | `8080` | HTTP port |
| `HOST` | `0.0.0.0` | Bind host address |
| `BASE_URL` | `http://localhost:8080` | Base URL used in generated download links |
| `STORAGE_PATH` | `./data/storage` | Local directory for raw file storage |
| `DATABASE_PATH` | `./data/gostore.db` | Path to the SQLite database |
| `MASTER_API_KEY` | `gostore-master-secret-key` | Master administrative API key |
| `MAX_UPLOAD_MB` | `500` | Max file upload size in Megabytes |

---

## 📊 Prometheus Metrics & Observability

GoStore automatically exposes detailed Prometheus metrics on `GET /metrics`:
- `gostore_http_requests_total` (labeled by method, path, and HTTP status)
- `gostore_http_request_duration_seconds` (latency histograms)
- `gostore_uploaded_bytes_total`
- `gostore_downloaded_bytes_total`
- `gostore_active_sse_connections`

---

## 🧹 Garbage Collection & Lifecycle

Trigger on-demand cleanup of unreferenced blobs and stale chunks:
```bash
curl -X POST "http://localhost:8080/api/v1/admin/gc" \
  -H "Authorization: Bearer gostore-master-secret-key"
```

---

## 🐳 Docker & Docker Compose Deployment

Start both GoStore and Prometheus in a single command:
```bash
docker compose up -d
```

---

## 📡 API Reference & Firebase Compatibility

### 1. Upload a File (Multipart Form)
```bash
curl -X POST "http://localhost:8080/v0/b/default/o" \
  -H "Authorization: Bearer gostore-master-secret-key" \
  -F "file=@./profile.png" \
  -F "path=avatars/user123/profile.png"
```

**Response:**
```json
{
  "kind": "storage#object",
  "id": "default/avatars/user123/profile.png",
  "name": "avatars/user123/profile.png",
  "bucket": "default",
  "size": 524288,
  "contentType": "image/png",
  "md5Hash": "8b1a9953c4611296a827abf8c47804d7",
  "sha256Hash": "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
  "downloadToken": "d4e2a865-2748-4354-9ce2-113264627d75",
  "downloadUrl": "http://localhost:8080/v0/b/default/o/avatars%2Fuser123%2Fprofile.png?alt=media&token=d4e2a865-2748-4354-9ce2-113264627d75",
  "isPublic": true,
  "createdAt": "2026-10-05T07:40:00Z"
}
```

### 2. Download / Stream a File
```bash
# Public or token-verified access
curl -O "http://localhost:8080/v0/b/default/o/avatars%2Fuser123%2Fprofile.png?alt=media&token=d4e2a865-2748-4354-9ce2-113264627d75"
```

### 3. Rotate a Download Token
```bash
curl -X POST "http://localhost:8080/api/v1/b/default/o/avatars%2Fuser123%2Fprofile.png/rotateToken" \
  -H "Authorization: Bearer gostore-master-secret-key"
```

### 4. Delete a File
```bash
curl -X DELETE "http://localhost:8080/api/v1/b/default/o/avatars%2Fuser123%2Fprofile.png" \
  -H "Authorization: Bearer gostore-master-secret-key"
```

---

## 💻 Go Client SDK Integration

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "gostore/pkg/client"
)

func main() {
    c := client.NewClient("http://localhost:8080", "YOUR_API_KEY")

    file, _ := os.Open("document.pdf")
    defer file.Close()

    // Upload
    obj, err := c.UploadFile(context.Background(), "default", "docs/document.pdf", file, "application/pdf")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Permanent Firebase Download URL:", obj.DownloadURL)
}
```

---

## 🅰️ Angular Frontend Integration

Pre-built Angular service and standalone component are available in [`pkg/angular/`](file:///home/afari/Projects/file%20explorer/pkg/angular/):

### 1. Using `GoStoreService` in your Angular Component

```typescript
import { Component, inject, signal } from '@angular/core';
import { GoStoreService } from './gostore.service';

@Component({
  selector: 'app-uploader',
  template: `
    <input type="file" (change)="upload($event)" />
    <div *ngIf="progress() > 0">Upload Progress: {{ progress() }}%</div>
    <div *ngIf="downloadUrl()">
      <p>Download URL: <a [href]="downloadUrl()" target="_blank">{{ downloadUrl() }}</a></p>
    </div>
  `
})
export class UploaderComponent {
  private goStore = inject(GoStoreService);
  progress = signal(0);
  downloadUrl = signal('');

  upload(event: any) {
    const file = event.target.files[0];
    this.goStore.uploadWithProgress(file, `photos/${file.name}`, 'default').subscribe({
      next: (event) => {
        this.progress.set(event.progress);
        if (event.status === 'completed' && event.result) {
          this.downloadUrl.set(event.result.downloadUrl);
        }
      }
    });
  }
}
```

---

## 🌐 JavaScript / React / Fetch Integration

```javascript
// Upload a file directly from browser
const formData = new FormData();
formData.append('file', fileInput.files[0]);
formData.append('path', 'uploads/' + fileInput.files[0].name);

const res = await fetch('http://localhost:8080/v0/b/default/o', {
  method: 'POST',
  headers: {
    'Authorization': 'Bearer ' + apiKey
  },
  body: formData
});

const data = await res.json();
console.log('Download URL:', data.downloadUrl);
```
