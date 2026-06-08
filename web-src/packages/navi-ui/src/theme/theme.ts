// Generic NAVI theme engine: the design-token data model plus the function that
// projects a ThemeModel onto CSS custom properties (`--navi-*`). This is the
// app-agnostic core of the design system. App-specific concerns (persistence,
// presets, server schema versioning) live in the consuming app.

export type ThemeMode = 'light' | 'dark' | 'system';
export type EffectiveMode = 'light' | 'dark';
export type Density = 'compact' | 'comfortable';
export type FontScale = 'small' | 'default' | 'large';

export type ThemeTokens = {
  background: string;
  surface: string;
  text: string;
  muted: string;
  border: string;
  danger: string;
  warning: string;
  success: string;
};

export type ThemeTokenGroups = {
  light: ThemeTokens;
  dark: ThemeTokens;
};

export type ThemeModel = {
  mode: ThemeMode;
  accent: string;
  density: Density;
  font_scale: FontScale;
  tokens: ThemeTokenGroups;
};

export const DEFAULT_ACCENT = '#3b82f6';

export const BASE_TOKENS: ThemeTokenGroups = {
  light: {
    background: '#f8fafc',
    surface: '#ffffff',
    text: '#0f172a',
    muted: '#64748b',
    border: '#dbe3ef',
    danger: '#dc2626',
    warning: '#d97706',
    success: '#059669',
  },
  dark: {
    background: '#0b1020',
    surface: '#111827',
    text: '#e5e7eb',
    muted: '#9ca3af',
    border: '#1f2937',
    danger: '#f87171',
    warning: '#fbbf24',
    success: '#34d399',
  },
};

export const DEFAULT_THEME: ThemeModel = {
  mode: 'system',
  accent: DEFAULT_ACCENT,
  density: 'compact',
  font_scale: 'default',
  tokens: {
    light: { ...BASE_TOKENS.light },
    dark: { ...BASE_TOKENS.dark },
  },
};

export function systemMode(): EffectiveMode {
  if (typeof window === 'undefined') return 'dark';
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
}

export function effectiveMode(mode: ThemeMode): EffectiveMode {
  return mode === 'system' ? systemMode() : mode;
}

export function activeTokens(theme: ThemeModel): ThemeTokens {
  return theme.tokens[effectiveMode(theme.mode)];
}

export function createDefaultTheme(mode: ThemeMode = 'system', accent = DEFAULT_ACCENT): ThemeModel {
  return {
    ...DEFAULT_THEME,
    mode,
    accent,
    tokens: {
      light: { ...BASE_TOKENS.light },
      dark: { ...BASE_TOKENS.dark },
    },
  };
}

export function cloneTheme(theme: ThemeModel): ThemeModel {
  return {
    mode: theme.mode,
    accent: theme.accent,
    density: theme.density,
    font_scale: theme.font_scale,
    tokens: {
      light: { ...theme.tokens.light },
      dark: { ...theme.tokens.dark },
    },
  };
}

export function normalizeTheme(value: unknown): ThemeModel {
  if (!isRecord(value)) return cloneTheme(DEFAULT_THEME);
  const mode = isThemeMode(value.mode) ? value.mode : 'system';
  const accent = isHex(value.accent) ? value.accent : DEFAULT_ACCENT;
  const density = value.density === 'comfortable' ? 'comfortable' : 'compact';
  const fontScale = value.font_scale === 'small' || value.font_scale === 'large' ? value.font_scale : 'default';

  return {
    mode,
    accent,
    density,
    font_scale: fontScale,
    tokens: normalizeTokenGroups(value.tokens),
  };
}

/**
 * Project a ThemeModel onto the CSS custom properties consumed by `@navi/ui`
 * primitives and app stylesheets. Writes to `target` (defaults to `#root`).
 * Safe to call repeatedly; safe to call when no DOM is present (no-op).
 */
