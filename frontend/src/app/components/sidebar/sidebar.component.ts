import { Component, inject, signal, OnInit, OnDestroy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterModule } from '@angular/router';
import { Subscription } from 'rxjs';
import { GoStoreService } from '../../services/gostore.service';
import { Project, StorageEvent } from '../../services/gostore.models';

@Component({
  selector: 'app-sidebar',
  standalone: true,
  imports: [CommonModule, RouterModule],
  templateUrl: './sidebar.component.html',
  styleUrl: './sidebar.component.css'
})
export class SidebarComponent implements OnInit, OnDestroy {
  goStore = inject(GoStoreService);
  private router = inject(Router);
  private sseSub?: Subscription;

  currentUser = signal(this.goStore.currentUser());
  currentProject = signal(this.goStore.currentProject());
  projects = signal<Project[]>([]);
  latestEvents = signal<StorageEvent[]>([]);
  showProjDropdown = signal<boolean>(false);

  closeSidebar(): void {
    this.goStore.closeMobileSidebar();
  }

  ngOnInit(): void {
    this.loadProjects();
    this.initSSE();
  }

  ngOnDestroy(): void {
    this.sseSub?.unsubscribe();
  }

  private initSSE(): void {
    this.sseSub = this.goStore.listenToEvents().subscribe({
      next: (event) => {
        this.latestEvents.update(prev => [event, ...prev.slice(0, 9)]);
      },
      error: (err) => console.warn('SSE warning:', err)
    });
  }

  loadProjects(): void {
    this.goStore.listProjects().subscribe({
      next: (list) => {
        this.projects.set(list || []);
      },
      error: (err) => console.error('Failed to load projects', err)
    });
  }

  selectProject(p: Project): void {
    this.goStore.selectProject(p);
    this.currentProject.set(p);
    this.showProjDropdown.set(false);
    this.router.navigate(['/projects', p.id, 'storage']);
  }

  logout(): void {
    this.goStore.logout();
    this.router.navigate(['/login']);
  }
}
