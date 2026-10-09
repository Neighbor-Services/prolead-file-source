import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { GoStoreService } from '../../services/gostore.service';
import { NotificationService } from '../../services/notification.service';
import { StaffUser } from '../../services/gostore.models';

@Component({
  selector: 'app-staff',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './staff.component.html',
  styleUrl: './staff.component.css'
})
export class StaffComponent implements OnInit {
  private goStore = inject(GoStoreService);
  private notify = inject(NotificationService);

  staffList = signal<StaffUser[]>([]);
  isLoading = signal<boolean>(false);
  searchQuery = signal<string>('');
  roleFilter = signal<string>('ALL');

  // Modals
  showAddModal = signal<boolean>(false);
  showEditModal = signal<boolean>(false);
  showResetModal = signal<boolean>(false);

  // Form State
  newUsername = signal<string>('');
  newEmail = signal<string>('');
  newPassword = signal<string>('');
  newRole = signal<'superadmin' | 'admin' | 'operator' | 'viewer'>('admin');

  editingStaff = signal<StaffUser | null>(null);
  editRole = signal<string>('admin');
  editStatus = signal<string>('active');

  targetResetStaff = signal<StaffUser | null>(null);
  resetPasswordInput = signal<string>('');

  ngOnInit(): void {
    this.loadStaff();
  }

  loadStaff(): void {
    this.isLoading.set(true);
    this.goStore.listStaff().subscribe({
      next: (users) => {
        this.staffList.set(users || []);
        this.isLoading.set(false);
      },
      error: (err) => {
        this.isLoading.set(false);
        this.notify.error('Failed to load staff list: ' + (err.error?.message || err.message));
      }
    });
  }

  filteredStaff(): StaffUser[] {
    let list = this.staffList();
    const role = this.roleFilter();
    const q = this.searchQuery().trim().toLowerCase();

    if (role !== 'ALL') {
      list = list.filter(u => u.role === role || (role === 'superadmin' && u.isSuperuser));
    }

    if (q) {
      list = list.filter(u => 
        u.username.toLowerCase().includes(q) ||
        (u.email || '').toLowerCase().includes(q) ||
        u.role.toLowerCase().includes(q)
      );
    }

    return list;
  }

  openAddModal(): void {
    this.newUsername.set('');
    this.newEmail.set('');
    this.newPassword.set('');
    this.newRole.set('admin');
    this.showAddModal.set(true);
  }

  submitAddStaff(): void {
    const username = this.newUsername().trim();
    const email = this.newEmail().trim();
    const password = this.newPassword().trim();
    const role = this.newRole();

    if (!username || !password) {
      this.notify.error('Username and initial password are required');
      return;
    }

    if (password.length < 6) {
      this.notify.error('Password must be at least 6 characters long');
      return;
    }

    this.goStore.createStaff({ username, email, password, role }).subscribe({
      next: (user) => {
        this.showAddModal.set(false);
        this.notify.success(`Staff member "${user.username}" created successfully.`);
        this.loadStaff();
      },
      error: (err) => {
        this.notify.error('Failed to create staff member: ' + (err.error?.message || err.message));
      }
    });
  }

  openEditModal(staff: StaffUser): void {
    this.editingStaff.set(staff);
    this.editRole.set(staff.role || 'admin');
    this.editStatus.set(staff.status || 'active');
    this.showEditModal.set(true);
  }

  submitEditStaff(): void {
    const staff = this.editingStaff();
    if (!staff) return;

    this.goStore.updateStaff(staff.id, {
      role: this.editRole(),
      status: this.editStatus()
    }).subscribe({
      next: () => {
        this.showEditModal.set(false);
        this.notify.success(`Staff account "${staff.username}" updated.`);
        this.loadStaff();
      },
      error: (err) => {
        this.notify.error('Failed to update staff account: ' + (err.error?.message || err.message));
      }
    });
  }

  openResetModal(staff: StaffUser): void {
    this.targetResetStaff.set(staff);
    this.resetPasswordInput.set('');
    this.showResetModal.set(true);
  }

  submitResetPassword(): void {
    const staff = this.targetResetStaff();
    const pass = this.resetPasswordInput().trim();
    if (!staff || !pass) {
      this.notify.error('Please enter a new password');
      return;
    }
    if (pass.length < 6) {
      this.notify.error('Password must be at least 6 characters long');
      return;
    }

    this.goStore.resetStaffPassword(staff.id, pass).subscribe({
      next: () => {
        this.showResetModal.set(false);
        this.notify.success(`Password for "${staff.username}" reset successfully.`);
      },
      error: (err) => {
        this.notify.error('Failed to reset password: ' + (err.error?.message || err.message));
      }
    });
  }

  async deleteStaff(staff: StaffUser): Promise<void> {
    const confirmed = await this.notify.confirm({
      title: 'Revoke Staff Access?',
      message: `Are you sure you want to permanently delete staff member "${staff.username}"? They will immediately lose dashboard access.`,
      confirmText: 'Revoke & Delete',
      type: 'danger'
    });

    if (!confirmed) return;

    this.goStore.deleteStaff(staff.id).subscribe({
      next: () => {
        this.notify.success(`Staff account "${staff.username}" removed.`);
        this.loadStaff();
      },
      error: (err) => {
        this.notify.error('Failed to delete staff: ' + (err.error?.message || err.message));
      }
    });
  }
}
