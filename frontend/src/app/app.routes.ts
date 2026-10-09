import { Routes } from '@angular/router';
import { LoginComponent } from './pages/login/login.component';
import { ProjectsComponent } from './pages/projects/projects.component';
import { LayoutComponent } from './components/layout/layout.component';
import { StorageComponent } from './pages/storage/storage.component';
import { KeysComponent } from './pages/keys/keys.component';
import { WebhooksComponent } from './pages/webhooks/webhooks.component';
import { OperationsComponent } from './pages/operations/operations.component';
import { StaffComponent } from './pages/staff/staff.component';
import { authGuard } from './guards/auth.guard';

export const routes: Routes = [
  { path: 'login', component: LoginComponent },
  { path: 'projects', component: ProjectsComponent, canActivate: [authGuard] },
  {
    path: '',
    component: LayoutComponent,
    canActivate: [authGuard],
    children: [
      { path: '', redirectTo: 'projects', pathMatch: 'full' },
      { path: 'projects/:projectId/storage', component: StorageComponent },
      { path: 'projects/:projectId/keys', component: KeysComponent },
      { path: 'projects/:projectId/webhooks', component: WebhooksComponent },
      { path: 'operations', component: OperationsComponent },
      { path: 'staff', component: StaffComponent },
    ]
  },
  { path: '**', redirectTo: 'projects' }
];
