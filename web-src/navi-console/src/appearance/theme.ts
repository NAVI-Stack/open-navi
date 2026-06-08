// Console appearance layer.
//
// The generic theme ENGINE — the token data model, `applyTheme`, and the
// color/contrast utilities — now lives in the shared @navi/ui library. This
// module re-exports that engine (so existing `@/appearance/theme` imports keep
// working) and adds the Console-specific concerns on top: named presets, server
// persistence, and appearance schema versioning.

import {
  applyTheme,
  cloneTheme,
  normalizeTheme,
  createDefaultTheme,
  activeTokens,
  systemMode,
  effectiveMode,
  contrastRatio,
  DEFAULT_ACCENT,
  BASE_TOKENS,
  DEFAULT_THEME,
  type ThemeMode,
  type EffectiveMode,
  type Density,
  type FontScale,
  type ThemeTokens,
  type ThemeTokenGroups,
  type ThemeModel,
} from '@navi/ui';

// Re-export the shared engine so call sites can keep importing from here.
export {
  applyTheme,
  cloneTheme,
  normalizeTheme,
  createDefaultTheme,
  activeTokens,
  systemMode,
  effectiveMode,
  contrastRatio,
  DEFAULT_ACCENT,
  BASE_TOKENS,
  DEFAULT_THEME,
};
export type {
  ThemeMode,
  EffectiveMode,
  Density,
  FontScale,
  ThemeTokens,
  ThemeTokenGroups,
  ThemeModel,
};

/** Back-compat alias: the Console historically applied themes via this name. */
export const applyConsoleTheme = applyTheme;

export const APPEARANCE_SCHEMA_VERSION = 'console.appearance.v1';

export type ThemePreset = {
  id: string;
  name: string;
  theme: ThemeModel;
  updated_at?: string;
};

export type ConsoleAppearanceState = {
  schema_version: 'console.appearance.v1';
  selected_preset_id: string;
  theme: ThemeModel;
  presets: ThemePreset[];
  updated_at?: string;
};

export type ConsoleAppearanceResponse = {
  appearance: ConsoleAppearanceState;
  persisted: boolean;
};

export const DEFAULT_APPEARANCE: ConsoleAppearanceState = {
  schema_version: APPEARANCE_SCHEMA_VERSION,
  selected_preset_id: 'default',
  theme: DEFAULT_THEME,
  presets: [],
};

export function createDefaultAppearance(): ConsoleAppearanceState {
  return {
    ...DEFAULT_APPEARANCE,
    theme: cloneTheme(DEFAULT_THEME),
    presets: [],
  };
}

export function cloneAppearance(state: ConsoleAppearanceState): ConsoleAppearanceState {
  return {
    schema_version: APPEARANCE_SCHEMA_VERSION,
    selected_preset_id: state.selected_preset_id || 'default',
    theme: cloneTheme(state.theme),
    presets: state.presets.map((preset) => ({
      ...preset,
      theme: cloneTheme(preset.theme),
    })),
    updated_at: state.updated_at,
  };
}

export function normalizeAppearance(value: unknown): ConsoleAppearanceState {
  if (!isRecord(value)) return createDefaultAppearance();
  const source = isRecord(value.appearance) ? value.appearance : value;
  if (!isRecord(source)) return createDefaultAppearance();

  const presetsRaw = Array.isArray(source.presets) ? source.presets : [];
  const presets = presetsRaw
    .filter(isRecord)
    .map((preset) => ({
      id: cleanPresetID(String(preset.id ?? preset.name ?? 'preset')),
      name: String(preset.name ?? preset.id ?? 'Preset').slice(0, 48),
      theme: normalizeTheme(preset.theme),
      updated_at: typeof preset.updated_at === 'string' ? preset.updated_at : undefined,
    }))
    .filter((preset) => preset.id && preset.id !== 'default' && preset.name);

  const selected = cleanPresetID(String(source.selected_preset_id ?? 'default')) || 'default';
  const selectedExists = selected === 'default' || presets.some((preset) => preset.id === selected);

  return {
    schema_version: APPEARANCE_SCHEMA_VERSION,
    selected_preset_id: selectedExists ? selected : 'default',
    theme: normalizeTheme(source.theme),
    presets,
    updated_at: typeof source.updated_at === 'string' ? source.updated_at : undefined,
  };
}

export async function loadPersistedAppearance(): Promise<ConsoleAppearanceResponse | null> {
  try {
    const res = await fetch('/api/console/appearance', { credentials: 'same-origin' });
    if (!res.ok) return null;
    const data = await res.json();
    return {
      appearance: normalizeAppearance(data),
      persisted: Boolean(data?.persisted),
    };
  } catch {
    return null;
  }
}

export function cleanPresetID(raw: string): string {
  return raw
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
