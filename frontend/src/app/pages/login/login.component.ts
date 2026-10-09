import { Component, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { GoStoreService } from '../../services/gostore.service';

@Component({
  selector: 'app-login',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './login.component.html',
  styleUrl: './login.component.css'
})
export class LoginComponent {
  private goStore = inject(GoStoreService);
  private router = inject(Router);

  username = signal<string>('');
  password = signal<string>('');
  totpCode = signal<string>('');
  tempToken = signal<string>('');
  require2FA = signal<boolean>(false);
  errorMessage = signal<string>('');
  isLoading = signal<boolean>(false);
  showPassword = signal<boolean>(false);

  togglePassword(): void {
    this.showPassword.update(v => !v);
  }

  onLogin(): void {
    const userVal = this.username().trim();
    const passVal = this.password().trim();

    if (!userVal || !passVal) {
      this.errorMessage.set('Please enter both username/email and password');
      return;
    }

    this.isLoading.set(true);
    this.errorMessage.set('');

    this.goStore.login(userVal, passVal).subscribe({
      next: (res) => {
        this.isLoading.set(false);
        if (res.require2fa && res.tempToken) {
          this.tempToken.set(res.tempToken);
          this.require2FA.set(true);
          this.totpCode.set('');
        } else {
          this.router.navigate(['/projects']);
        }
      },
      error: (err) => {
        this.isLoading.set(false);
        this.errorMessage.set(this.extractErrorMessage(err));
      }
    });
  }

  onVerify2FA(): void {
    const code = this.totpCode().trim();
    if (!code || code.length !== 6) {
      this.errorMessage.set('Please enter the 6-digit authentication code');
      return;
    }

    this.isLoading.set(true);
    this.errorMessage.set('');

    this.goStore.verify2FA(this.tempToken(), code).subscribe({
      next: () => {
        this.isLoading.set(false);
        this.router.navigate(['/projects']);
      },
      error: (err) => {
        this.isLoading.set(false);
        this.errorMessage.set(this.extractErrorMessage(err));
      }
    });
  }

  cancel2FA(): void {
    this.require2FA.set(false);
    this.tempToken.set('');
    this.totpCode.set('');
    this.errorMessage.set('');
  }

  private extractErrorMessage(err: any): string {
    if (!err) return 'Invalid credentials or connection error';
    if (typeof err === 'string') return err;
    if (typeof err.error === 'string') return err.error;
    if (err.error?.error?.message && typeof err.error.error.message === 'string') {
      return err.error.error.message;
    }
    if (typeof err.error?.error === 'string') return err.error.error;
    if (typeof err.error?.message === 'string') return err.error.message;
    if (typeof err.message === 'string') return err.message;
    return 'Invalid credentials or connection error';
  }
}
