import { Component, inject, signal, OnInit, computed } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterModule } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { GoStoreService } from '../../services/gostore.service';
import { ThemeService } from '../../services/theme.service';
import { Project, TwoFactorSetupResult } from '../../services/gostore.models';

@Component({
  selector: 'app-navbar',
  standalone: true,
  imports: [CommonModule, RouterModule, FormsModule],
  templateUrl: './navbar.component.html',
  styleUrl: './navbar.component.css'
})
export class NavbarComponent implements OnInit {
  goStore = inject(GoStoreService);
  themeService = inject(ThemeService);
  private router = inject(Router);

  currentUser = computed(() => this.goStore.currentUser());
  currentProject = signal(this.goStore.currentProject());
  projects = signal<Project[]>([]);
  showUserMenu = signal<boolean>(false);

  // 2FA Security Modal
  show2FAModal = signal<boolean>(false);
  twoFactorData = signal<TwoFactorSetupResult | null>(null);
  twoFactorCode = signal<string>('');
  twoFactorLoading = signal<boolean>(false);
  twoFactorError = signal<string | null>(null);
  twoFactorSuccess = signal<string | null>(null);
  isDisabling2FA = signal<boolean>(false);
  copiedSecret = signal<boolean>(false);

  ngOnInit(): void {
    this.goStore.listProjects().subscribe({
      next: (list) => this.projects.set(list || []),
      error: () => {}
    });
  }

  toggleTheme(): void {
    this.themeService.toggleTheme();
  }

  open2FAModal(): void {
    this.showUserMenu.set(false);
    this.twoFactorError.set(null);
    this.twoFactorSuccess.set(null);
    this.twoFactorCode.set('');
    this.copiedSecret.set(false);
    this.isDisabling2FA.set(false);

    const user = this.currentUser();
    if (user?.twoFactorEnabled) {
      this.isDisabling2FA.set(true);
      this.show2FAModal.set(true);
    } else {
      this.twoFactorLoading.set(true);
      this.goStore.setup2FA().subscribe({
        next: (data) => {
          this.twoFactorData.set(data);
          this.twoFactorLoading.set(false);
          this.show2FAModal.set(true);
        },
        error: (err) => {
          this.twoFactorLoading.set(false);
          this.twoFactorError.set(err.error?.message || 'Failed to initialize 2FA setup');
          this.show2FAModal.set(true);
        }
      });
    }
  }

  close2FAModal(): void {
    this.show2FAModal.set(false);
    this.twoFactorData.set(null);
    this.twoFactorCode.set('');
    this.twoFactorError.set(null);
    this.twoFactorSuccess.set(null);
  }

  copySecret(secret: string): void {
    navigator.clipboard.writeText(secret);
    this.copiedSecret.set(true);
    setTimeout(() => this.copiedSecret.set(false), 2000);
  }

  getQrCodeUrl(otpAuthUri: string): string {
    return `https://api.qrserver.com/v1/create-qr-code/?size=200x200&margin=10&data=${encodeURIComponent(otpAuthUri)}`;
  }

  confirmEnable2FA(): void {
    const code = this.twoFactorCode().trim();
    const data = this.twoFactorData();
    if (!code || code.length !== 6) {
      this.twoFactorError.set('Please enter a valid 6-digit authentication code');
      return;
    }
    if (!data?.secret) {
      this.twoFactorError.set('Invalid secret session. Please retry.');
      return;
    }

    this.twoFactorLoading.set(true);
    this.twoFactorError.set(null);

    this.goStore.enable2FA(code, data.secret).subscribe({
      next: (res) => {
        this.twoFactorLoading.set(false);
        this.twoFactorSuccess.set('Two-Factor Authentication is now active on your account!');
        
        // Update user state
        const current = this.goStore.currentUser();
        if (current) {
          const updated = { ...current, twoFactorEnabled: true };
          this.goStore.currentUser.set(updated);
          localStorage.setItem('gostore_user', JSON.stringify(updated));
        }

        setTimeout(() => {
          this.close2FAModal();
        }, 2000);
      },
      error: (err) => {
        this.twoFactorLoading.set(false);
        this.twoFactorError.set(err.error?.message || 'Invalid authenticator code. Please try again.');
      }
    });
  }

  confirmDisable2FA(): void {
    const code = this.twoFactorCode().trim();
    if (!code || code.length !== 6) {
      this.twoFactorError.set('Please enter your current 6-digit authenticator code to disable 2FA');
      return;
    }

    this.twoFactorLoading.set(true);
    this.twoFactorError.set(null);

    this.goStore.disable2FA(code).subscribe({
      next: (res) => {
        this.twoFactorLoading.set(false);
        this.twoFactorSuccess.set('Two-Factor Authentication has been disabled.');
        
        // Update user state
        const current = this.goStore.currentUser();
        if (current) {
          const updated = { ...current, twoFactorEnabled: false };
          this.goStore.currentUser.set(updated);
          localStorage.setItem('gostore_user', JSON.stringify(updated));
        }

        setTimeout(() => {
          this.close2FAModal();
        }, 1500);
      },
      error: (err) => {
        this.twoFactorLoading.set(false);
        this.twoFactorError.set(err.error?.message || 'Invalid authenticator code. Please try again.');
      }
    });
  }

  logout(): void {
    this.goStore.logout();
    this.router.navigate(['/login']);
  }
}

