import { Component, inject, signal, computed, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { GoStoreService } from '../../services/gostore.service';
import { NotificationService } from '../../services/notification.service';
import { APIKey, StorageBucket } from '../../services/gostore.models';

@Component({
  selector: 'app-keys',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './keys.component.html',
  styleUrl: './keys.component.css'
})
export class KeysComponent implements OnInit {
  private goStore = inject(GoStoreService);
  private notify = inject(NotificationService);
  private route = inject(ActivatedRoute);

  projectId = signal<string>('p-default');
  apiKeys = signal<APIKey[]>([]);
  availableBuckets = signal<StorageBucket[]>([]);
  searchQuery = signal<string>('');
  statusFilter = signal<'all' | 'active' | 'revoked'>('all');
  revealedKeys = signal<Set<string>>(new Set());

  // Create Key Modal State
  showNewKeyModal = signal<boolean>(false);
  newKeyName = signal<string>('');
  newKeyRole = signal<'admin' | 'read-write' | 'read-only'>('read-write');
  newKeyExpiresDays = signal<number>(0); // 0 = never
  newKeyRateLimit = signal<number>(0);   // 0 = unlimited
  selectedBuckets = signal<Set<string>>(new Set());
  allowAllBuckets = signal<boolean>(true);

  // Key Secret Created Reveal Modal
  newlyCreatedKey = signal<APIKey | null>(null);

  // SDK Quickstart Drawer Modal
  snippetTargetKey = signal<APIKey | null>(null);
  activeSnippetTab = signal<'curl' | 'ts' | 'go' | 'python' | 'flutter' | 's3'>('curl');

  // Stats
  totalKeysCount = computed(() => this.apiKeys().length);
  activeKeysCount = computed(() => this.apiKeys().filter(k => !k.revoked).length);
  totalRequestsCount = computed(() => this.apiKeys().reduce((sum, k) => sum + (k.requestCount || 0), 0));

  filteredKeys = computed(() => {
    let list = this.apiKeys();
    const q = this.searchQuery().trim().toLowerCase();
    const filter = this.statusFilter();

    if (filter === 'active') {
      list = list.filter(k => !k.revoked);
    } else if (filter === 'revoked') {
      list = list.filter(k => k.revoked);
    }

    if (q) {
      list = list.filter(k => 
        k.name.toLowerCase().includes(q) || 
        k.key.toLowerCase().includes(q) ||
        k.role.toLowerCase().includes(q) ||
        (k.allowedBuckets || []).some(b => b.toLowerCase().includes(q))
      );
    }

    return list;
  });

  ngOnInit(): void {
    this.route.paramMap.subscribe(params => {
      const pid = params.get('projectId');
      if (pid) this.projectId.set(pid);
      this.loadAPIKeys();
      this.loadBuckets();
    });
  }

  loadAPIKeys(): void {
    this.goStore.listAPIKeys(this.projectId()).subscribe({
      next: (keys) => this.apiKeys.set(keys || []),
      error: (err) => console.error('Failed to load API keys', err)
    });
  }

  loadBuckets(): void {
    this.goStore.listBuckets().subscribe({
      next: (buckets) => this.availableBuckets.set(buckets || []),
      error: () => {}
    });
  }

  toggleBucketSelection(bucketName: string): void {
    const set = new Set(this.selectedBuckets());
    if (set.has(bucketName)) {
      set.delete(bucketName);
    } else {
      set.add(bucketName);
    }
    this.selectedBuckets.set(set);
    this.allowAllBuckets.set(false);
  }

  setAllowAllBuckets(val: boolean): void {
    this.allowAllBuckets.set(val);
    if (val) {
      this.selectedBuckets.set(new Set());
    }
  }

  openCreateKeyModal(): void {
    this.newKeyName.set('');
    this.newKeyRole.set('read-write');
    this.newKeyExpiresDays.set(0);
    this.newKeyRateLimit.set(0);
    this.allowAllBuckets.set(true);
    this.selectedBuckets.set(new Set());
    this.showNewKeyModal.set(true);
  }

  createKey(): void {
    const name = this.newKeyName().trim();
    if (!name) {
      this.notify.warning('Please enter a key name or client identifier.');
      return;
    }

    const allowedBuckets = this.allowAllBuckets() ? [] : Array.from(this.selectedBuckets());

    this.goStore.createAPIKey({
      projectId: this.projectId(),
      name,
      role: this.newKeyRole(),
      allowedBuckets,
      expiresInDays: this.newKeyExpiresDays(),
      rateLimitReqPerMin: this.newKeyRateLimit()
    }).subscribe({
      next: (created) => {
        this.showNewKeyModal.set(false);
        this.newlyCreatedKey.set(created);
        this.loadAPIKeys();
        this.notify.success(`API Key "${name}" generated!`);
      },
      error: (err) => this.notify.error('Failed to create API key: ' + (err.error?.error?.message || err.message))
    });
  }

  toggleRevealKey(id: string): void {
    const set = new Set(this.revealedKeys());
    if (set.has(id)) {
      set.delete(id);
    } else {
      set.add(id);
    }
    this.revealedKeys.set(set);
  }

  async rotateKey(k: APIKey): Promise<void> {
    const confirmed = await this.notify.confirm({
      title: `Rotate Secret for "${k.name}"?`,
      message: 'Rotating will invalidate the current secret key immediately and generate a new one. Applications using the previous key will need to be updated.',
      confirmText: 'Rotate Secret',
      type: 'warning'
    });
    if (!confirmed) return;

    this.goStore.rotateAPIKey(k.id).subscribe({
      next: (updated) => {
        this.loadAPIKeys();
        this.newlyCreatedKey.set(updated);
        this.notify.success(`API Key secret for "${k.name}" has been rotated!`);
      },
      error: (err) => this.notify.error('Failed to rotate API key: ' + err.message)
    });
  }

  async toggleRevoke(k: APIKey): Promise<void> {
    const action = k.revoked ? 'Re-activate' : 'Revoke';
    const confirmed = await this.notify.confirm({
      title: `${action} API Key?`,
      message: k.revoked
        ? `Re-activate key "${k.name}"? Client applications will regain access immediately.`
        : `Revoke key "${k.name}"? Client requests with this key will be rejected until re-activated.`,
      confirmText: action,
      type: k.revoked ? 'primary' : 'warning'
    });
    if (!confirmed) return;

    this.goStore.toggleRevokeAPIKey(k.id, !k.revoked).subscribe({
      next: () => {
        this.loadAPIKeys();
        this.notify.success(`Key "${k.name}" status updated.`);
      },
      error: (err) => this.notify.error(`Failed to ${action.toLowerCase()} key: ` + err.message)
    });
  }

  async deleteKey(k: APIKey): Promise<void> {
    const confirmed = await this.notify.confirm({
      title: 'Delete API Key?',
      message: `Permanently delete API Key "${k.name}"? This credential will be completely removed from the server.`,
      confirmText: 'Delete Key',
      type: 'danger'
    });
    if (!confirmed) return;

    this.goStore.deleteAPIKey(k.id).subscribe({
      next: () => {
        this.loadAPIKeys();
        this.notify.success('API Key permanently deleted.');
      },
      error: (err) => this.notify.error('Failed to delete key: ' + err.message)
    });
  }

  openSnippetModal(k: APIKey): void {
    this.snippetTargetKey.set(k);
  }

  getBaseApiUrl(): string {
    return this.goStore.getBaseUrl() || (typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8080');
  }

  getSnippetCurl(key: APIKey): string {
    const url = this.getBaseApiUrl();
    return `# 1. Upload Object via REST API
curl -X POST "${url}/v0/b/default/o?name=avatar.png" \\
  -H "Authorization: Bearer ${key.key}" \\
  -H "Content-Type: image/png" \\
  --data-binary "@avatar.png"

# 2. List Objects in Bucket
curl -X GET "${url}/api/v1/b/default/o" \\
  -H "Authorization: Bearer ${key.key}"`;
  }

  getSnippetTS(key: APIKey): string {
    const url = this.getBaseApiUrl();
    return `import { ProleadFileClient } from '@prolead/file-sdk';

const client = new ProleadFileClient({
  baseUrl: '${url}',
  apiKey: '${key.key}'
});

// Upload a file
const fileObj = await client.upload({
  bucket: 'default',
  path: 'documents/report.pdf',
  file: myFileBlob
});
console.log('Download URL:', fileObj.downloadUrl);`;
  }

  getSnippetGo(key: APIKey): string {
    const url = this.getBaseApiUrl();
    return `package main

import (
\t"context"
\t"fmt"
\t"os"
\tgoproleadfile "gostore/packages/go_prolead_file"
)

func main() {
\tclient := goproleadfile.NewClient("${url}", "${key.key}")
\tf, _ := os.Open("image.png")
\tdefer f.Close()

\tobj, err := client.Upload(context.Background(), "default", "images/image.png", "image/png", f)
\tif err != nil {
\t\tpanic(err)
\t}
\tfmt.Println("Uploaded:", obj.DownloadURL)
}`;
  }

  getSnippetPython(key: APIKey): string {
    const url = this.getBaseApiUrl();
    return `import requests

BASE_URL = "${url}"
API_KEY = "${key.key}"

headers = {"Authorization": f"Bearer {API_KEY}"}

# List files
res = requests.get(f"{BASE_URL}/api/v1/b/default/o", headers=headers)
print("Files:", res.json())`;
  }

  getSnippetFlutter(key: APIKey): string {
    const url = this.getBaseApiUrl();
    return `import 'package:flutter_prolead_file/flutter_prolead_file.dart';

final client = ProleadFileClient(
  baseUrl: '${url}',
  apiKey: '${key.key}',
);

// Display image widget with caching & on-the-fly thumbnail resizing
Widget buildAvatar() {
  return ProleadImage(
    client: client,
    bucket: 'default',
    path: 'images/profile.jpg',
    width: 120,
    height: 120,
  );
}`;
  }

  getSnippetS3(key: APIKey): string {
    const url = this.getBaseApiUrl();
    return `# Configure AWS S3 CLI for Prolead File S3 Gateway
export AWS_ACCESS_KEY_ID="${key.key}"
export AWS_SECRET_ACCESS_KEY="unused"
export AWS_DEFAULT_REGION="us-east-1"

# List S3 Bucket
aws --endpoint-url ${url}/s3 s3 ls s3://default/

# Upload file via S3 Gateway
aws --endpoint-url ${url}/s3 s3 cp ./local-doc.pdf s3://default/docs/`;
  }

  getActiveSnippetCode(): string {
    const k = this.snippetTargetKey();
    if (!k) return '';
    switch (this.activeSnippetTab()) {
      case 'curl': return this.getSnippetCurl(k);
      case 'ts': return this.getSnippetTS(k);
      case 'go': return this.getSnippetGo(k);
      case 'python': return this.getSnippetPython(k);
      case 'flutter': return this.getSnippetFlutter(k);
      case 's3': return this.getSnippetS3(k);
      default: return '';
    }
  }

  copyActiveSnippet(): void {
    this.copyText(this.getActiveSnippetCode(), 'Code snippet copied to clipboard!');
  }

  copyText(text: string, msg = 'Copied to clipboard!'): void {
    navigator.clipboard.writeText(text);
    this.notify.success(msg);
  }
}
