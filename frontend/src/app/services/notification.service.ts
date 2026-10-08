import { Injectable, signal } from '@angular/core';

export interface ToastItem {
  id: string;
  type: 'success' | 'error' | 'info' | 'warning';
  title?: string;
  message: string;
  duration: number;
  createdAt: number;
}

export interface DialogConfig {
  id: string;
  type: 'danger' | 'warning' | 'info' | 'success' | 'primary' | 'error';
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  isAlertOnly?: boolean;
  resolve: (result: boolean) => void;
}

@Injectable({
  providedIn: 'root'
})
export class NotificationService {
  toasts = signal<ToastItem[]>([]);
  activeDialog = signal<DialogConfig | null>(null);

  // --- Toast Methods ---

  toast(message: string, type: 'success' | 'error' | 'info' | 'warning' = 'info', title?: string, duration = 3500): string {
    const id = 'toast-' + Math.random().toString(36).substring(2, 9);
    const item: ToastItem = {
      id,
      type,
      title,
      message,
      duration,
      createdAt: Date.now()
    };

    this.toasts.update(list => [...list, item]);

    if (duration > 0) {
      setTimeout(() => {
        this.removeToast(id);
      }, duration);
    }

    return id;
  }

  success(message: string, title?: string, duration = 3500): string {
    return this.toast(message, 'success', title || 'Success', duration);
  }

  error(message: string, title?: string, duration = 4500): string {
    return this.toast(message, 'error', title || 'Error', duration);
  }

  info(message: string, title?: string, duration = 3500): string {
    return this.toast(message, 'info', title, duration);
  }

  warning(message: string, title?: string, duration = 4000): string {
    return this.toast(message, 'warning', title || 'Warning', duration);
  }

  removeToast(id: string): void {
    this.toasts.update(list => list.filter(t => t.id !== id));
  }

  // --- HTML Dialog (Modal) Methods ---

  confirm(options: {
    title: string;
    message: string;
    confirmText?: string;
    cancelText?: string;
    type?: 'danger' | 'warning' | 'info' | 'primary';
  }): Promise<boolean> {
    return new Promise<boolean>((resolve) => {
      this.activeDialog.set({
        id: 'dialog-' + Math.random().toString(36).substring(2, 9),
        title: options.title,
        message: options.message,
        confirmText: options.confirmText || 'Confirm',
        cancelText: options.cancelText || 'Cancel',
        type: options.type || 'primary',
        isAlertOnly: false,
        resolve
      });
    });
  }

  alert(options: {
    title: string;
    message: string;
    buttonText?: string;
    type?: 'success' | 'error' | 'info' | 'warning';
  }): Promise<void> {
    return new Promise<void>((resolve) => {
      this.activeDialog.set({
        id: 'dialog-' + Math.random().toString(36).substring(2, 9),
        title: options.title,
        message: options.message,
        confirmText: options.buttonText || 'OK',
        type: options.type || 'info',
        isAlertOnly: true,
        resolve: () => resolve()
      });
    });
  }

  closeDialog(result = false): void {
    const d = this.activeDialog();
    if (d) {
      this.activeDialog.set(null);
      d.resolve(result);
    }
  }
}
