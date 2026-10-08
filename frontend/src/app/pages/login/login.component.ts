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
