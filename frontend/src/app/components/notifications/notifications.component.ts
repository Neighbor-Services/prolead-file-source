import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { NotificationService } from '../../services/notification.service';

@Component({
  selector: 'app-notifications',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './notifications.component.html',
  styleUrl: './notifications.component.css'
})
export class NotificationsComponent {
  notifyService = inject(NotificationService);

  toasts = this.notifyService.toasts;
  activeDialog = this.notifyService.activeDialog;

  dismissToast(id: string): void {
    this.notifyService.removeToast(id);
  }

  onConfirm(): void {
    this.notifyService.closeDialog(true);
  }

  onCancel(): void {
    this.notifyService.closeDialog(false);
  }
}
