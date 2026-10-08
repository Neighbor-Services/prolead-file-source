import { Component, inject, signal, computed, OnInit, OnDestroy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { GoStoreService } from '../../services/gostore.service';
import { NotificationService } from '../../services/notification.service';
import { DedupReport, AuditLogEntry, WorkerPoolStats, WorkerJobRecord, JobType } from '../../services/gostore.models';

@Component({
  selector: 'app-operations',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './operations.component.html',
  styleUrl: './operations.component.css'
})
export class OperationsComponent implements OnInit, OnDestroy {
  private goStore = inject(GoStoreService);
  private notify = inject(NotificationService);

  // Active Tab
  activeTab = signal<'workers' | 'dedup' | 'audit' | 'gateways'>('workers');

  // Background Worker Pool State
  workerStats = signal<WorkerPoolStats>({
    activeWorkers: 0,
    totalWorkers: 4,
    maxWorkers: 32,
    queueLength: 0,
    queueCapacity: 500,
    jobsProcessed: 0,
    jobsSucceeded: 0,
    jobsFailed: 0,
    uptimeSeconds: 0,
    throughputPerMin: 0,
    avgDurationMs: 0
  });

  workerJobs = signal<WorkerJobRecord[]>([]);
  isLoadingJobs = signal<boolean>(false);
  jobStatusFilter = signal<'ALL' | 'PROCESSING' | 'COMPLETED' | 'FAILED' | 'QUEUED'>('ALL');
  jobSearchQuery = signal<string>('');
  isScaling = signal<boolean>(false);
  targetConcurrency = signal<number>(4);
  autoRefreshTimer: any = null;

  // Dedup & Audit State
  dedupReport = signal<DedupReport | null>(null);
  auditLogs = signal<AuditLogEntry[]>([]);
  totalAuditLogs = signal<number>(0);
  loadingAudit = signal<boolean>(false);
  actionFilter = signal<string>('');
  
  // Status message
  operationStatus = signal<string | null>(null);
  isOperating = signal<boolean>(false);

  // Computed filtered jobs
  filteredJobs = computed(() => {
    let list = this.workerJobs();
    const filter = this.jobStatusFilter();
    const q = this.jobSearchQuery().trim().toLowerCase();

    if (filter !== 'ALL') {
      list = list.filter(j => j.status === filter);
    }

    if (q) {
      list = list.filter(j => 
        j.id.toLowerCase().includes(q) ||
        j.type.toLowerCase().includes(q) ||
        (j.bucket || '').toLowerCase().includes(q) ||
        (j.path || '').toLowerCase().includes(q) ||
        (j.error || '').toLowerCase().includes(q)
      );
    }

    return list;
  });

  ngOnInit(): void {
    this.loadWorkerStats();
    this.loadWorkerJobs();
    this.loadDedupReport();
    this.loadAuditLogs();

    // Start auto-refresh polling for worker engine metrics every 3 seconds
    this.autoRefreshTimer = setInterval(() => {
      this.loadWorkerStats();
      if (this.activeTab() === 'workers') {
        this.loadWorkerJobs(false);
      }
    }, 3000);
  }

  ngOnDestroy(): void {
    if (this.autoRefreshTimer) {
      clearInterval(this.autoRefreshTimer);
    }
  }

  loadWorkerStats(): void {
    this.goStore.getWorkerStats().subscribe({
      next: (stats) => {
        if (stats) {
          this.workerStats.set(stats);
          this.targetConcurrency.set(stats.totalWorkers);
        }
      },
      error: () => {}
    });
  }

  loadWorkerJobs(showLoading = true): void {
    if (showLoading) this.isLoadingJobs.set(true);
    this.goStore.getWorkerJobs(100).subscribe({
      next: (jobs) => {
        this.workerJobs.set(jobs || []);
        this.isLoadingJobs.set(false);
      },
      error: () => {
        this.isLoadingJobs.set(false);
      }
    });
  }

  scaleWorkers(count: number): void {
    if (count <= 0 || count > 32) return;
    this.isScaling.set(true);
    this.targetConcurrency.set(count);

    this.goStore.scaleWorkerPool(count).subscribe({
      next: (res) => {
        this.isScaling.set(false);
        this.notify.success(`Worker concurrency scaled to ${res.totalWorkers} workers.`);
        this.loadWorkerStats();
      },
      error: (err) => {
        this.isScaling.set(false);
        this.notify.error('Failed to scale workers: ' + err.message);
      }
    });
  }

  triggerWorkerJob(type: JobType, label: string, bucket = '', path = ''): void {
    this.isOperating.set(true);
    this.operationStatus.set(`Enqueuing background task: ${label}...`);

    this.goStore.triggerWorkerJob(type, bucket, path).subscribe({
      next: (res) => {
        this.isOperating.set(false);
        this.notify.success(`Job enqueued (${label}). Job ID: ${res.jobId.substring(0, 8)}...`);
        this.operationStatus.set(`Task "${label}" dispatched to worker pool.`);
        this.loadWorkerStats();
        this.loadWorkerJobs();
      },
      error: (err) => {
        this.isOperating.set(false);
        this.notify.error(`Failed to trigger ${label}: ` + err.message);
      }
    });
  }

  clearHistory(): void {
    this.goStore.clearWorkerHistory().subscribe({
      next: () => {
        this.notify.success('Worker job history cleared.');
        this.loadWorkerJobs();
      },
      error: (err) => this.notify.error('Failed to clear history: ' + err.message)
    });
  }

  loadDedupReport(): void {
    this.goStore.getDedupReport().subscribe({
      next: (rep) => this.dedupReport.set(rep),
      error: (err) => console.error('Failed to load dedup report', err)
    });
  }

  loadAuditLogs(): void {
    this.loadingAudit.set(true);
    this.goStore.getAuditLogs(50, 0, this.actionFilter()).subscribe({
      next: (res) => {
        this.auditLogs.set(res.items || []);
        this.totalAuditLogs.set(res.totalCount || 0);
        this.loadingAudit.set(false);
      },
      error: (err) => {
        console.error('Failed to load audit logs', err);
        this.loadingAudit.set(false);
      }
    });
  }

  downloadHotBackup(): void {
    this.isOperating.set(true);
    this.operationStatus.set('Generating Hot SQLite & Storage Metadata snapshot archive...');
    this.goStore.downloadBackup().subscribe({
      next: (blob) => {
        this.isOperating.set(false);
        this.operationStatus.set('Backup successfully generated and downloaded.');
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `prolead-hot-backup-${Date.now()}.zip`;
        a.click();
        window.URL.revokeObjectURL(url);
      },
      error: (err) => {
        this.isOperating.set(false);
        this.operationStatus.set('Backup Export Failed: ' + err.message);
      }
    });
  }

  onRestoreFileSelected(event: any): void {
    const file = event.target.files[0];
    if (!file) return;

    this.isOperating.set(true);
    this.operationStatus.set(`Uploading and restoring from snapshot ${file.name}...`);
    this.goStore.restoreBackup(file).subscribe({
      next: (res) => {
        this.isOperating.set(false);
        this.operationStatus.set('System successfully restored: ' + res.message);
        this.loadDedupReport();
        this.loadAuditLogs();
        this.loadWorkerJobs();
      },
      error: (err) => {
        this.isOperating.set(false);
        this.operationStatus.set('Snapshot Restore Failed: ' + err.message);
      }
    });
  }

  formatBytes(bytes: number): string {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  formatUptime(sec: number): string {
    const d = Math.floor(sec / (3600 * 24));
    const h = Math.floor((sec % (3600 * 24)) / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    if (d > 0) return `${d}d ${h}h ${m}m`;
    if (h > 0) return `${h}h ${m}m ${s}s`;
    return `${m}m ${s}s`;
  }

  getBaseUrl(): string {
    return this.goStore.getBaseUrl() || (typeof window !== 'undefined' ? window.location.origin : 'https://file.proleadsolutions.co');
  }
}
