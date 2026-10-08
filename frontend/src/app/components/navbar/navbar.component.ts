import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterModule } from '@angular/router';
import { GoStoreService } from '../../services/gostore.service';
import { ThemeService } from '../../services/theme.service';
import { Project } from '../../services/gostore.models';

@Component({
  selector: 'app-navbar',
  standalone: true,
  imports: [CommonModule, RouterModule],
  templateUrl: './navbar.component.html',
  styleUrl: './navbar.component.css'
})
export class NavbarComponent implements OnInit {
  private goStore = inject(GoStoreService);
  themeService = inject(ThemeService);
  private router = inject(Router);

  currentUser = signal(this.goStore.currentUser());
  currentProject = signal(this.goStore.currentProject());
  projects = signal<Project[]>([]);
  showUserMenu = signal<boolean>(false);

  ngOnInit(): void {
    this.goStore.listProjects().subscribe({
      next: (list) => this.projects.set(list || []),
      error: () => {}
    });
  }

  toggleTheme(): void {
    this.themeService.toggleTheme();
  }

  logout(): void {
    this.goStore.logout();
    this.router.navigate(['/login']);
  }
}
