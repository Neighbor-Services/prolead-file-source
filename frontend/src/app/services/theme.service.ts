import { Injectable, signal, effect } from '@angular/core';

export type ThemeMode = 'dark' | 'light';

@Injectable({
  providedIn: 'root'
})
export class ThemeService {
  private readonly THEME_KEY = 'gostore_theme_pref';
  theme = signal<ThemeMode>(this.getInitialTheme());

  constructor() {
    this.applyTheme(this.theme());
    effect(() => {
      const current = this.theme();
      this.applyTheme(current);
      localStorage.setItem(this.THEME_KEY, current);
    });
  }

  private getInitialTheme(): ThemeMode {
    const saved = localStorage.getItem(this.THEME_KEY) as ThemeMode | null;
    if (saved === 'light' || saved === 'dark') {
      return saved;
    }
    return 'dark';
  }

  toggleTheme(): void {
    this.theme.update(prev => prev === 'dark' ? 'light' : 'dark');
  }

  setTheme(mode: ThemeMode): void {
    this.theme.set(mode);
  }

  private applyTheme(mode: ThemeMode): void {
    document.documentElement.setAttribute('data-theme', mode);
  }
}
