import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { GoStoreService } from '../../services/gostore.service';
import { NotificationService } from '../../services/notification.service';
import { ThemeService } from '../../services/theme.service';
import { Project, StorageStats, APIKey } from '../../services/gostore.models';

@Component({
  selector: 'app-projects',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './projects.component.html',
  styleUrl: './projects.component.css'
})
export class ProjectsComponent implements OnInit {
  private goStore = inject(GoStoreService);
  private notify = inject(NotificationService);
  private router = inject(Router);
  themeService = inject(ThemeService);

  theme = this.themeService.theme;
  projects = signal<Project[]>([]);
  stats = signal<StorageStats | null>(null);
  currentUser = signal(this.goStore.currentUser());
  searchQuery = signal<string>('');
  viewMode = signal<'grid' | 'list'>('grid');

  // Modal States
  showNewModal = signal<boolean>(false);
  showSuccessModal = signal<boolean>(false);
  isSubmitting = signal<boolean>(false);

  // Form Fields
  newProjName = signal<string>('');
  newProjDesc = signal<string>('');
  autoGenerateKey = signal<boolean>(true);
  keyRole = signal<string>('read-write');
  storageQuota = signal<string>('0'); // 0 = unlimited

  // Success Provisioned Data
  createdProject = signal<Project | null>(null);
  createdApiKey = signal<APIKey | null>(null);
  copyFeedback = signal<string>('');

  get filteredProjects(): Project[] {
    const q = this.searchQuery().toLowerCase().trim();
    if (!q) return this.projects();
    return this.projects().filter(p => 
      p.name.toLowerCase().includes(q) || 
      p.id.toLowerCase().includes(q) || 
      (p.slug && p.slug.toLowerCase().includes(q)) ||
      (p.description && p.description.toLowerCase().includes(q))
    );
  }

  ngOnInit(): void {
    this.loadProjects();
    this.loadStats();
  }

  loadProjects(): void {
    this.goStore.listProjects().subscribe({
      next: (list) => this.projects.set(list || []),
      error: (err) => console.error('Failed to load projects', err)
    });
  }

  loadStats(): void {
    this.goStore.getStats().subscribe({
      next: (s) => this.stats.set(s),
      error: (err) => console.error('Failed to load stats', err)
    });
  }

  openProject(p: Project): void {
    this.goStore.selectProject(p);
    this.router.navigate(['/projects', p.id, 'storage']);
  }

  openProjectKeys(p: Project, event: Event): void {
    event.stopPropagation();
    this.goStore.selectProject(p);
    this.router.navigate(['/projects', p.id, 'keys']);
  }

  openProjectWebhooks(p: Project, event: Event): void {
    event.stopPropagation();
    this.goStore.selectProject(p);
    this.router.navigate(['/projects', p.id, 'webhooks']);
  }

  createProject(): void {
    const name = this.newProjName().trim();
    if (!name) return;

    this.isSubmitting.set(true);
    this.goStore.createProject(name, this.newProjDesc().trim()).subscribe({
      next: (created) => {
        this.createdProject.set(created);
        this.goStore.selectProject(created);

        if (this.autoGenerateKey()) {
          this.goStore.createAPIKey(`${name} Initial Client Key`, this.keyRole()).subscribe({
            next: (key) => {
              this.createdApiKey.set(key);
              this.finishCreation();
            },
            error: () => {
              this.finishCreation();
            }
          });
        } else {
          this.finishCreation();
        }
      },
      error: (err) => {
        this.isSubmitting.set(false);
        this.notify.error('Failed to create project: ' + err.message);
      }
    });
  }

  private finishCreation(): void {
    this.isSubmitting.set(false);
    this.showNewModal.set(false);
    this.showSuccessModal.set(true);
    this.newProjName.set('');
    this.newProjDesc.set('');
    this.loadProjects();
    this.loadStats();
    this.notify.success('Project created successfully!');
  }

  copyText(text: string, label: string): void {
    navigator.clipboard.writeText(text);
    this.copyFeedback.set(label);
    this.notify.success('API Key Copied!');
    setTimeout(() => this.copyFeedback.set(''), 2500);
  }

  async deleteProject(p: Project, event: Event): Promise<void> {
    event.stopPropagation();
    if (p.id === 'p-default' || p.slug === 'default') {
      this.notify.warning('The Default Project cannot be deleted.');
      return;
    }

    const confirmed = await this.notify.confirm({
      title: 'Delete Project?',
      message: `Delete project "${p.name}" and all its scoped resources?`,
      confirmText: 'Delete Project',
      type: 'danger'
    });
    if (!confirmed) return;

    this.goStore.deleteProject(p.id).subscribe({
      next: () => {
        this.loadProjects();
        this.notify.success(`Project "${p.name}" deleted.`);
      },
      error: (err) => this.notify.error('Failed to delete project: ' + err.message)
    });
  }

  formatBytes(bytes: number): string {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  toggleTheme(): void {
    this.themeService.toggleTheme();
  }

  logout(): void {
    this.goStore.logout();
    this.router.navigate(['/login']);
  }
}
