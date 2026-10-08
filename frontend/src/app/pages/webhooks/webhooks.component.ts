import { Component, inject, signal, computed, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { GoStoreService } from '../../services/gostore.service';
import { NotificationService } from '../../services/notification.service';
import { WebhookConfig, WebhookDelivery, TestWebhookResult } from '../../services/gostore.models';

export interface EventOption {
  id: string;
  name: string;
  desc: string;
}

@Component({
  selector: 'app-webhooks',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './webhooks.component.html',
  styleUrl: './webhooks.component.css'
})
export class WebhooksComponent implements OnInit {
  private goStore = inject(GoStoreService);
  private notify = inject(NotificationService);
  private route = inject(ActivatedRoute);

  projectId = signal<string>('p-default');
  webhooks = signal<WebhookConfig[]>([]);
  searchQuery = signal<string>('');
  statusFilter = signal<'all' | 'active' | 'paused'>('all');
  revealedSecrets = signal<Set<string>>(new Set());

  // Modal: Register Webhook
  showNewWebhookModal = signal<boolean>(false);
  newWebhookName = signal<string>('');
  newWebhookUrl = signal<string>('');
  newWebhookSecret = signal<string>('');
  selectedEvents = signal<Set<string>>(new Set(['*']));
  isSubmitting = signal<boolean>(false);

  // Modal: Test Ping Live Result
  testingWebhookId = signal<string | null>(null);
  testResult = signal<TestWebhookResult | null>(null);
  isTesting = signal<boolean>(false);

  // Modal: Delivery History Log
  historyTargetWebhook = signal<WebhookConfig | null>(null);
  deliveries = signal<WebhookDelivery[]>([]);
  isLoadingDeliveries = signal<boolean>(false);
  selectedDelivery = signal<WebhookDelivery | null>(null);

  // Modal: HMAC Verification Snippets
  verifySnippetTarget = signal<WebhookConfig | null>(null);
  activeVerifyLang = signal<'node' | 'go' | 'python' | 'php'>('node');

  // Available event types
  availableEvents: EventOption[] = [
    { id: '*', name: 'Wildcard: All Events (*)', desc: 'Receive all system notifications and storage lifecycle events' },
    { id: 'file.created', name: 'file.created', desc: 'Fired when a new object is uploaded or chunk assembled' },
    { id: 'file.deleted', name: 'file.deleted', desc: 'Fired when an object is soft-deleted or permanently removed' },
    { id: 'file.restored', name: 'file.restored', desc: 'Fired when a file is restored from trash directory' },
    { id: 'bucket.created', name: 'bucket.created', desc: 'Fired when a new storage bucket container is provisioned' },
    { id: 'bucket.deleted', name: 'bucket.deleted', desc: 'Fired when a storage bucket is removed' },
    { id: 'token.rotated', name: 'token.rotated', desc: 'Fired when a file access download token is regenerated' },
    { id: 'key.created', name: 'key.created', desc: 'Fired when a new API access key is provisioned' },
    { id: 'key.revoked', name: 'key.revoked', desc: 'Fired when an API access key is revoked or invalidated' },
    { id: 'lifecycle.sweep', name: 'lifecycle.sweep', desc: 'Fired when automated lifecycle retention rule purge completes' },
  ];

  // Computed KPIs
  totalWebhooksCount = computed(() => this.webhooks().length);
  activeWebhooksCount = computed(() => this.webhooks().filter(w => w.enabled).length);
  totalDeliveriesCount = computed(() => this.webhooks().reduce((acc, w) => acc + (w.successCount || 0) + (w.failureCount || 0), 0));
  overallSuccessRate = computed(() => {
    let success = 0;
    let total = 0;
    for (const w of this.webhooks()) {
      success += (w.successCount || 0);
      total += (w.successCount || 0) + (w.failureCount || 0);
    }
    if (total === 0) return '100%';
    return Math.round((success / total) * 100) + '%';
  });

  filteredWebhooks = computed(() => {
    let list = this.webhooks();
    const q = this.searchQuery().trim().toLowerCase();
    const filter = this.statusFilter();

    if (filter === 'active') {
      list = list.filter(w => w.enabled);
    } else if (filter === 'paused') {
      list = list.filter(w => !w.enabled);
    }

    if (q) {
      list = list.filter(w =>
        (w.name || '').toLowerCase().includes(q) ||
        w.url.toLowerCase().includes(q) ||
        (w.events || []).some(e => e.toLowerCase().includes(q))
      );
    }

    return list;
  });

  ngOnInit(): void {
    this.route.paramMap.subscribe(params => {
      const pid = params.get('projectId');
      if (pid) this.projectId.set(pid);
      this.loadWebhooks();
    });
  }

  loadWebhooks(): void {
    this.goStore.listWebhooks(this.projectId()).subscribe({
      next: (list) => this.webhooks.set(list || []),
      error: (err) => console.error('Failed to load webhooks', err)
    });
  }

  openRegisterModal(): void {
    this.newWebhookName.set('');
    this.newWebhookUrl.set('');
    this.newWebhookSecret.set('');
    this.selectedEvents.set(new Set(['*']));
    this.showNewWebhookModal.set(true);
  }

  toggleEventSelection(eventId: string): void {
    const set = new Set(this.selectedEvents());
    if (eventId === '*') {
      if (set.has('*')) {
        set.clear();
      } else {
        set.clear();
        set.add('*');
      }
    } else {
      set.delete('*');
      if (set.has(eventId)) {
        set.delete(eventId);
      } else {
        set.add(eventId);
      }
      if (set.size === 0) {
        set.add('*');
      }
    }
    this.selectedEvents.set(set);
  }

  createWebhook(): void {
    const url = this.newWebhookUrl().trim();
    if (!url) {
      this.notify.warning('Please enter a destination webhook URL.');
      return;
    }

    this.isSubmitting.set(true);
    const events = Array.from(this.selectedEvents());

    this.goStore.registerWebhook({
      projectId: this.projectId(),
      name: this.newWebhookName().trim() || 'Webhook Listener',
      url,
      secret: this.newWebhookSecret().trim(),
      events,
      enabled: true
    }).subscribe({
      next: (created) => {
        this.isSubmitting.set(false);
        this.showNewWebhookModal.set(false);
        this.loadWebhooks();
        this.notify.success(`Webhook "${created.name || url}" registered successfully!`);
      },
      error: (err) => {
        this.isSubmitting.set(false);
        this.notify.error('Failed to register webhook: ' + err.message);
      }
    });
  }

  toggleWebhookStatus(wh: WebhookConfig): void {
    const targetState = !wh.enabled;
    this.goStore.toggleWebhook(wh.id, targetState).subscribe({
      next: () => {
        wh.enabled = targetState;
        this.notify.success(`Webhook ${targetState ? 'activated' : 'paused'}.`);
        this.loadWebhooks();
      },
      error: (err) => this.notify.error('Failed to update webhook status: ' + err.message)
    });
  }

  toggleSecretVisibility(id: string): void {
    const set = new Set(this.revealedSecrets());
    if (set.has(id)) {
      set.delete(id);
    } else {
      set.add(id);
    }
    this.revealedSecrets.set(set);
  }

  testPing(wh: WebhookConfig): void {
    this.testingWebhookId.set(wh.id);
    this.testResult.set(null);
    this.isTesting.set(true);

    this.goStore.testWebhook(wh.id).subscribe({
      next: (res) => {
        this.isTesting.set(false);
        this.testResult.set(res);
        this.loadWebhooks();
        if (res.success) {
          this.notify.success(`Test ping succeeded (${res.statusCode} in ${res.durationMs}ms)`);
        } else {
          this.notify.warning(`Test ping responded with status: ${res.statusCode || 'Error'} (${res.durationMs}ms)`);
        }
      },
      error: (err) => {
        this.isTesting.set(false);
        this.notify.error('Failed to send test ping: ' + err.message);
      }
    });
  }

  openDeliveries(wh: WebhookConfig): void {
    this.historyTargetWebhook.set(wh);
    this.isLoadingDeliveries.set(true);
    this.deliveries.set([]);
    this.selectedDelivery.set(null);

    this.goStore.listWebhookDeliveries(wh.id).subscribe({
      next: (dels) => {
        this.deliveries.set(dels || []);
        this.isLoadingDeliveries.set(false);
        if (dels && dels.length > 0) {
          this.selectedDelivery.set(dels[0]);
        }
      },
      error: (err) => {
        this.isLoadingDeliveries.set(false);
        this.notify.error('Failed to load delivery logs: ' + err.message);
      }
    });
  }

  redeliver(delivery: WebhookDelivery): void {
    const wh = this.historyTargetWebhook();
    if (!wh) return;

    this.notify.info(`Re-dispatching event ${delivery.event}...`);
    this.goStore.redeliverWebhook(wh.id, delivery.id).subscribe({
      next: (res) => {
        if (res.success) {
          this.notify.success(`Redelivery succeeded (${res.statusCode} in ${res.durationMs}ms)`);
        } else {
          this.notify.warning(`Redelivery finished with status ${res.statusCode || 'Error'}`);
        }
        this.openDeliveries(wh);
        this.loadWebhooks();
      },
      error: (err) => this.notify.error('Failed to redeliver: ' + err.message)
    });
  }

  openVerifyModal(wh: WebhookConfig): void {
    this.verifySnippetTarget.set(wh);
  }

  getVerifySnippetCode(): string {
    const wh = this.verifySnippetTarget();
    const secret = wh?.secret || 'whsec_your_signing_secret_here';

    switch (this.activeVerifyLang()) {
      case 'node':
        return `const crypto = require('crypto');

// Express.js Webhook Verification Middleware
function verifyProleadSignature(req, res, next) {
  const signature = req.headers['x-proleadfile-signature'] || req.headers['x-gostore-signature'];
  const secret = '${secret}';

  if (!signature) {
    return res.status(401).send('Missing signature header');
  }

  // Compute HMAC SHA256 of raw request body
  const expected = 'sha256=' + crypto
    .createHmac('sha256', secret)
    .update(req.rawBody || JSON.stringify(req.body))
    .digest('hex');

  if (crypto.timingSafeEqual(Buffer.from(signature), Buffer.from(expected))) {
    return next(); // Verified!
  }
  return res.status(403).send('Invalid webhook HMAC signature');
}`;

      case 'go':
        return `package main

import (
\t"crypto/hmac"
\t"crypto/sha256"
\t"encoding/hex"
\t"fmt"
\t"io"
\t"net/http"
)

func VerifyWebhook(secret string, r *http.Request) (bool, error) {
\tsigHeader := r.Header.Get("X-ProleadFile-Signature")
\tbodyBytes, err := io.ReadAll(r.Body)
\tif err != nil {
\t\treturn false, err
\t}

\tmac := hmac.New(sha256.New, []byte(secret))
\tmac.Write(bodyBytes)
\texpected := fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))

\treturn hmac.Equal([]byte(sigHeader), []byte(expected)), nil
}`;

      case 'python':
        return `import hmac
import hashlib
from flask import Flask, request, abort

app = Flask(__name__)
SECRET = "${secret}".encode("utf-8")

@app.route("/webhooks/prolead", methods=["POST"])
def handle_webhook():
    signature = request.headers.get("X-ProleadFile-Signature", "")
    payload = request.get_data()

    expected_sig = "sha256=" + hmac.new(SECRET, payload, hashlib.sha256).hexdigest()

    if not hmac.compare_digest(signature, expected_sig):
        abort(403, "Invalid HMAC signature")

    event_data = request.json
    print(f"Received verified event: {request.headers.get('X-ProleadFile-Event')}")
    return {"received": True}, 200`;

      case 'php':
        return `<?php
$secret = "${secret}";
$signature = $_SERVER['HTTP_X_PROLEADFILE_SIGNATURE'] ?? '';
$payload = file_get_contents('php://input');

$expected = 'sha256=' . hash_hmac('sha256', $payload, $secret);

if (!hash_equals($signature, $expected)) {
    http_response_code(403);
    echo "Invalid signature";
    exit;
}

$event = json_decode($payload, true);
// Process event...
http_response_code(200);
echo json_encode(["status" => "ok"]);
?>`;
      default:
        return '';
    }
  }

  async deleteWebhook(wh: WebhookConfig): Promise<void> {
    const confirmed = await this.notify.confirm({
      title: `Delete Webhook Listener?`,
      message: `Permanently delete webhook "${wh.name || wh.url}"? Microservices listening at this endpoint will no longer receive storage notifications.`,
      confirmText: 'Delete Listener',
      type: 'danger'
    });
    if (!confirmed) return;

    this.goStore.deleteWebhook(wh.id).subscribe({
      next: () => {
        this.loadWebhooks();
        this.notify.success('Webhook listener removed.');
      },
      error: (err) => this.notify.error('Failed to delete webhook: ' + err.message)
    });
  }

  copyText(text: string, msg = 'Copied to clipboard!'): void {
    navigator.clipboard.writeText(text);
    this.notify.success(msg);
  }
}
