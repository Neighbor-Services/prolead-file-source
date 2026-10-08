import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { GoStoreService } from '../services/gostore.service';

export const authGuard: CanActivateFn = (route, state) => {
  const goStore = inject(GoStoreService);
  const router = inject(Router);

  if (goStore.currentUser() || localStorage.getItem('gostore_user') || localStorage.getItem('gostore_api_key')) {
    return true;
  }

  // Not authenticated, redirect to login page
  router.navigate(['/login']);
  return false;
};
