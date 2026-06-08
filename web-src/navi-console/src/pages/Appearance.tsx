import { useEffect, useMemo, useRef, useState, type ComponentType } from 'react';
import {
  AlertTriangle,
  CheckCircle2,
  Download,
  Monitor,
  Moon,
  RotateCcw,
  Save,
  Sun,
  Trash2,
  Upload,
} from 'lucide-react';
import { useConsoleAppearance, useSaveConsoleAppearance } from '@/api/appearance';
import { JsonPanel } from '@/components/JsonPanel';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { ContextualChatBar } from '@/components/appearance/ContextualChatBar';
import {
  APPEARANCE_SCHEMA_VERSION,
  BASE_TOKENS,
  DEFAULT_ACCENT,
  activeTokens,
  applyConsoleTheme,
  cleanPresetID,
  cloneAppearance,
  cloneTheme,
  contrastRatio,
  createDefaultAppearance,
  createDefaultTheme,
  effectiveMode,
  normalizeAppearance,
  type ConsoleAppearanceState,
  type Density,
  type EffectiveMode,
  type FontScale,
  type ThemeMode,
  type ThemeModel,
  type ThemeTokens,
} from '@/appearance/theme';
import styles from './Appearance.module.css';

type ModeOption = {
  value: ThemeMode;
  label: string;
  Icon: ComponentType<{ size?: number }>;
};

const MODE_OPTIONS: ModeOption[] = [
  { value: 'system', label: 'System', Icon: Monitor },
  { value: 'dark', label: 'Dark', Icon: Moon },
  { value: 'light', label: 'Light', Icon: Sun },
];

const ACCENT_PRESETS = ['#3b82f6', '#38bdf8', '#14b8a6', '#22c55e', '#f59e0b', '#ef4444'];

const TOKEN_LABELS: Array<{ key: keyof ThemeTokens; label: string }> = [
  { key: 'background', label: 'Background' },
  { key: 'surface', label: 'Surface' },
  { key: 'text', label: 'Text' },
  { key: 'muted', label: 'Muted' },
  { key: 'border', label: 'Border' },
  { key: 'danger', label: 'Danger' },
  { key: 'warning', label: 'Warning' },
  { key: 'success', label: 'Success' },
];

const FONT_OPTIONS: Array<{ value: FontScale; label: string }> = [
  { value: 'small', label: 'Small' },
  { value: 'default', label: 'Default' },
  { value: 'large', label: 'Large' },
];

const DENSITY_OPTIONS: Array<{ value: Density; label: string }> = [
  { value: 'compact', label: 'Compact' },
  { value: 'comfortable', label: 'Comfortable' },
];

