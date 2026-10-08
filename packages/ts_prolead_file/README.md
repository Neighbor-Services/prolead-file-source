# @prolead-file/sdk

Official TypeScript and Node.js SDK for **Prolead File** on-premise high-performance object storage appliance.

---

## 🚀 Features

- **Universal Runtime Support**: Works seamlessly in Node.js (v18+), Bun, Deno, and modern web browsers.
- **Full TypeScript Definitions**: End-to-end type safety for buckets, objects, versions, and quotas.
- **Multipart Uploads & Streaming**: Upload Files, Blobs, Buffers, and Strings with ease.
- **Dynamic Image Transformations**: On-the-fly resizing, cropping, transcoding, and quality optimization.
- **Signed URLs**: Generate secure, time-limited HMAC download links.
- **Server-Sent Events (SSE)**: Built-in event listener for real-time storage mutations.

---

## 📦 Installation

```bash
npm install @prolead-file/sdk
```

---

## 🛠️ Quick Start

```typescript
import { ProleadFileClient } from '@prolead-file/sdk';

const client = new ProleadFileClient({
  baseUrl: 'http://localhost:8080',
  apiKey: 'prolead-master-secret-key'
});

// 1. List Buckets
const buckets = await client.listBuckets();
console.log('Buckets:', buckets);

// 2. Upload String or Buffer
const file = await client.upload('Hello Prolead File!', {
  bucket: 'default',
  path: 'logs/app.log',
  contentType: 'text/plain'
});
console.log('Uploaded:', file.downloadUrl);

// 3. Real-Time Event Subscription
const unsubscribe = client.subscribeEvents((event) => {
  console.log('Storage Event:', event.eventType, event.path);
});
```
