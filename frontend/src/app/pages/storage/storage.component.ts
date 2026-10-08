import { Component, inject, signal, computed, OnInit, OnDestroy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { DomSanitizer, SafeResourceUrl } from '@angular/platform-browser';
import { Subscription } from 'rxjs';
import { GoStoreService } from '../../services/gostore.service';
import { NotificationService } from '../../services/notification.service';
import { StorageObject, FileVersion, StorageBucket, StorageStats, ShareLink, LifecycleRule } from '../../services/gostore.models';

interface UploadTask {
  filename: string;
  progress: number;
}

@Component({
  selector: 'app-storage',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './storage.component.html',
  styleUrl: './storage.component.css'
})
export class StorageComponent implements OnInit, OnDestroy {
  private goStore = inject(GoStoreService);
  private notify = inject(NotificationService);
  private route = inject(ActivatedRoute);
  private sanitizer = inject(DomSanitizer);
  private routeSub?: Subscription;

  projectId = signal<string>('p-default');
  activeSubView = signal<'files' | 'trash'>('files');
  viewMode = signal<'table' | 'grid'>('table');
  activeCategory = signal<string>('all');
  sortField = signal<'name' | 'size' | 'updatedAt' | 'contentType'>('name');
  sortDirection = signal<'asc' | 'desc'>('asc');

  buckets = signal<StorageBucket[]>([]);
  currentBucket = signal<string>('default');
  currentPrefix = signal<string>('');
  files = signal<StorageObject[]>([]);
  folderPrefixes = signal<string[]>([]);
  stats = signal<StorageStats | null>(null);
  searchQuery = signal<string>('');

  selectedFilePaths = signal<Set<string>>(new Set());
  activeUploads = signal<UploadTask[]>([]);
  isDraggingOver = signal<boolean>(false);

  // Inspector / Details Side Drawer
  inspectedItem = signal<StorageObject | null>(null);

  // File Detail & Image Resizer Modal
  selectedFile = signal<StorageObject | null>(null);
  transformW = signal<number>(0);
  transformH = signal<number>(0);
  transformFormat = signal<'webp' | 'jpeg' | 'png' | 'gif'>('webp');
  transformFit = signal<'cover' | 'contain' | 'fill' | 'scale'>('cover');
  transformQuality = signal<number>(85);

  // Expiring Signed Link Modal
  showSignedModal = signal<boolean>(false);
  targetSignFile = signal<StorageObject | null>(null);
  generatedSignedUrl = signal<string>('');
  signDuration = 3600;

  // Version History Modal
  showVersionModal = signal<boolean>(false);
  targetVersionFile = signal<StorageObject | null>(null);
  fileVersions = signal<FileVersion[]>([]);

  // Create Bucket Modal
  showNewBucketModal = signal<boolean>(false);

  // Public Share Modal
  showShareModal = signal<boolean>(false);
  shareTargetFile = signal<StorageObject | null>(null);
  sharePassword = signal<string>('');
  shareMaxDownloads = signal<number>(0);
  shareExpiresDays = signal<number>(7);
  generatedShare = signal<ShareLink | null>(null);

  // Lifecycle & Retention Modal
  showLifecycleModal = signal<boolean>(false);
  lifecycleRule = signal<LifecycleRule>({
    bucket: 'default',
    trashDays: 30,
    versionLimit: 5,
    expirationDays: 0,
    enabled: true
  });

  // Rename Modal
  showRenameModal = signal<boolean>(false);
  renameTarget = signal<StorageObject | null>(null);
  renameTargetFolder = signal<string | null>(null);
  renameInput = signal<string>('');

  // Move / Copy Modal
  showMoveCopyModal = signal<boolean>(false);
  moveCopyTarget = signal<StorageObject | null>(null);
  moveCopyIsMove = signal<boolean>(true);
  moveCopyDstBucket = signal<string>('default');
  moveCopyDstPath = signal<string>('');

  // Edit Bucket Settings Modal
  showEditBucketModal = signal<boolean>(false);
  editBucketData = signal<{ name: string; description: string; isPublic: boolean; maxFileSize: number; allowedMimes: string }>({
    name: '',
    description: '',
    isPublic: false,
    maxFileSize: 0,
    allowedMimes: ''
  });

  // New Folder Modal
  showNewFolderModal = signal<boolean>(false);
  newFolderName = signal<string>('');

  // Filtered & Sorted files
  filteredFiles = computed(() => {
    let list = [...this.files()];
    const cat = this.activeCategory();

    if (cat === 'images') {
      list = list.filter(f => f.contentType.startsWith('image/'));
    } else if (cat === 'videos') {
      list = list.filter(f => f.contentType.startsWith('video/'));
    } else if (cat === 'audio') {
      list = list.filter(f => f.contentType.startsWith('audio/'));
    } else if (cat === 'documents') {
      list = list.filter(f => f.contentType.includes('pdf') || f.contentType.includes('word') || f.contentType.includes('document') || f.contentType.includes('sheet') || f.name.endsWith('.pdf') || f.name.endsWith('.docx') || f.name.endsWith('.xlsx'));
    } else if (cat === 'code') {
      list = list.filter(f => this.isTextOrCode(f.contentType) || f.name.endsWith('.json') || f.name.endsWith('.go') || f.name.endsWith('.ts') || f.name.endsWith('.js') || f.name.endsWith('.html') || f.name.endsWith('.css') || f.name.endsWith('.py'));
    } else if (cat === 'archives') {
      list = list.filter(f => f.contentType.includes('zip') || f.contentType.includes('tar') || f.contentType.includes('gzip') || f.name.endsWith('.zip') || f.name.endsWith('.tar.gz'));
    } else if (cat === 'folders') {
      list = [];
    }

    const field = this.sortField();
    const asc = this.sortDirection() === 'asc';

    list.sort((a, b) => {
      let cmp = 0;
      if (field === 'name') {
        cmp = a.name.localeCompare(b.name);
      } else if (field === 'size') {
        cmp = a.size - b.size;
      } else if (field === 'updatedAt') {
        cmp = new Date(a.updatedAt).getTime() - new Date(b.updatedAt).getTime();
      } else if (field === 'contentType') {
        cmp = (a.contentType || '').localeCompare(b.contentType || '');
      }
      return asc ? cmp : -cmp;
    });

    return list;
  });

  // Current Bucket Info & Quota
  currentBucketObj = computed(() => {
    return this.buckets().find(b => b.name === this.currentBucket()) || null;
  });

  totalBucketBytes = computed(() => {
    return this.files().reduce((acc, f) => acc + (f.size || 0), 0);
  });

  ngOnInit(): void {
    this.routeSub = this.route.paramMap.subscribe(params => {
      const pid = params.get('projectId');
      if (pid) {
        this.projectId.set(pid);
        this.goStore.listProjects().subscribe({
          next: (projs) => {
            const proj = (projs || []).find(p => p.id === pid || p.slug === pid);
            if (proj) {
              this.goStore.selectProject(proj);
            }
          },
          error: () => {}
        });
      }
      this.currentPrefix.set('');
      this.loadStats();
      this.loadBuckets();
    });
  }

  ngOnDestroy(): void {
    this.routeSub?.unsubscribe();
  }

  loadStats(): void {
    this.goStore.getStats().subscribe({
      next: (s) => this.stats.set(s),
      error: (err) => console.error('Failed to load stats', err)
    });
  }

  loadBuckets(): void {
    this.goStore.listBuckets(this.projectId()).subscribe({
      next: (b) => {
        this.buckets.set(b || []);
        if (b && b.length > 0) {
          const exists = b.some(x => x.name === this.currentBucket());
          if (!exists) {
            this.currentBucket.set(b[0].name);
          }
          this.loadFiles(this.currentPrefix());
        } else {
          this.files.set([]);
          this.folderPrefixes.set([]);
        }
      },
      error: (err) => console.error('Failed to load buckets', err)
    });
  }

  loadFiles(prefix: string = ''): void {
    this.currentPrefix.set(prefix);
    this.selectedFilePaths.set(new Set());
    const trashOnly = this.activeSubView() === 'trash';

    this.goStore.listFiles(this.currentBucket(), prefix, '/', 200, 0, this.searchQuery(), trashOnly).subscribe({
      next: (res) => {
        // Filter out internal folder marker files (.keep) from the visible file list
        const cleanFiles = (res.items || []).filter(f => !f.name.endsWith('.keep') && !f.path.endsWith('.keep'));
        this.files.set(cleanFiles);
        this.folderPrefixes.set(res.prefixes || []);

        // If inspected item is no longer in the list, close inspector
        if (this.inspectedItem()) {
          const found = cleanFiles.find(f => f.path === this.inspectedItem()!.path);
          this.inspectedItem.set(found || null);
        }
      },
      error: (err) => console.error('Failed to load files', err)
    });
  }

  getBreadcrumbs(): { name: string; prefix: string }[] {
    const prefix = this.currentPrefix();
    if (!prefix) return [];
    const parts = prefix.replace(/\/$/, '').split('/');
    const crumbs: { name: string; prefix: string }[] = [];
    let acc = '';
    for (const part of parts) {
      acc += `${part}/`;
      crumbs.push({ name: part, prefix: acc });
    }
    return crumbs;
  }

  navigateUp(): void {
    const prefix = this.currentPrefix();
    if (!prefix) return;
    const parts = prefix.replace(/\/$/, '').split('/');
    parts.pop();
    const parentPrefix = parts.length > 0 ? `${parts.join('/')}/` : '';
    this.loadFiles(parentPrefix);
  }

  selectBucket(bucketName: string): void {
    this.currentBucket.set(bucketName);
    this.currentPrefix.set('');
    this.searchQuery.set('');
    this.inspectedItem.set(null);
    this.loadFiles('');
  }

  switchSubView(view: 'files' | 'trash'): void {
    this.activeSubView.set(view);
    this.inspectedItem.set(null);
    this.loadFiles('');
  }

  setCategory(cat: string): void {
    this.activeCategory.set(cat);
  }

  toggleSort(field: 'name' | 'size' | 'updatedAt' | 'contentType'): void {
    if (this.sortField() === field) {
      this.sortDirection.set(this.sortDirection() === 'asc' ? 'desc' : 'asc');
    } else {
      this.sortField.set(field);
      this.sortDirection.set('asc');
    }
  }

  onSearchInput(event: Event): void {
    const query = (event.target as HTMLInputElement).value;
    this.searchQuery.set(query);
    this.loadFiles(this.currentPrefix());
  }

  clearSearch(): void {
    this.searchQuery.set('');
    this.loadFiles(this.currentPrefix());
  }

  getFolderName(prefix: string): string {
    const clean = prefix.substring(this.currentPrefix().length);
    return clean.replace(/\/$/, '');
  }

  getThumbnailUrl(rawUrl: string): string {
    return this.goStore.getTransformedImageUrl(rawUrl, {
      width: 120,
      height: 120,
      fit: 'cover'
    });
  }

  getCustomTransformedUrl(): string {
    if (!this.selectedFile()) return '';
    const w = this.transformW();
    const h = this.transformH();
    const fmt = this.transformFormat();
    const fit = this.transformFit();
    const q = this.transformQuality();

    return this.goStore.getTransformedImageUrl(this.selectedFile()!.downloadUrl, {
      width: w > 0 ? w : undefined,
      height: h > 0 ? h : undefined,
      format: fmt,
      fit: fit,
      quality: q
    });
  }

  setTransform(w: number, h: number): void {
    this.transformW.set(w);
    this.transformH.set(h);
  }

  // --- Inspector Drawer ---
  inspectItem(item: StorageObject, event?: Event): void {
    if (event) {
      event.stopPropagation();
    }
    this.inspectedItem.set(item);
  }

  closeInspector(): void {
    this.inspectedItem.set(null);
  }

  // --- Selection & Batch Operations ---
  toggleSelectFile(path: string, event?: Event): void {
    if (event) {
      event.stopPropagation();
    }
    const current = new Set(this.selectedFilePaths());
    if (current.has(path)) {
      current.delete(path);
    } else {
      current.add(path);
    }
    this.selectedFilePaths.set(current);
  }

  clearSelection(): void {
    this.selectedFilePaths.set(new Set());
  }

  toggleSelectAll(): void {
    if (this.isAllSelected()) {
      this.selectedFilePaths.set(new Set());
    } else {
      const all = new Set(this.filteredFiles().map(f => f.path));
      this.selectedFilePaths.set(all);
    }
  }

  isAllSelected(): boolean {
    const files = this.filteredFiles();
    return files.length > 0 && files.every(f => this.selectedFilePaths().has(f.path));
  }

  downloadSelectedAsZip(): void {
    const paths = Array.from(this.selectedFilePaths());
    if (paths.length === 0) return;

    this.goStore.downloadZip(this.currentBucket(), paths).subscribe({
      next: (blob) => {
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `${this.currentBucket()}-selected-archive.zip`;
        a.click();
        window.URL.revokeObjectURL(url);
        this.notify.success('ZIP archive downloaded successfully!');
      },
      error: (err) => this.notify.error('Failed to download ZIP: ' + err.message)
    });
  }

  async deleteSelectedFiles(): Promise<void> {
    const paths = Array.from(this.selectedFilePaths());
    if (paths.length === 0) return;

    const purge = this.activeSubView() === 'trash';
    const confirmed = await this.notify.confirm({
      title: purge ? 'Permanently Purge Files?' : 'Move to Trash?',
      message: `Are you sure you want to ${purge ? 'permanently purge' : 'move to trash'} ${paths.length} selected item(s)?`,
      confirmText: purge ? 'Purge Files' : 'Move to Trash',
      type: 'danger'
    });
    if (!confirmed) return;

    let deleted = 0;
    paths.forEach(p => {
      this.goStore.deleteFile(this.currentBucket(), p, purge).subscribe({
        next: () => {
          deleted++;
          if (deleted === paths.length) {
            this.selectedFilePaths.set(new Set());
            this.loadFiles(this.currentPrefix());
            this.loadStats();
            this.notify.success(`${deleted} file(s) removed.`);
          }
        },
        error: (err) => this.notify.error('Delete error: ' + err.message)
      });
    });
  }

  // --- Drag & Drop & Uploads ---
  onFileSelected(event: Event): void {
    const input = event.target as HTMLInputElement;
    if (!input.files || input.files.length === 0) return;

    for (let i = 0; i < input.files.length; i++) {
      this.uploadSingleFile(input.files[i]);
    }
    input.value = '';
  }

  onDragOver(e: DragEvent): void {
    e.preventDefault();
    this.isDraggingOver.set(true);
  }

  onDragLeave(e: DragEvent): void {
    e.preventDefault();
    this.isDraggingOver.set(false);
  }

  onDrop(e: DragEvent): void {
    e.preventDefault();
    this.isDraggingOver.set(false);
    if (!e.dataTransfer || !e.dataTransfer.files) return;

    for (let i = 0; i < e.dataTransfer.files.length; i++) {
      this.uploadSingleFile(e.dataTransfer.files[i]);
    }
  }

  private uploadSingleFile(file: File): void {
    const destPath = this.currentPrefix() ? `${this.currentPrefix()}${file.name}` : file.name;
    const task: UploadTask = { filename: file.name, progress: 0 };
    this.activeUploads.update(prev => [...prev, task]);

    this.goStore.uploadWithProgress(file, destPath, this.currentBucket()).subscribe({
      next: (event) => {
        this.activeUploads.update(list => 
          list.map(t => t.filename === file.name ? { ...t, progress: event.progress } : t)
        );
        if (event.status === 'completed') {
          setTimeout(() => {
            this.activeUploads.update(list => list.filter(t => t.filename !== file.name));
          }, 1500);
          this.loadFiles(this.currentPrefix());
          this.loadStats();
          this.notify.success(`Uploaded "${file.name}" successfully!`);
        }
      },
      error: (err) => {
        console.error('Upload error', err);
        this.notify.error(`Upload failed: ${err.error?.error?.message || err.message}`);
        this.activeUploads.update(list => list.filter(t => t.filename !== file.name));
      }
    });
  }

  openDetailModal(f: StorageObject, event?: Event): void {
    if (event) {
      event.stopPropagation();
    }
    this.selectedFile.set(f);
    this.transformW.set(0);
    this.transformH.set(0);
  }

  closeDetailModal(): void {
    this.selectedFile.set(null);
  }

  rotateToken(f?: StorageObject): void {
    const target = f || this.selectedFile() || this.inspectedItem();
    if (!target) return;

    this.goStore.rotateDownloadToken(target.bucket, target.path).subscribe({
      next: (updated) => {
        if (this.selectedFile()) this.selectedFile.set(updated);
        if (this.inspectedItem()) this.inspectedItem.set(updated);
        this.loadFiles(this.currentPrefix());
        this.copyText(updated.downloadToken, 'New Download Token Issued & Copied!');
      },
      error: (err) => this.notify.error('Failed to rotate token: ' + err.message)
    });
  }

  async deleteFile(f: StorageObject, permanent = false, event?: Event): Promise<void> {
    if (event) {
      event.stopPropagation();
    }
    const confirmed = await this.notify.confirm({
      title: permanent ? 'Permanently Purge File?' : 'Move to Trash?',
      message: permanent 
        ? `Permanently delete "${f.name}"? This action cannot be undone.` 
        : `Move "${f.name}" to the trash bin?`,
      confirmText: permanent ? 'Purge Permanently' : 'Move to Trash',
      type: 'danger'
    });
    if (!confirmed) return;

    this.goStore.deleteFile(f.bucket, f.path, permanent).subscribe({
      next: () => {
        if (this.selectedFile()?.path === f.path) this.closeDetailModal();
        if (this.inspectedItem()?.path === f.path) this.closeInspector();
        this.loadFiles(this.currentPrefix());
        this.loadStats();
        this.notify.success(permanent ? `"${f.name}" permanently purged.` : `"${f.name}" moved to trash.`);
      },
      error: (err) => this.notify.error('Failed to delete: ' + err.message)
    });
  }

  restoreSingleFile(f: StorageObject, event?: Event): void {
    if (event) {
      event.stopPropagation();
    }
    this.goStore.restoreFile(f.bucket, f.path).subscribe({
      next: () => {
        this.loadFiles(this.currentPrefix());
        this.loadStats();
        this.notify.success(`"${f.name}" restored successfully.`);
      },
      error: (err) => this.notify.error('Failed to restore: ' + err.message)
    });
  }

  // --- Folder Operations ---
  async deleteFolder(folderPrefix: string, event?: Event): Promise<void> {
    if (event) {
      event.stopPropagation();
    }
    const folderName = this.getFolderName(folderPrefix);
    const permanent = this.activeSubView() === 'trash';
    const confirmed = await this.notify.confirm({
      title: permanent ? 'Purge Folder?' : 'Delete Folder?',
      message: permanent
        ? `Permanently purge folder "${folderName}/" and all objects inside?`
        : `Move folder "${folderName}/" and all contents to Trash?`,
      confirmText: permanent ? 'Purge Folder' : 'Delete Folder',
      type: 'danger'
    });
    if (!confirmed) return;

    this.goStore.deleteFolder(this.currentBucket(), folderPrefix, permanent).subscribe({
      next: (res) => {
        this.notify.success(`Folder removed (${res.deleted} objects affected).`);
        this.loadFiles(this.currentPrefix());
        this.loadStats();
      },
      error: (err) => this.notify.error('Failed to delete folder: ' + err.message)
    });
  }

  downloadFolderZip(folderPrefix: string, event?: Event): void {
    if (event) {
      event.stopPropagation();
    }
    this.goStore.listFiles(this.currentBucket(), folderPrefix, '', 1000).subscribe({
      next: (res) => {
        const paths = (res.items || []).map(f => f.path);
        if (paths.length === 0) {
          this.notify.warning('Folder is empty.');
          return;
        }
        this.goStore.downloadZip(this.currentBucket(), paths).subscribe({
          next: (blob) => {
            const url = window.URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `${this.getFolderName(folderPrefix)}-folder.zip`;
            a.click();
            window.URL.revokeObjectURL(url);
            this.notify.success('Folder ZIP archive ready!');
          },
          error: (err) => this.notify.error('Failed to download ZIP: ' + err.message)
        });
      },
      error: (err) => this.notify.error('Failed to inspect folder contents: ' + err.message)
    });
  }

  // --- Rename Modal ---
  openRenameModal(f: StorageObject, event?: Event): void {
    if (event) event.stopPropagation();
    this.renameTarget.set(f);
    this.renameTargetFolder.set(null);
    this.renameInput.set(f.name);
    this.showRenameModal.set(true);
  }

  openRenameFolderModal(folderPrefix: string, event?: Event): void {
    if (event) event.stopPropagation();
    this.renameTarget.set(null);
    this.renameTargetFolder.set(folderPrefix);
    this.renameInput.set(this.getFolderName(folderPrefix));
    this.showRenameModal.set(true);
  }

  submitRename(): void {
    const newName = this.renameInput().trim();
    if (!newName) return;

    const file = this.renameTarget();
    if (file) {
      this.goStore.renameFile(file.bucket, file.path, newName).subscribe({
        next: () => {
          this.showRenameModal.set(false);
          this.loadFiles(this.currentPrefix());
          this.notify.success('File renamed successfully.');
        },
        error: (err) => this.notify.error('Rename failed: ' + (err.error?.error?.message || err.message))
      });
      return;
    }

    const folder = this.renameTargetFolder();
    if (folder) {
      // Rename folder by copying contents to new prefix
      const oldPrefix = folder;
      const parent = this.currentPrefix();
      const newPrefix = `${parent}${newName}/`;

      this.goStore.listFiles(this.currentBucket(), oldPrefix, '', 1000).subscribe({
        next: (res) => {
          const items = res.items || [];
          if (items.length === 0) {
            // Just create new keep
            this.newFolderName.set(newName);
            this.createFolder();
            this.showRenameModal.set(false);
            return;
          }
          let completed = 0;
          items.forEach(item => {
            const rel = item.path.substring(oldPrefix.length);
            const dst = `${newPrefix}${rel}`;
            this.goStore.moveFile(item.bucket, item.path, item.bucket, dst).subscribe({
              next: () => {
                completed++;
                if (completed === items.length) {
                  this.showRenameModal.set(false);
                  this.loadFiles(this.currentPrefix());
                  this.notify.success('Folder renamed successfully.');
                }
              }
            });
          });
        }
      });
    }
  }

  // --- Move / Copy Modal ---
  openMoveModal(f: StorageObject, event?: Event): void {
    if (event) event.stopPropagation();
    this.moveCopyTarget.set(f);
    this.moveCopyIsMove.set(true);
    this.moveCopyDstBucket.set(f.bucket);
    this.moveCopyDstPath.set(f.path);
    this.showMoveCopyModal.set(true);
  }

  openCopyModal(f: StorageObject, event?: Event): void {
    if (event) event.stopPropagation();
    this.moveCopyTarget.set(f);
    this.moveCopyIsMove.set(false);
    this.moveCopyDstBucket.set(f.bucket);
    this.moveCopyDstPath.set(`copy_of_${f.name}`);
    this.showMoveCopyModal.set(true);
  }

  submitMoveCopy(): void {
    const f = this.moveCopyTarget();
    if (!f) return;

    const dstBucket = this.moveCopyDstBucket().trim();
    const dstPath = this.moveCopyDstPath().trim();
    if (!dstBucket || !dstPath) return;

    const op = this.moveCopyIsMove()
      ? this.goStore.moveFile(f.bucket, f.path, dstBucket, dstPath)
      : this.goStore.copyFile(f.bucket, f.path, dstBucket, dstPath);

    op.subscribe({
      next: () => {
        this.showMoveCopyModal.set(false);
        this.loadFiles(this.currentPrefix());
        this.notify.success(this.moveCopyIsMove() ? 'File moved successfully.' : 'File copied successfully.');
      },
      error: (err) => this.notify.error('Operation failed: ' + (err.error?.error?.message || err.message))
    });
  }

  // --- Version History Modal ---
  openVersionHistoryModal(f: StorageObject, event?: Event): void {
    if (event) event.stopPropagation();
    this.targetVersionFile.set(f);
    this.goStore.listVersions(f.bucket, f.path).subscribe({
      next: (versions) => {
        this.fileVersions.set(versions);
        this.showVersionModal.set(true);
      },
      error: (err) => this.notify.error('Failed to fetch versions: ' + err.message)
    });
  }

  // --- Signed URL Modal ---
  openSignedUrlModal(f: StorageObject, event?: Event): void {
    if (event) event.stopPropagation();
    this.targetSignFile.set(f);
    this.generatedSignedUrl.set('');
    this.showSignedModal.set(true);
  }

  onDurationChange(event: Event): void {
    this.signDuration = parseInt((event.target as HTMLSelectElement).value, 10);
  }

  generateSignedUrl(): void {
    const f = this.targetSignFile();
    if (!f) return;

    this.goStore.generateSignedUrl(f.bucket, f.path, this.signDuration).subscribe({
      next: (res) => this.generatedSignedUrl.set(res.signedUrl),
      error: (err) => this.notify.error('Failed to generate signed URL: ' + err.message)
    });
  }

  // --- New Bucket Modal ---
  openNewBucketModal(): void {
    this.showNewBucketModal.set(true);
  }

  createNewBucket(name: string, desc: string): void {
    const trimmed = name.trim().toLowerCase();
    if (!trimmed) return;

    this.goStore.createBucket({ name: trimmed, description: desc, isPublic: true, projectId: this.projectId() }).subscribe({
      next: (created) => {
        this.showNewBucketModal.set(false);
        this.loadBuckets();
        this.selectBucket(created.name);
        this.notify.success(`Bucket "${created.name}" created!`);
      },
      error: (err) => this.notify.error('Failed to create bucket: ' + (err.error?.error?.message || err.message))
    });
  }

  // --- New Folder Modal ---
  openNewFolderModal(): void {
    this.newFolderName.set('');
    this.showNewFolderModal.set(true);
  }

  createFolder(): void {
    const name = this.newFolderName().trim().replace(/^\/+|\/+$/g, '');
    if (!name) return;
    const targetPrefix = this.currentPrefix() ? `${this.currentPrefix()}${name}/` : `${name}/`;
    const keepFilePath = `${targetPrefix}.keep`;
    
    const keepFile = new File([''], '.keep', { type: 'application/octet-stream' });
    this.showNewFolderModal.set(false);
    
    this.goStore.uploadWithProgress(keepFile, keepFilePath, this.currentBucket(), true).subscribe({
      next: (event) => {
        if (event.status === 'completed') {
          this.loadFiles(this.currentPrefix());
          this.loadStats();
          this.notify.success(`Folder "${name}" created!`);
        }
      },
      error: (err) => {
        console.error('Failed to create folder marker', err);
        this.loadFiles(this.currentPrefix());
      }
    });
  }

  // --- Edit Bucket Settings ---
  openEditBucketModal(): void {
    const current = this.buckets().find(b => b.name === this.currentBucket());
    this.editBucketData.set({
      name: this.currentBucket(),
      description: current?.description || '',
      isPublic: current?.isPublic ?? false,
      maxFileSize: current?.maxFileSize || 0,
      allowedMimes: (current?.allowedMimes || []).join(', ')
    });
    this.showEditBucketModal.set(true);
  }

  saveBucketSettings(): void {
    const data = this.editBucketData();
    const mimes = data.allowedMimes
      .split(',')
      .map(m => m.trim())
      .filter(m => m.length > 0);

    this.goStore.updateBucket(data.name, {
      description: data.description,
      isPublic: data.isPublic,
      maxFileSize: +data.maxFileSize,
      allowedMimes: mimes
    }).subscribe({
      next: () => {
        this.showEditBucketModal.set(false);
        this.loadBuckets();
        this.notify.success('Bucket settings updated!');
      },
      error: (err) => this.notify.error('Failed to update bucket: ' + (err.error?.error?.message || err.message))
    });
  }

  // --- Public Share Modal ---
  openShareModal(f: StorageObject, event?: Event): void {
    if (event) event.stopPropagation();
    this.shareTargetFile.set(f);
    this.sharePassword.set('');
    this.shareMaxDownloads.set(0);
    this.shareExpiresDays.set(7);
    this.generatedShare.set(null);
    this.showShareModal.set(true);
  }

  submitCreateShare(): void {
    const f = this.shareTargetFile();
    if (!f) return;

    let expiresAt: string | undefined;
    if (this.shareExpiresDays() > 0) {
      const d = new Date();
      d.setDate(d.getDate() + this.shareExpiresDays());
      expiresAt = d.toISOString();
    }

    this.goStore.createShareLink({
      bucket: f.bucket,
      path: f.path,
      password: this.sharePassword() || undefined,
      maxDownloads: this.shareMaxDownloads() > 0 ? this.shareMaxDownloads() : undefined,
      expiresAt: expiresAt
    }).subscribe({
      next: (sh) => {
        this.generatedShare.set(sh);
        this.notify.success('Public share link active!');
      },
      error: (err) => this.notify.error('Failed to create share link: ' + (err.error?.error?.message || err.message))
    });
  }

  // In-Browser Multi-Format Preview Modal
  showPreviewModal = signal<boolean>(false);
  previewFile = signal<StorageObject | null>(null);
  previewSafeUrl = signal<SafeResourceUrl | null>(null);
  previewTextContent = signal<string>('');
  previewLoading = signal<boolean>(false);
  previewZoom = signal<number>(100);
  previewImageLoading = signal<boolean>(true);
  previewImageError = signal<boolean>(false);

  openPreviewModal(f: StorageObject, event?: Event): void {
    if (event) event.stopPropagation();
    this.previewFile.set(f);
    this.previewTextContent.set('');
    this.previewLoading.set(true);
    this.previewImageLoading.set(true);
    this.previewImageError.set(false);
    this.previewZoom.set(100);

    const directUrl = this.getDirectDownloadUrl(f);
    this.previewSafeUrl.set(this.sanitizer.bypassSecurityTrustResourceUrl(directUrl));
    this.showPreviewModal.set(true);

    if (this.isTextOrCode(f.contentType) || f.name.endsWith('.md') || f.name.endsWith('.txt') || f.name.endsWith('.json') || f.name.endsWith('.go') || f.name.endsWith('.ts') || f.name.endsWith('.yaml') || f.name.endsWith('.sh') || f.name.endsWith('.sql')) {
      fetch(directUrl)
        .then(res => res.text())
        .then(text => {
          this.previewTextContent.set(text);
          this.previewLoading.set(false);
        })
        .catch(() => {
          this.previewLoading.set(false);
        });
    } else {
      this.previewLoading.set(false);
    }
  }

  closePreviewModal(): void {
    this.showPreviewModal.set(false);
    this.previewFile.set(null);
    this.previewSafeUrl.set(null);
    this.previewTextContent.set('');
    this.previewImageLoading.set(false);
    this.previewImageError.set(false);
  }

  zoomIn(): void {
    this.previewZoom.update(z => Math.min(z + 25, 400));
  }

  zoomOut(): void {
    this.previewZoom.update(z => Math.max(z - 25, 25));
  }

  resetZoom(): void {
    this.previewZoom.set(100);
  }

  onPreviewImageLoaded(): void {
    this.previewImageLoading.set(false);
    this.previewImageError.set(false);
  }

  onPreviewImageFailed(): void {
    this.previewImageLoading.set(false);
    this.previewImageError.set(true);
  }

  isTextOrCode(contentType: string): boolean {
    return contentType.startsWith('text/') ||
      contentType.includes('json') ||
      contentType.includes('javascript') ||
      contentType.includes('typescript') ||
      contentType.includes('xml') ||
      contentType.includes('yaml') ||
      contentType.includes('x-sh');
  }

  // --- Lifecycle & Retention ---
  openLifecycleModal(): void {
    this.goStore.getLifecycleRule(this.currentBucket()).subscribe({
      next: (rule) => {
        this.lifecycleRule.set(rule);
        this.showLifecycleModal.set(true);
      },
      error: () => {
        this.lifecycleRule.set({
          bucket: this.currentBucket(),
          trashDays: 30,
          versionLimit: 5,
          expirationDays: 0,
          enabled: true
        });
        this.showLifecycleModal.set(true);
      }
    });
  }

  saveLifecycleRule(): void {
    const rule = this.lifecycleRule();
    this.goStore.updateLifecycleRule(this.currentBucket(), rule).subscribe({
      next: () => {
        this.showLifecycleModal.set(false);
        this.notify.success('Lifecycle retention rules updated!');
      },
      error: (err) => this.notify.error('Failed to save lifecycle: ' + err.message)
    });
  }

  triggerLifecycleSweep(): void {
    this.goStore.triggerLifecycleSweep().subscribe({
      next: (res) => this.notify.info(res.message, 'Retention Sweep'),
      error: (err) => this.notify.error('Sweep failed: ' + err.message)
    });
  }

  triggerScrubber(): void {
    this.goStore.triggerScrubberReport().subscribe({
      next: (rep) => this.notify.success(`Verified ${rep.totalBlobsChecked} CAS blocks (${this.formatBytes(rep.totalBytesScrubbed)}). Corrupted: ${rep.corruptedBlobs}`, 'Scrubber Complete'),
      error: (err) => this.notify.error('Scrubber failed: ' + err.message)
    });
  }

  // --- URL and Format Helpers ---
  getDirectDownloadUrl(file: StorageObject): string {
    if (!file || !file.downloadUrl) return '';
    if (file.downloadUrl.startsWith('http://') || file.downloadUrl.startsWith('https://')) {
      return file.downloadUrl;
    }
    const origin = this.goStore.getBaseUrl() || (typeof window !== 'undefined' ? window.location.origin : '');
    return `${origin}${file.downloadUrl.startsWith('/') ? '' : '/'}${file.downloadUrl}`;
  }

  getGatewayUrl(file: StorageObject): string {
    if (!file) return '';
    const base = this.goStore.getBaseUrl() || (typeof window !== 'undefined' ? window.location.origin : '');
    return `${base}/v0/b/${file.bucket}/o/${encodeURIComponent(file.path)}?alt=media`;
  }

  getS3Uri(file: StorageObject): string {
    if (!file) return '';
    return `s3://${file.bucket}/${file.path}`;
  }

  copyText(text: string, msg: string): void {
    navigator.clipboard.writeText(text);
    this.notify.success(msg || 'Copied to clipboard!');
  }

  formatBytes(bytes: number): string {
    if (!+bytes) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
  }
}
