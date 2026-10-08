import { Component, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';

interface FeatureItem {
  id: number;
  category: string;
  name: string;
  description: string;
  endpoints: string[];
  status: 'active' | 'configured';
}

@Component({
  selector: 'app-features',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './features.component.html',
  styleUrl: './features.component.css'
})
export class FeaturesComponent {
  searchQuery = signal<string>('');
  selectedCategory = signal<string>('All');

  categories = [
    'All',
    'Core Storage & Deduplication',
    'Security & Access Control',
    'Multi-Tenancy & Project Workspaces',
    'Resilience & Operations',
    'Performance & Caching',
    'Real-time & Media Transforms'
  ];

  features: FeatureItem[] = [
    // 1-10: Core Storage & Deduplication
    { id: 1, category: 'Core Storage & Deduplication', name: 'Multi-Bucket Virtual Organization', description: 'Partition data logically into isolated buckets with public/private ACLs.', endpoints: ['GET /api/v1/buckets', 'POST /api/v1/buckets'], status: 'active' },
    { id: 2, category: 'Core Storage & Deduplication', name: 'Content-Addressable Deduplication (CAS)', description: 'Chunk-level SHA-256 deduplication stores unique byte blocks once across the cluster.', endpoints: ['CAS Storage Engine'], status: 'active' },
    { id: 3, category: 'Core Storage & Deduplication', name: 'Zero-Copy Stream Uploads & Downloads', description: 'Stream large files with io.CopyBuffer directly between network and chunk store.', endpoints: ['POST /v0/b/:bucket/o', 'GET /v0/b/:bucket/o/:name'], status: 'active' },
    { id: 4, category: 'Core Storage & Deduplication', name: 'Immutable Object Versioning', description: 'Preserves timestamped historical iterations on overwrite with instant rollback.', endpoints: ['GET /api/v1/b/:bucket/versions'], status: 'active' },
    { id: 5, category: 'Core Storage & Deduplication', name: 'Soft Deletes & Recycle Bin', description: 'Safety trash retention prevents accidental data loss with one-click restore.', endpoints: ['POST /api/v1/b/:bucket/restore', 'DELETE /api/v1/b/:bucket/o/:name?trash=true'], status: 'active' },
    { id: 6, category: 'Core Storage & Deduplication', name: 'Permanent Purge Protocol', description: 'Hard deletion frees physical references and cleans chunk metadata.', endpoints: ['DELETE /api/v1/b/:bucket/o/:name?permanent=true'], status: 'active' },
    { id: 7, category: 'Core Storage & Deduplication', name: 'Multi-Part Resumable Chunked Uploads', description: 'Upload massive datasets in concurrent 5MB chunks with automatic re-assembly.', endpoints: ['POST /api/v1/multipart/init', 'PUT /api/v1/multipart/chunk'], status: 'active' },
    { id: 8, category: 'Core Storage & Deduplication', name: 'Live Zip Archive Streaming', description: 'Compress and download entire directories or selected files on-the-fly.', endpoints: ['POST /api/v1/b/:bucket/download-zip'], status: 'active' },
    { id: 9, category: 'Core Storage & Deduplication', name: 'Custom JSON Object Metadata', description: 'Attach arbitrary key-value attributes, tags, and indexing data to objects.', endpoints: ['POST /v0/b/:bucket/o (metadata header)'], status: 'active' },
    { id: 10, category: 'Core Storage & Deduplication', name: 'Automatic MIME Sniffing & Overrides', description: 'Real-time content detection with manual Content-Type header overrides.', endpoints: ['GET /v0/b/:bucket/o/:name'], status: 'active' },

    // 11-20: Security & Access Control
    { id: 11, category: 'Security & Access Control', name: 'Bcrypt Admin Superuser Authentication', description: 'Secure administrative authentication using bcrypt password hashing.', endpoints: ['POST /api/v1/auth/login'], status: 'active' },
    { id: 12, category: 'Security & Access Control', name: 'CLI Superuser Provisioner (Django-like)', description: 'Dedicated CLI tool (cmd/createsuperuser) for zero-risk offline admin creation.', endpoints: ['CLI: go run ./cmd/createsuperuser'], status: 'active' },
    { id: 13, category: 'Security & Access Control', name: 'Granular Scoped API Keys', description: 'Generate read-only, read-write, or admin keys with bucket-level scoping.', endpoints: ['GET /api/v1/keys', 'POST /api/v1/keys'], status: 'active' },
    { id: 14, category: 'Security & Access Control', name: 'Instant API Key Revocation', description: 'Immediately terminate compromised credentials without restarting servers.', endpoints: ['POST /api/v1/keys/:id/revoke'], status: 'active' },
    { id: 15, category: 'Security & Access Control', name: 'HMAC-SHA256 Signed Time-Limited URLs', description: 'Grant temporary authenticated access for clients without revealing master credentials.', endpoints: ['POST /api/v1/b/:bucket/sign-url'], status: 'active' },
    { id: 16, category: 'Security & Access Control', name: 'Rotating Download Security Tokens', description: 'Invalidate public download links instantly with token rotation.', endpoints: ['POST /api/v1/b/:bucket/rotate-token'], status: 'active' },
    { id: 17, category: 'Security & Access Control', name: 'S3-Compatible V4 Signature Gateway', description: 'Drop-in interoperability for AWS SDKs and standard S3 tools (cyberduck, rclone).', endpoints: ['/s3/:bucket/*'], status: 'active' },
    { id: 18, category: 'Security & Access Control', name: 'Dynamic CORS Security Filtering', description: 'Fine-grained Cross-Origin Resource Sharing controls for web client access.', endpoints: ['CORS Middleware'], status: 'active' },
    { id: 19, category: 'Security & Access Control', name: 'Constant-Time Token Verification', description: 'Subtle cryptographic constant-time comparison prevents side-channel timing attacks.', endpoints: ['crypto/subtle in auth layer'], status: 'active' },
    { id: 20, category: 'Security & Access Control', name: 'IP-Bound Audit Logging', description: 'Immutable records logging client IP, User-Agent, action, and timestamp.', endpoints: ['GET /api/v1/admin/audit-logs'], status: 'active' },

    // 21-30: Multi-Tenancy & Project Workspaces
    { id: 21, category: 'Multi-Tenancy & Project Workspaces', name: 'Multi-Tenant Project Isolation', description: 'Tenant separation organizing storage buckets, keys, and events under Projects.', endpoints: ['GET /api/v1/projects', 'POST /api/v1/projects'], status: 'active' },
    { id: 22, category: 'Multi-Tenancy & Project Workspaces', name: 'Per-Project Storage Quotas', description: 'Configurable max storage and quota limits enforced at upload time.', endpoints: ['Domain Quota Engine'], status: 'active' },
    { id: 23, category: 'Multi-Tenancy & Project Workspaces', name: 'Custom Project Slugs & Identifiers', description: 'Human-readable URL paths for project routing and multi-account switching.', endpoints: ['/projects/:slug/*'], status: 'active' },
    { id: 24, category: 'Multi-Tenancy & Project Workspaces', name: 'Scoped Project API Credentials', description: 'API keys restricted to a single project workspace.', endpoints: ['/api/v1/keys (Project Header)'], status: 'active' },
    { id: 25, category: 'Multi-Tenancy & Project Workspaces', name: 'Isolated Bucket Namespaces', description: 'Projects create private buckets without naming collisions.', endpoints: ['Project Bucket Scoping'], status: 'active' },
    { id: 26, category: 'Multi-Tenancy & Project Workspaces', name: 'Tenant Usage Aggregation', description: 'Real-time calculation of project storage size, file counts, and bandwith.', endpoints: ['GET /api/v1/projects'], status: 'active' },
    { id: 27, category: 'Multi-Tenancy & Project Workspaces', name: 'Project Switcher & Context Persist', description: 'Frontend workspace switcher maintaining active project across page reloads.', endpoints: ['Frontend LocalStorage State'], status: 'active' },
    { id: 28, category: 'Multi-Tenancy & Project Workspaces', name: 'Project-Level Webhook Routing', description: 'Notifications filtered and delivered specifically to project endpoints.', endpoints: ['/api/v1/webhooks'], status: 'active' },
    { id: 29, category: 'Multi-Tenancy & Project Workspaces', name: 'Tenant-Level Audit Isolation', description: 'Audit trail filtered per tenant for compliance reporting.', endpoints: ['/api/v1/admin/audit-logs'], status: 'active' },
    { id: 30, category: 'Multi-Tenancy & Project Workspaces', name: 'Hot Project Migration & Export', description: 'Archive and export project assets into standard zip structures.', endpoints: ['Storage Hot Backup'], status: 'active' },

    // 31-40: Resilience & Operations
    { id: 31, category: 'Resilience & Operations', name: 'Hot SQLite WAL Snapshots', description: 'Generate live database and metadata backups without pausing writes or taking locks.', endpoints: ['POST /api/v1/admin/backup'], status: 'active' },
    { id: 32, category: 'Resilience & Operations', name: 'Content-Addressable Garbage Collector', description: 'Sweeps orphaned chunk files and updates free disk capacity.', endpoints: ['POST /api/v1/admin/gc'], status: 'active' },
    { id: 33, category: 'Resilience & Operations', name: 'SHA-256 Bitrot Scrubber', description: 'Periodic background integrity checks detecting disk bitrot or payload corruption.', endpoints: ['POST /api/v1/admin/scrub'], status: 'active' },
    { id: 34, category: 'Resilience & Operations', name: 'Auto-Expiring Ephemeral Objects', description: 'Set TTL durations in seconds on objects for automatic lifecycle expiration.', endpoints: ['POST /v0/b/:bucket/o (expiresInSeconds)'], status: 'active' },
    { id: 35, category: 'Resilience & Operations', name: 'Health & Liveness Probe Gateway', description: 'Kubernetes/Docker liveness and readiness health endpoints.', endpoints: ['GET /healthz'], status: 'active' },
    { id: 36, category: 'Resilience & Operations', name: 'OpenMetrics / Prometheus Exporter', description: 'Real-time telemetry for HTTP latencies, memory footprint, and disk usage.', endpoints: ['GET /metrics'], status: 'active' },
    { id: 37, category: 'Resilience & Operations', name: 'Atomic Safe Renames & Moves', description: 'Transaction-safe object key moves and renaming without file re-upload.', endpoints: ['POST /api/v1/b/:bucket/move'], status: 'active' },
    { id: 38, category: 'Resilience & Operations', name: 'Crash-Safe WAL Journaling', description: 'SQLite write-ahead logging prevents data corruption during unexpected power cuts.', endpoints: ['SQLite pragma wal'], status: 'active' },
    { id: 39, category: 'Resilience & Operations', name: 'Self-Healing Metadata Indexing', description: 'Reconstruct broken metadata indexes from on-disk chunk trees.', endpoints: ['CAS Scrubber Engine'], status: 'active' },
    { id: 40, category: 'Resilience & Operations', name: 'Structured JSON Logging', description: 'High-performance structured stdout logging for log shippers like Datadog & Loki.', endpoints: ['Go Logrus / Slog'], status: 'active' },

    // 41-50: Real-time, Performance & Media Transforms
    { id: 41, category: 'Real-time & Media Transforms', name: 'Dynamic On-The-Fly Image Resizing', description: 'Instant image thumbnailing using Lanczos resampling query parameters (?w=300&h=300).', endpoints: ['GET /v0/b/:bucket/o/:name?w=200&h=200'], status: 'active' },
    { id: 42, category: 'Real-time & Media Transforms', name: 'Image Format Transcoding (WebP/JPEG)', description: 'Convert PNGs/JPEGs to high-compression WebP dynamically (?format=webp).', endpoints: ['GET /v0/b/:bucket/o/:name?format=webp'], status: 'active' },
    { id: 43, category: 'Real-time & Media Transforms', name: 'Dynamic Image Quality Scaling', description: 'Adjust JPEG/WebP compression quality dynamically (?q=75).', endpoints: ['GET /v0/b/:bucket/o/:name?q=75'], status: 'active' },
    { id: 44, category: 'Real-time & Media Transforms', name: 'Server-Sent Events (SSE) Live Feed', description: 'Real-time streaming pipeline broadcasting upload, delete, and bucket events.', endpoints: ['GET /api/v1/events'], status: 'active' },
    { id: 45, category: 'Real-time & Media Transforms', name: 'Automated Webhook Dispatch Engine', description: 'Deliver JSON event payloads to external URLs with retry support.', endpoints: ['POST /api/v1/webhooks'], status: 'active' },
    { id: 46, category: 'Performance & Caching', name: 'In-Memory Adaptive LRU Cache', description: 'High-speed ARC/LRU RAM cache serving hot files directly from memory.', endpoints: ['Local Memory Storage Layer'], status: 'active' },
    { id: 47, category: 'Performance & Caching', name: 'HTTP 304 Not Modified & ETag Caching', description: 'Client-side conditional GET caching using SHA-256 ETags.', endpoints: ['If-None-Match / ETag Headers'], status: 'active' },
    { id: 48, category: 'Performance & Caching', name: 'HTTP Byte-Range Streaming (RFC 7233)', description: 'Stream video and audio with instant seeking (Range: bytes=0-1048576).', endpoints: ['GET /v0/b/:bucket/o/:name (Range)'], status: 'active' },
    { id: 49, category: 'Real-time & Media Transforms', name: 'Zoneless Angular 19 Client UI', description: 'High-performance reactive frontend using modern Angular Signals & standalone components.', endpoints: ['Frontend Architecture'], status: 'active' },
    { id: 50, category: 'Real-time & Media Transforms', name: 'Full Interactive Admin Dashboard', description: 'Unified multi-workspace control center for modern cloud file exploration.', endpoints: ['All Frontend Routes'], status: 'active' }
  ];

  filteredFeatures(): FeatureItem[] {
    const q = this.searchQuery().toLowerCase().trim();
    const cat = this.selectedCategory();
    return this.features.filter(f => {
      const matchCat = cat === 'All' || f.category === cat;
      const matchQuery = !q || f.name.toLowerCase().includes(q) || f.description.toLowerCase().includes(q) || f.category.toLowerCase().includes(q);
      return matchCat && matchQuery;
    });
  }
}