export function applyTheme(theme: ThemeModel, target?: HTMLElement | null): void {
  const el = target ?? (typeof document !== 'undefined' ? document.getElementById('root') : null);
  if (!el) return;

  const resolvedMode = effectiveMode(theme.mode);
  const tokens = theme.tokens[resolvedMode];

  el.style.setProperty('color-scheme', resolvedMode);
  el.style.setProperty('--navi-bg', tokens.background);
  el.style.setProperty('--navi-panel', tokens.surface);
  el.style.setProperty('--navi-panel-hover', panelHoverFor(resolvedMode));
  el.style.setProperty('--navi-text', tokens.text);
  el.style.setProperty('--navi-muted', tokens.muted);
  el.style.setProperty('--navi-border', tokens.border);
  el.style.setProperty('--navi-accent', theme.accent);
  el.style.setProperty('--navi-accent-dim', hexToRgba(theme.accent, 0.16));
  el.style.setProperty('--navi-danger', tokens.danger);
  el.style.setProperty('--navi-danger-dim', hexToRgba(tokens.danger, 0.16));
  el.style.setProperty('--navi-warning', tokens.warning);
  el.style.setProperty('--navi-warning-dim', hexToRgba(tokens.warning, 0.16));
  el.style.setProperty('--navi-success', tokens.success);
  el.style.setProperty('--navi-success-dim', hexToRgba(tokens.success, 0.16));
  el.style.setProperty('--navi-font-size', fontSizeFor(theme.font_scale));
  el.style.setProperty('--navi-density-unit', theme.density === 'comfortable' ? '1.18' : '1');
  el.style.setProperty('--font-mono', 'var(--navi-font-mono)');
  el.style.setProperty('--color-bg-secondary', 'var(--navi-panel)');
  el.style.setProperty('--color-bg-tertiary', 'var(--navi-panel-hover)');
  el.style.setProperty('--color-bg-tertiary-rgb', rgbString(panelHoverFor(resolvedMode)));
  el.style.setProperty('--color-text-primary', 'var(--navi-text)');
  el.style.setProperty('--color-text-secondary', 'var(--navi-muted)');
  el.style.setProperty('--color-text-tertiary', 'var(--navi-muted)');
  el.style.setProperty('--color-border', 'var(--navi-border)');
  el.style.setProperty('--color-accent', 'var(--navi-accent)');
  el.style.setProperty('--color-warning', 'var(--navi-warning)');
  el.style.setProperty('--color-danger', 'var(--navi-danger)');
  el.style.setProperty('--color-success', 'var(--navi-success)');
}

/** WCAG relative-contrast ratio between two hex colors (1..21). */
export function contrastRatio(foreground: string, background: string): number {
  const fg = relativeLuminance(hexToRgb(foreground));
  const bg = relativeLuminance(hexToRgb(background));
  const light = Math.max(fg, bg);
  const dark = Math.min(fg, bg);
  return (light + 0.05) / (dark + 0.05);
}

function normalizeTokenGroups(value: unknown): ThemeTokenGroups {
  if (!isRecord(value)) {
    return { light: { ...BASE_TOKENS.light }, dark: { ...BASE_TOKENS.dark } };
  }

  const hasGroups = isRecord(value.light) || isRecord(value.dark);
  if (hasGroups) {
    return {
      light: normalizeTokens(value.light, BASE_TOKENS.light),
      dark: normalizeTokens(value.dark, BASE_TOKENS.dark),
    };
  }

  const legacy = normalizeTokens(value, BASE_TOKENS.dark);
  return {
    light: { ...BASE_TOKENS.light },
    dark: legacy,
  };
}

function normalizeTokens(value: unknown, fallback: ThemeTokens): ThemeTokens {
  if (!isRecord(value)) return { ...fallback };
  return {
    background: isHex(value.background) ? value.background : fallback.background,
    surface: isHex(value.surface) ? value.surface : fallback.surface,
    text: isHex(value.text) ? value.text : fallback.text,
    muted: isHex(value.muted) ? value.muted : fallback.muted,
    border: isHex(value.border) ? value.border : fallback.border,
    danger: isHex(value.danger) ? value.danger : fallback.danger,
    warning: isHex(value.warning) ? value.warning : fallback.warning,
    success: isHex(value.success) ? value.success : fallback.success,
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isThemeMode(value: unknown): value is ThemeMode {
  return value === 'system' || value === 'light' || value === 'dark';
}

function isHex(value: unknown): value is string {
  return typeof value === 'string' && /^#[0-9a-fA-F]{6}$/.test(value);
}

function hexToRgba(hex: string, alpha: number): string {
  const { r, g, b } = hexToRgb(hex);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

function hexToRgb(hex: string): { r: number; g: number; b: number } {
  const value = hex.replace('#', '');
  const parsed = Number.parseInt(value, 16);
  if (Number.isNaN(parsed)) return { r: 59, g: 130, b: 246 };
  return {
    r: (parsed >> 16) & 255,
    g: (parsed >> 8) & 255,
    b: parsed & 255,
  };
}

function relativeLuminance({ r, g, b }: { r: number; g: number; b: number }): number {
  const channel = [r, g, b].map((value) => {
    const normalized = value / 255;
    return normalized <= 0.03928 ? normalized / 12.92 : ((normalized + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * channel[0] + 0.7152 * channel[1] + 0.0722 * channel[2];
}

function panelHoverFor(mode: EffectiveMode): string {
  return mode === 'dark' ? '#1a2332' : '#eef2f7';
}

function rgbString(hex: string): string {
  const { r, g, b } = hexToRgb(hex);
  return `${r}, ${g}, ${b}`;
}

function fontSizeFor(scale: FontScale): string {
  if (scale === 'small') return '13px';
  if (scale === 'large') return '15px';
  return '14px';
}