export function Appearance() {
  const appearanceQuery = useConsoleAppearance();
  const saveAppearance = useSaveConsoleAppearance();
  const hydrated = useRef(false);
  const [appearance, setAppearance] = useState<ConsoleAppearanceState>(() => createDefaultAppearance());
  const [dirty, setDirty] = useState(false);
  const [tokenGroup, setTokenGroup] = useState<EffectiveMode>('dark');
  const [presetName, setPresetName] = useState('');
  const [importText, setImportText] = useState('');
  const [importError, setImportError] = useState<string | null>(null);

  useEffect(() => {
    if (!appearanceQuery.data || (hydrated.current && dirty)) return;
    setAppearance(cloneAppearance(appearanceQuery.data.appearance));
    hydrated.current = true;
    setDirty(false);
  }, [appearanceQuery.data, dirty]);

  useEffect(() => {
    applyConsoleTheme(appearance.theme);
  }, [appearance.theme]);

  useEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: light)');
    const handleChange = () => applyConsoleTheme(appearance.theme);
    media.addEventListener('change', handleChange);
    return () => media.removeEventListener('change', handleChange);
  }, [appearance.theme]);

  const theme = appearance.theme;
  const resolvedMode = effectiveMode(theme.mode);
  const tokens = activeTokens(theme);
  const exportedJSON = useMemo(() => JSON.stringify(appearance, null, 2), [appearance]);
  const warnings = useMemo(() => contrastWarnings(theme), [theme]);
  const persistenceError = saveAppearance.error || appearanceQuery.error;

  const updateAppearance = (updater: (current: ConsoleAppearanceState) => ConsoleAppearanceState) => {
    setAppearance((current) => {
      const next = updater(cloneAppearance(current));
      return normalizeAppearance(next);
    });
    setDirty(true);
  };

  const updateTheme = (updater: (theme: ThemeModel) => ThemeModel) => {
    updateAppearance((current) => ({
      ...current,
      theme: updater(cloneTheme(current.theme)),
    }));
  };

  const persist = async (state = appearance) => {
    const normalized = normalizeAppearance(state);
    const saved = await saveAppearance.mutateAsync(normalized);
    setAppearance(cloneAppearance(saved.appearance));
    setDirty(false);
  };

  const savePreset = async () => {
    const trimmed = presetName.trim();
    const name = trimmed || currentPresetName(appearance) || 'Console Theme';
    const id = cleanPresetID(name);
    if (!id || id === 'default') {
      setImportError('Choose a non-default preset name.');
      return;
    }
    const now = new Date().toISOString();
    const next = cloneAppearance(appearance);
    const existing = next.presets.findIndex((preset) => preset.id === id);
    const preset = { id, name, theme: cloneTheme(next.theme), updated_at: now };
    if (existing >= 0) {
      next.presets[existing] = preset;
    } else {
      next.presets = [...next.presets, preset];
    }
    next.selected_preset_id = id;
    await persist(next);
    setPresetName('');
    setImportError(null);
  };

  const deleteSelectedPreset = async () => {
    if (appearance.selected_preset_id === 'default') return;
    const next = cloneAppearance(appearance);
    next.presets = next.presets.filter((preset) => preset.id !== next.selected_preset_id);
    next.selected_preset_id = 'default';
    next.theme = createDefaultTheme();
    await persist(next);
  };

  const selectPreset = (id: string) => {
    updateAppearance((current) => {
      if (id === 'default') {
        current.selected_preset_id = 'default';
        current.theme = createDefaultTheme();
        return current;
      }
      const preset = current.presets.find((item) => item.id === id);
      if (!preset) return current;
      current.selected_preset_id = preset.id;
      current.theme = cloneTheme(preset.theme);
      return current;
    });
  };

  const resetAccent = () => updateTheme((current) => ({ ...current, accent: DEFAULT_ACCENT }));

  const resetTokensForGroup = () => {
    updateTheme((current) => ({
      ...current,
      tokens: {
        ...current.tokens,
        [tokenGroup]: { ...BASE_TOKENS[tokenGroup] },
      },
    }));
  };

  const resetAll = () => {
    setAppearance(createDefaultAppearance());
    setDirty(true);
  };

  const importAppearance = () => {
    setImportError(null);
    try {
      const parsed = JSON.parse(importText);
      const next = normalizeAppearance(parsed);
      setAppearance(next);
      setDirty(true);
    } catch (err) {
      setImportError(err instanceof Error ? err.message : 'Invalid appearance JSON.');
    }
  };

  const downloadExport = () => {
    const blob = new Blob([exportedJSON], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'navi-console-appearance.json';
    link.click();
    URL.revokeObjectURL(url);
  };

  const persistenceLabel = saveAppearance.isPending
    ? 'Saving'
    : dirty
      ? 'Unsaved'
      : appearanceQuery.data?.persisted
        ? 'Persisted'
        : 'Default';

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div>
          <h1 className={styles.heading}>Appearance</h1>
          <div className={styles.subtle}>Durable owner console preferences</div>
        </div>
        <div className={styles.headerActions}>
          <StatusBadge
            label={persistenceLabel}
            variant={saveAppearance.isError ? 'danger' : dirty ? 'warning' : 'success'}
            size="md"
          />
          <button type="button" className={styles.resetButton} onClick={() => persist()} disabled={saveAppearance.isPending || !dirty}>
            <Save size={15} />
            Save
          </button>
        </div>
      </div>

      <ContextualChatBar
        currentAppearance={appearance}
        onPreviewPatch={(previewAppearance) => {
          if (previewAppearance) {
            applyConsoleTheme(previewAppearance.theme);
          } else {
            applyConsoleTheme(appearance.theme);
          }
        }}
        onApplyPatch={(patchedAppearance) => {
          setAppearance(patchedAppearance);
          setDirty(true);
        }}
      />

      {persistenceError && (
        <div className={styles.statusPanel}>
          <AlertTriangle size={16} />
          <div>
            <div className={styles.statusTitle}>Persistence failed.</div>
            <div className={styles.errorText}>{persistenceError instanceof Error ? persistenceError.message : String(persistenceError)}</div>
          </div>
        </div>
      )}

      <div className={styles.layout}>
        <section className={styles.panel}>
          <div className={styles.sectionHeader}>
            <h2>Presets</h2>
            <span>{appearance.presets.length} saved</span>
          </div>
          <div className={styles.presetRow}>
            <select value={appearance.selected_preset_id} onChange={(event) => selectPreset(event.target.value)}>
              <option value="default">Default</option>
              {appearance.presets.map((preset) => (
                <option key={preset.id} value={preset.id}>{preset.name}</option>
              ))}
            </select>
            <button type="button" className={styles.iconButton} onClick={deleteSelectedPreset} disabled={appearance.selected_preset_id === 'default'} title="Delete selected preset">
              <Trash2 size={15} />
            </button>
          </div>
          <div className={styles.savePresetRow}>
            <input
              type="text"
              value={presetName}
              onChange={(event) => setPresetName(event.target.value)}
              placeholder="Preset name"
            />
            <button type="button" className={styles.secondaryButton} onClick={savePreset} disabled={saveAppearance.isPending}>
              Save Preset
            </button>
          </div>

          <div className={styles.sectionHeader}>
            <h2>Mode</h2>
            <span>{resolvedMode}</span>
          </div>
          <div className={styles.segmented} role="group" aria-label="Appearance mode">
            {MODE_OPTIONS.map(({ value, label, Icon }) => (
              <button
                key={value}
                type="button"
                className={theme.mode === value ? styles.segmentActive : styles.segment}
                onClick={() => updateTheme((current) => ({ ...current, mode: value }))}
              >
                <Icon size={15} />
                {label}
              </button>
            ))}
          </div>

          <div className={styles.sectionHeader}>
            <h2>Comfort</h2>
          </div>
          <div className={styles.compactGrid}>
            <label>
              <span>Density</span>
              <select value={theme.density} onChange={(event) => updateTheme((current) => ({ ...current, density: event.target.value as Density }))}>
                {DENSITY_OPTIONS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </label>
            <label>
              <span>Font Size</span>
              <select value={theme.font_scale} onChange={(event) => updateTheme((current) => ({ ...current, font_scale: event.target.value as FontScale }))}>
                {FONT_OPTIONS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </label>
          </div>

          <div className={styles.sectionHeader}>
            <h2>Accent</h2>
            <button type="button" className={styles.linkButton} onClick={resetAccent}>Reset</button>
          </div>
          <div className={styles.accentGrid}>
            {ACCENT_PRESETS.map((accent) => (
              <button
                key={accent}
                type="button"
                className={theme.accent === accent ? styles.swatchActive : styles.swatch}
                style={{ background: accent }}
                onClick={() => updateTheme((current) => ({ ...current, accent }))}
                aria-label={`Use accent ${accent}`}
                title={accent}
              />
            ))}
            <label className={styles.colorField}>
              <span>Custom</span>
              <input type="color" value={theme.accent} onChange={(event) => updateTheme((current) => ({ ...current, accent: event.target.value }))} />
            </label>
          </div>

          <div className={styles.sectionHeader}>
            <h2>Semantic Tokens</h2>
            <button type="button" className={styles.linkButton} onClick={resetTokensForGroup}>Reset {tokenGroup}</button>
          </div>
          <div className={styles.segmented} role="group" aria-label="Token group">
            {(['dark', 'light'] as EffectiveMode[]).map((group) => (
              <button
                key={group}
                type="button"
                className={tokenGroup === group ? styles.segmentActive : styles.segment}
                onClick={() => setTokenGroup(group)}
              >
                {group}
              </button>
            ))}
          </div>
          <div className={styles.tokenGrid}>
            {TOKEN_LABELS.map(({ key, label }) => (
              <label key={key} className={styles.tokenControl}>
                <span>{label}</span>
                <input
                  type="color"
                  value={theme.tokens[tokenGroup][key]}
                  onChange={(event) => updateTheme((current) => ({
                    ...current,
                    tokens: {
                      ...current.tokens,
                      [tokenGroup]: {
                        ...current.tokens[tokenGroup],
                        [key]: event.target.value,
                      },
                    },
                  }))}
                />
                <code>{theme.tokens[tokenGroup][key]}</code>
              </label>
            ))}
          </div>

          <div className={styles.resetRow}>
            <button type="button" className={styles.secondaryButton} onClick={resetAll}>
              <RotateCcw size={15} />
              Reset All
            </button>
          </div>
        </section>

        <section className={styles.panel}>
          <div className={styles.sectionHeader}>
            <h2>Preview</h2>
            <span>{currentPresetName(appearance) || 'custom'}</span>
          </div>

          <div className={styles.previewSurface}>
            <div className={styles.previewTopline}>
              <span>Operator snapshot</span>
              <StatusBadge label="ready" variant="success" size="sm" />
            </div>
            <div className={styles.previewCards}>
              <div className={styles.previewCard}>
                <div className={styles.cardLabel}>Active run</div>
                <div className={styles.cardValue}>model routing</div>
              </div>
              <div className={styles.previewCard}>
                <div className={styles.cardLabel}>Queue</div>
                <div className={styles.cardValue}>3 pending</div>
              </div>
            </div>
            <div className={styles.previewActions}>
              <button type="button" className={styles.primaryButton}>Approve</button>
              <button type="button" className={styles.secondaryButton}>Defer</button>
            </div>
            <form className={styles.previewForm}>
              <label>
                <span>Display name</span>
                <input type="text" value="NAVI Console" readOnly />
              </label>
              <label>
                <span>Density</span>
                <select value={theme.density} disabled>
                  <option value={theme.density}>{theme.density}</option>
                </select>
              </label>
            </form>
          </div>

          <div className={styles.sectionHeader}>
            <h2>Contrast</h2>
            {warnings.length === 0 ? <CheckCircle2 size={15} className={styles.successIcon} /> : <AlertTriangle size={15} className={styles.warningIcon} />}
          </div>
          {warnings.length === 0 ? (
            <div className={styles.okState}>Core text contrast checks pass for the active mode.</div>
          ) : (
            <div className={styles.warningList}>
              {warnings.map((warning) => (
                <div key={warning} className={styles.warningItem}>{warning}</div>
              ))}
            </div>
          )}

          <div className={styles.tokenPreview}>
            {TOKEN_LABELS.map(({ key, label }) => (
              <div key={key} className={styles.tokenChip}>
                <span className={styles.tokenDot} style={{ background: tokens[key] }} />
                <span>{label}</span>
              </div>
            ))}
          </div>
        </section>

        <section className={`${styles.panel} ${styles.rawPanel}`}>
          <div className={styles.sectionHeader}>
            <h2>Import / Export</h2>
            <button type="button" className={styles.iconTextButton} onClick={downloadExport}>
              <Download size={15} />
              Export
            </button>
          </div>
          <textarea className={styles.jsonArea} value={exportedJSON} readOnly aria-label="Exported appearance JSON" />
          <textarea
            className={styles.jsonArea}
            value={importText}
            onChange={(event) => setImportText(event.target.value)}
            placeholder={`Paste ${APPEARANCE_SCHEMA_VERSION} JSON`}
            aria-label="Import appearance JSON"
          />
          <div className={styles.importActions}>
            <button type="button" className={styles.secondaryButton} onClick={importAppearance} disabled={!importText.trim()}>
              <Upload size={15} />
              Import Preview
            </button>
            {importError && <span className={styles.errorText}>{importError}</span>}
          </div>
          <JsonPanel data={appearance} label="appearance.json" />
        </section>
      </div>
    </div>
  );
}

function currentPresetName(appearance: ConsoleAppearanceState): string {
  if (appearance.selected_preset_id === 'default') return 'Default';
  return appearance.presets.find((preset) => preset.id === appearance.selected_preset_id)?.name ?? '';
}

function contrastWarnings(theme: ThemeModel): string[] {
  const tokens = activeTokens(theme);
  const checks = [
    { label: 'Text on background', fg: tokens.text, bg: tokens.background, min: 4.5 },
    { label: 'Text on surface', fg: tokens.text, bg: tokens.surface, min: 4.5 },
    { label: 'Muted text on surface', fg: tokens.muted, bg: tokens.surface, min: 3 },
    { label: 'Accent on background', fg: theme.accent, bg: tokens.background, min: 3 },
  ];
  return checks.flatMap((check) => {
    const ratio = contrastRatio(check.fg, check.bg);
    return ratio < check.min ? [`${check.label}: ${ratio.toFixed(2)}:1, below ${check.min}:1`] : [];
  });
}
