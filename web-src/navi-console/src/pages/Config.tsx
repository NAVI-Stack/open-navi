import { useState, useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { JsonPanel } from '@/components/JsonPanel';
import { User, Sliders, Activity, Cpu, ChevronDown, ChevronRight, Wifi, WifiOff, RefreshCw, Check, AlertCircle } from 'lucide-react';
import styles from './Config.module.css';

import {
  useAuthMe,
  useIdentity,
  useExperience,
  useExperienceInspect,
  useExperienceModuleRegistry,
  usePresenceSnapshot,
  useNaviPresence,
  updateUserPresence,
  useLlmActive,
  useLlmPreferences,
  useLlmProviders,
  useLlmProfiles,
  useLlmProviderModels,
  useLlmProviderHealth,
  setLlmActive,
  patchLlmPreferences,
  configureLlmProvider,
  disableLlmProvider,
} from '@/api/config';
import type { LlmProviderDescriptor, LlmModelProfile } from '@/types/api';

type TabType = 'identity' | 'experience' | 'presence' | 'llm';

export function Config() {
  const [activeTab, setActiveTab] = useState<TabType>('identity');

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.heading}>Config</h1>
      </div>

      <div className={styles.tabs}>
        <div
          className={`${styles.tab} ${activeTab === 'identity' ? styles.active : ''}`}
          onClick={() => setActiveTab('identity')}
        >
          Identity
        </div>
        <div
          className={`${styles.tab} ${activeTab === 'experience' ? styles.active : ''}`}
          onClick={() => setActiveTab('experience')}
        >
          Experience
        </div>
        <div
          className={`${styles.tab} ${activeTab === 'presence' ? styles.active : ''}`}
          onClick={() => setActiveTab('presence')}
        >
          Presence
        </div>
        <div
          className={`${styles.tab} ${activeTab === 'llm' ? styles.active : ''}`}
          onClick={() => setActiveTab('llm')}
        >
          LLM
        </div>
      </div>

      <div className={styles.layout}>
        {activeTab === 'identity' && <IdentityTab />}
        {activeTab === 'experience' && <ExperienceTab />}
        {activeTab === 'presence' && <PresenceTab />}
        {activeTab === 'llm' && <LlmTab />}
      </div>
    </div>
  );
}

function IdentityTab() {
  const { data: auth, isLoading: authLoading, error: authError } = useAuthMe();
  const { data: identity, isLoading: idLoading, error: idError } = useIdentity();

  return (
    <div className={styles.contentPane}>
      <div className={styles.section}>
        <div className={styles.sectionTitle}>
          <User size={16} style={{ display: 'inline', marginRight: 8, verticalAlign: 'text-bottom' }} />
          Identity
        </div>
        
        {authError && <div className={styles.errorRow}>Auth Error: {(authError as Error).message}</div>}
        {idError && <div className={styles.errorRow}>Identity Error: {(idError as Error).message}</div>}

        <div className={styles.sectionLabel}>Auth Principal</div>
        {authLoading ? <div className={styles.loadingRow}>Loading...</div> : <JsonPanel data={auth ?? {}} />}

        <div className={styles.sectionLabel} style={{ marginTop: 20 }}>Active NAVI Identity</div>
        {idLoading ? <div className={styles.loadingRow}>Loading...</div> : <JsonPanel data={identity ?? {}} />}
      </div>
    </div>
  );
}

function ExperienceTab() {
  const { data: exp, isLoading: expLoading, error: expError } = useExperience();
  const { data: inspect, isLoading: insLoading, error: insError } = useExperienceInspect();
  const { data: registry, isLoading: regLoading, error: regError } = useExperienceModuleRegistry();

  return (
    <div className={styles.contentPane}>
      <div className={styles.section}>
        <div className={styles.sectionTitle}>
          <Sliders size={16} style={{ display: 'inline', marginRight: 8, verticalAlign: 'text-bottom' }} />
          Experience Configuration
        </div>

        {expError && <div className={styles.errorRow}>Experience Error: {(expError as Error).message}</div>}
        {insError && <div className={styles.errorRow}>Inspect Error: {(insError as Error).message}</div>}
        {regError && <div className={styles.errorRow}>Registry Error: {(regError as Error).message}</div>}

        <div className={styles.sectionLabel}>Stored Experience Config</div>
        {expLoading ? <div className={styles.loadingRow}>Loading...</div> : <JsonPanel data={exp ?? {}} />}

        <div className={styles.sectionLabel} style={{ marginTop: 20 }}>Effective / Inspected Output</div>
        {insLoading ? <div className={styles.loadingRow}>Loading...</div> : <JsonPanel data={inspect ?? {}} />}

        <div className={styles.sectionLabel} style={{ marginTop: 20 }}>Module Registry</div>
        {regLoading ? <div className={styles.loadingRow}>Loading...</div> : <JsonPanel data={registry ?? {}} />}
      </div>
    </div>
  );
}

function PresenceTab() {
  const queryClient = useQueryClient();
  const { data: snapshot, isLoading: snapLoading, error: snapError } = usePresenceSnapshot();
  const { data: naviPresence, isLoading: naviLoading, error: naviError } = useNaviPresence();

  const [statusInput, setStatusInput] = useState('');
  const [updating, setUpdating] = useState(false);
  const [updateError, setUpdateError] = useState<string | null>(null);

  const handleUpdate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!statusInput.trim()) return;

    setUpdating(true);
    setUpdateError(null);
    try {
      await updateUserPresence({ public_status: statusInput.trim() });
      queryClient.invalidateQueries({ queryKey: ['presence'] });
      setStatusInput('');
    } catch (err) {
      setUpdateError(err instanceof Error ? err.message : 'Failed to update presence');
    } finally {
      setUpdating(false);
    }
  };

  return (
    <div className={styles.contentPane}>
      <div className={styles.section}>
        <div className={styles.sectionTitle}>
          <Activity size={16} style={{ display: 'inline', marginRight: 8, verticalAlign: 'text-bottom' }} />
          Presence
        </div>

        {snapError && <div className={styles.errorRow}>Snapshot Error: {(snapError as Error).message}</div>}
        {naviError && <div className={styles.errorRow}>NAVI Error: {(naviError as Error).message}</div>}

        <div className={styles.sectionLabel}>Update User Presence</div>
        <form onSubmit={handleUpdate} style={{ display: 'flex', gap: 10, alignItems: 'flex-start', marginBottom: 20 }}>
          <div style={{ flex: 1 }}>
            <input
              type="text"
              className={styles.input}
              placeholder="Enter new public status (e.g. busy, available)"
              value={statusInput}
              onChange={(e) => setStatusInput(e.target.value)}
              disabled={updating}
            />
          </div>
          <button type="submit" className={`${styles.actionBtn} ${styles.primary}`} disabled={updating || !statusInput.trim()}>
            {updating ? 'Updating...' : 'Update Status'}
          </button>
        </form>
        {updateError && <div className={styles.errorRow} style={{ marginTop: -10, marginBottom: 15 }}>{updateError}</div>}

        <div className={styles.sectionLabel}>Full Presence Snapshot</div>
        {snapLoading ? <div className={styles.loadingRow}>Loading...</div> : <JsonPanel data={snapshot ?? {}} />}

        <div className={styles.sectionLabel} style={{ marginTop: 20 }}>NAVI Presence</div>
        {naviLoading ? <div className={styles.loadingRow}>Loading...</div> : <JsonPanel data={naviPresence ?? {}} />}
      </div>
    </div>
  );
}

// ─── LLM Tab ────────────────────────────────────────────────────────────────

function LlmTab() {
  const queryClient = useQueryClient();
  const { data: active, isLoading: actLoading, error: actError } = useLlmActive();
  const { data: prefs, isLoading: prefsLoading, error: prefsError } = useLlmPreferences();
  const { data: providers, isLoading: provLoading, error: provError } = useLlmProviders();
  const { data: profiles, isLoading: profLoading, error: profError } = useLlmProfiles();

  return (
    <div className={styles.contentPane}>
      {/* Error banner row */}
      {actError && <div className={styles.errorRow}>Active Model Error: {(actError as Error).message}</div>}
      {prefsError && <div className={styles.errorRow}>Preferences Error: {(prefsError as Error).message}</div>}
      {provError && <div className={styles.errorRow}>Providers Error: {(provError as Error).message}</div>}
      {profError && <div className={styles.errorRow}>Profiles Error: {(profError as Error).message}</div>}

      {/* Section 1 — Active Model Switcher */}
      <div className={styles.llmSection}>
        <div className={styles.llmSectionHeader}>
          <Cpu size={15} className={styles.llmSectionIcon} />
          <span className={styles.llmSectionTitle}>Active Model</span>
          {active && !actLoading && (
            <span className={styles.activeModelBadge}>{active.status ?? `${active.provider}/${active.model}`}</span>
          )}
        </div>
        {actLoading || provLoading ? (
          <div className={styles.loadingRow}>Loading...</div>
        ) : (
          <ActiveModelSwitcher
            currentProvider={active?.provider ?? ''}
            currentModel={active?.model ?? ''}
            providers={providers?.providers ?? []}
            onApplied={() => queryClient.invalidateQueries({ queryKey: ['llm', 'active'] })}
          />
        )}
      </div>

      {/* Section 2 — Preferences Editor */}
      <div className={styles.llmSection}>
        <div className={styles.llmSectionHeader}>
          <Sliders size={15} className={styles.llmSectionIcon} />
          <span className={styles.llmSectionTitle}>Routing Preferences</span>
        </div>
        {prefsLoading || profLoading ? (
          <div className={styles.loadingRow}>Loading...</div>
        ) : (
          <PreferencesEditor
            prefs={prefs ?? {}}
            profiles={profiles?.profiles ?? []}
            onSaved={() => queryClient.invalidateQueries({ queryKey: ['llm', 'preferences'] })}
          />
        )}
      </div>

      {/* Section 3 — Providers List */}
      <div className={styles.llmSection}>
        <div className={styles.llmSectionHeader}>
          <Activity size={15} className={styles.llmSectionIcon} />
          <span className={styles.llmSectionTitle}>Providers</span>
          {providers && !provLoading && (
            <span className={styles.providerCountBadge}>{providers.providers?.length ?? 0} configured</span>
          )}
        </div>
        {provLoading ? (
          <div className={styles.loadingRow}>Loading...</div>
        ) : (
          <ProvidersList providers={providers?.providers ?? []} />
        )}
      </div>
    </div>
  );
}

// ─── Active Model Switcher ───────────────────────────────────────────────────

function ActiveModelSwitcher({
  currentProvider,
  currentModel,
  providers,
  onApplied,
}: {
  currentProvider: string;
  currentModel: string;
  providers: LlmProviderDescriptor[];
  onApplied: () => void;
}) {
  const [selectedProvider, setSelectedProvider] = useState(currentProvider);
  const [selectedModel, setSelectedModel] = useState(currentModel);
  const [saving, setSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>('idle');
  const [saveError, setSaveError] = useState<string | null>(null);

  // When provider changes, reset model selection
  useEffect(() => {
    if (selectedProvider !== currentProvider) {
      setSelectedModel('');
    } else {
      setSelectedModel(currentModel);
    }
  }, [selectedProvider, currentProvider, currentModel]);

  const { data: modelsData, isLoading: modelsLoading } = useLlmProviderModels(
    selectedProvider || null
  );
  const models = modelsData?.models ?? [];

  const isDirty = selectedProvider !== currentProvider || selectedModel !== currentModel;

  const handleApply = async () => {
    if (!selectedProvider || !selectedModel) return;
    setSaving(true);
    setSaveStatus('idle');
    setSaveError(null);
    try {
      await setLlmActive(selectedProvider, selectedModel);
      setSaveStatus('success');
      onApplied();
      setTimeout(() => setSaveStatus('idle'), 2500);
    } catch (err) {
      setSaveStatus('error');
      setSaveError(err instanceof Error ? err.message : 'Failed to update active model');
    } finally {
      setSaving(false);
    }
  };

  const enabledProviders = providers.filter(p => p.enabled);

  return (
    <div className={styles.switcherContainer}>
      <div className={styles.switcherRow}>
        {/* Provider selector */}
        <div className={styles.switcherField}>
          <label className={styles.switcherLabel}>Provider</label>
          <select
            className={styles.select}
            value={selectedProvider}
            onChange={e => setSelectedProvider(e.target.value)}
            disabled={saving}
          >
            {!selectedProvider && <option value="">Select provider…</option>}
            {enabledProviders.map(p => (
              <option key={p.key} value={p.key}>
                {p.display_name} ({p.kind})
                {p.healthy === false ? ' ⚠' : ''}
              </option>
            ))}
          </select>
        </div>

        {/* Model selector */}
        <div className={styles.switcherField}>
          <label className={styles.switcherLabel}>Model</label>
          <select
            className={styles.select}
            value={selectedModel}
            onChange={e => setSelectedModel(e.target.value)}
            disabled={saving || !selectedProvider || modelsLoading}
          >
            {modelsLoading ? (
              <option>Loading models…</option>
            ) : (
              <>
                {!selectedModel && <option value="">Select model…</option>}
                {models.map(m => (
                  <option key={m.name} value={m.name}>{m.name}</option>
                ))}
                {models.length === 0 && selectedProvider && (
                  <option value="" disabled>No models available</option>
                )}
              </>
            )}
          </select>
        </div>

        {/* Apply button */}
        <button
          className={`${styles.actionBtn} ${styles.primary} ${styles.switcherApplyBtn}`}
          onClick={handleApply}
          disabled={saving || !isDirty || !selectedProvider || !selectedModel}
        >
          {saving ? (
            <RefreshCw size={14} className={styles.spinIcon} />
          ) : saveStatus === 'success' ? (
            <><Check size={14} /> Applied</>
          ) : (
            'Apply'
          )}
        </button>
      </div>
      {saveStatus === 'error' && saveError && (
        <div className={styles.errorRow} style={{ marginTop: 8 }}>
          <AlertCircle size={13} style={{ marginRight: 5, verticalAlign: 'middle' }} />
          {saveError}
        </div>
      )}
      {!isDirty && currentProvider && currentModel && (
        <div className={styles.currentModelHint}>
          Currently active: <code>{currentProvider}/{currentModel}</code>
        </div>
      )}
    </div>
  );
}

// ─── Preferences Editor ──────────────────────────────────────────────────────

const TASK_CLASSES = [
  { key: 'default_agentic', label: 'Agentic', description: 'Multi-step autonomous tasks' },
  { key: 'default_coding', label: 'Coding', description: 'Code generation and review' },
  { key: 'default_chat', label: 'Chat', description: 'Conversational interactions' },
  { key: 'default_reasoning', label: 'Reasoning', description: 'Analysis and planning' },
  { key: 'default_lightweight', label: 'Lightweight', description: 'Quick status queries' },
] as const;

const ROUTING_VISIBILITY_OPTIONS = [
  { value: 'silent', label: 'Silent', description: 'Routing decisions are never surfaced' },
  { value: 'thinking', label: 'Thinking', description: 'Injected as LLM context only' },
  { value: 'debug', label: 'Debug', description: 'Shown when debug mode is enabled' },
  { value: 'user', label: 'User', description: 'Always shown in replies' },
];

function PreferencesEditor({
  prefs,
  profiles,
  onSaved,
}: {
  prefs: Record<string, unknown>;
  profiles: LlmModelProfile[];
  onSaved: () => void;
}) {
  const [form, setForm] = useState(() => ({
    default_agentic: (prefs.default_agentic as string) ?? '',
    default_coding: (prefs.default_coding as string) ?? '',
    default_chat: (prefs.default_chat as string) ?? '',
    default_reasoning: (prefs.default_reasoning as string) ?? '',
    default_lightweight: (prefs.default_lightweight as string) ?? '',
    prefer_local: (prefs.prefer_local as boolean) ?? false,
    cost_sensitive: (prefs.cost_sensitive as boolean) ?? false,
    routing_visibility: (prefs.routing_visibility as string) ?? 'silent',
  }));

  const [saving, setSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'success' | 'error'>('idle');
  const [saveError, setSaveError] = useState<string | null>(null);

  // Sync when prefs change from server
  useEffect(() => {
    setForm({
      default_agentic: (prefs.default_agentic as string) ?? '',
      default_coding: (prefs.default_coding as string) ?? '',
      default_chat: (prefs.default_chat as string) ?? '',
      default_reasoning: (prefs.default_reasoning as string) ?? '',
      default_lightweight: (prefs.default_lightweight as string) ?? '',
      prefer_local: (prefs.prefer_local as boolean) ?? false,
      cost_sensitive: (prefs.cost_sensitive as boolean) ?? false,
      routing_visibility: (prefs.routing_visibility as string) ?? 'silent',
    });
  }, [prefs]);

  // Build unique "provider/model" option list from profiles
  const modelOptions = profiles.map(p => ({
    value: `${p.provider_key}/${p.model_id}`,
    label: `${p.provider_key} / ${p.model_id}`,
  }));

  const handleSave = async () => {
    setSaving(true);
    setSaveStatus('idle');
    setSaveError(null);
    try {
      const patch: Record<string, unknown> = {};
      for (const key of Object.keys(form)) {
        const val = form[key as keyof typeof form];
        if (val !== '' && val !== undefined) patch[key] = val;
      }
      await patchLlmPreferences(patch);
      setSaveStatus('success');
      onSaved();
      setTimeout(() => setSaveStatus('idle'), 2500);
    } catch (err) {
      setSaveStatus('error');
      setSaveError(err instanceof Error ? err.message : 'Failed to save preferences');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className={styles.prefsContainer}>
      {/* Task class defaults grid */}
      <div className={styles.sectionLabel} style={{ marginBottom: 8 }}>Default model per task class</div>
      <div className={styles.taskClassGrid}>
        {TASK_CLASSES.map(tc => (
          <div key={tc.key} className={styles.taskClassItem}>
            <div className={styles.taskClassLabel}>{tc.label}</div>
            <div className={styles.taskClassDesc}>{tc.description}</div>
            <select
              className={styles.select}
              value={form[tc.key]}
              onChange={e => setForm(f => ({ ...f, [tc.key]: e.target.value }))}
              disabled={saving}
            >
              <option value="">Auto (scored routing)</option>
              {modelOptions.map(opt => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </div>
        ))}
      </div>

      {/* Flags row */}
      <div className={styles.sectionLabel} style={{ marginTop: 20, marginBottom: 10 }}>Routing flags</div>
      <div className={styles.flagsRow}>
        <label className={styles.toggleLabel}>
          <span className={styles.toggleText}>
            <span className={styles.toggleTitle}>Prefer Local</span>
            <span className={styles.toggleDesc}>Boost local (Ollama) models in scoring</span>
          </span>
          <div
            className={`${styles.toggle} ${form.prefer_local ? styles.toggleOn : ''}`}
            onClick={() => !saving && setForm(f => ({ ...f, prefer_local: !f.prefer_local }))}
            role="switch"
            aria-checked={form.prefer_local}
          >
            <div className={styles.toggleThumb} />
          </div>
        </label>

        <label className={styles.toggleLabel}>
          <span className={styles.toggleText}>
            <span className={styles.toggleTitle}>Cost Sensitive</span>
            <span className={styles.toggleDesc}>Prefer cheaper, faster models</span>
          </span>
          <div
            className={`${styles.toggle} ${form.cost_sensitive ? styles.toggleOn : ''}`}
            onClick={() => !saving && setForm(f => ({ ...f, cost_sensitive: !f.cost_sensitive }))}
            role="switch"
            aria-checked={form.cost_sensitive}
          >
            <div className={styles.toggleThumb} />
          </div>
        </label>
      </div>

      {/* Routing visibility */}
      <div className={styles.sectionLabel} style={{ marginTop: 20, marginBottom: 8 }}>Routing visibility</div>
      <div className={styles.visibilityRow}>
        {ROUTING_VISIBILITY_OPTIONS.map(opt => (
          <button
            key={opt.value}
            className={`${styles.visibilityBtn} ${form.routing_visibility === opt.value ? styles.visibilityBtnActive : ''}`}
            onClick={() => !saving && setForm(f => ({ ...f, routing_visibility: opt.value }))}
            title={opt.description}
            disabled={saving}
          >
            {opt.label}
          </button>
        ))}
      </div>
      <div className={styles.visibilityHint}>
        {ROUTING_VISIBILITY_OPTIONS.find(o => o.value === form.routing_visibility)?.description}
      </div>

      {/* Save row */}
      <div className={styles.prefsSaveRow}>
        {saveStatus === 'error' && saveError && (
          <span className={styles.saveErrorText}>
            <AlertCircle size={13} style={{ marginRight: 4, verticalAlign: 'middle' }} />
            {saveError}
          </span>
        )}
        <button
          className={`${styles.actionBtn} ${styles.primary}`}
          onClick={handleSave}
          disabled={saving}
        >
          {saving ? (
            <><RefreshCw size={14} className={styles.spinIcon} /> Saving…</>
          ) : saveStatus === 'success' ? (
            <><Check size={14} /> Saved</>
          ) : (
            'Save Preferences'
          )}
        </button>
      </div>
    </div>
  );
}

// ─── Providers List ──────────────────────────────────────────────────────────

function ProvidersList({ providers }: { providers: LlmProviderDescriptor[] }) {
  if (!providers || providers.length === 0) {
    return <div className={styles.loadingRow}>No providers configured.</div>;
  }

  return (
    <div className={styles.providersList}>
      {providers.map(p => (
        <ProviderCard key={p.key} provider={p} />
      ))}
    </div>
  );
}

function ProviderCard({ provider }: { provider: LlmProviderDescriptor }) {
  const [expanded, setExpanded] = useState(false);
  const [healthCheck, setHealthCheck] = useState(false);
  const [editing, setEditing] = useState(false);
  const [apiKey, setApiKey] = useState('');
  const [model, setModel] = useState('');
  const [endpoint, setEndpoint] = useState('');
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const { data: modelsData, isLoading: modelsLoading } = useLlmProviderModels(expanded ? provider.key : null);
  const { data: healthData, isLoading: healthLoading } = useLlmProviderHealth(healthCheck ? provider.key : null, healthCheck);

  const models = modelsData?.models ?? [];
  const isOllama = provider.key === 'ollama';

  async function handleSaveCredentials() {
    setSaving(true);
    setSaveError(null);
    try {
      await configureLlmProvider(provider.key, {
        ...(apiKey ? { api_key: apiKey } : {}),
        ...(model ? { model } : {}),
        ...(endpoint ? { endpoint } : {}),
      });
      void queryClient.invalidateQueries({ queryKey: ['llm', 'providers'] });
      void queryClient.invalidateQueries({ queryKey: ['llm', 'profiles'] });
      setEditing(false);
      setApiKey('');
      setModel('');
      setEndpoint('');
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  }

  async function handleDisconnect() {
    try {
      await disableLlmProvider(provider.key);
      void queryClient.invalidateQueries({ queryKey: ['llm', 'providers'] });
      void queryClient.invalidateQueries({ queryKey: ['llm', 'profiles'] });
    } catch {
      // Silently ignore
    }
  }

  return (
    <div className={`${styles.providerCard} ${!provider.enabled ? styles.providerCardDisabled : ''}`}>
      <div className={styles.providerCardHeader}>
        <div className={styles.providerCardLeft}>
          <button
            className={styles.providerExpandBtn}
            onClick={() => setExpanded(e => !e)}
            aria-label={expanded ? 'Collapse' : 'Expand'}
          >
            {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </button>
          <div className={styles.providerCardInfo}>
            <span className={styles.providerName}>{provider.display_name}</span>
            <span className={styles.providerKey}>{provider.key}</span>
          </div>
        </div>
        <div className={styles.providerCardRight}>
          {/* Kind badge */}
          <span className={`${styles.badge} ${styles[`badge_${provider.kind}`]}`}>{provider.kind}</span>
          {/* Enabled badge */}
          <span className={`${styles.badge} ${provider.enabled ? styles.badgeEnabled : styles.badgeDisabled}`}>
            {provider.enabled ? 'enabled' : 'disabled'}
          </span>
          {/* Healthy indicator (live or from descriptor) */}
          {provider.healthy !== undefined && !healthData && (
            provider.healthy ? (
              <span className={styles.healthyDot} title="Healthy" />
            ) : (
              <span className={styles.unhealthyDot} title="Unhealthy" />
            )
          )}
          {healthData && (
            healthData.healthy ? (
              <span className={styles.healthyDot} title={`Healthy — ${healthData.latency_ms ?? 0}ms`} />
            ) : (
              <span className={styles.unhealthyDot} title={healthData.message ?? 'Unhealthy'} />
            )
          )}
          {/* Model count */}
          {((modelsData && modelsData.models) ? modelsData.models.length : provider.model_count) !== undefined && (
            <span className={styles.modelCountBadge}>
              {modelsData && modelsData.models ? modelsData.models.length : provider.model_count} models
            </span>
          )}
          {/* Edit credentials button */}
          <button
            className={styles.healthCheckBtn}
            onClick={() => { setEditing(e => !e); setSaveError(null); }}
            title={editing ? 'Cancel editing' : 'Edit credentials'}
          >
            {editing ? <Check size={12} style={{ color: 'var(--navi-accent)' }} /> : <Sliders size={12} />}
          </button>
          {/* Health check button */}
          <button
            className={styles.healthCheckBtn}
            onClick={() => setHealthCheck(h => !h)}
            title={healthCheck ? 'Stop polling health' : 'Check health'}
            disabled={healthLoading}
          >
            {healthLoading ? (
              <RefreshCw size={12} className={styles.spinIcon} />
            ) : healthCheck ? (
              <Wifi size={12} style={{ color: 'var(--navi-accent)' }} />
            ) : (
              <WifiOff size={12} />
            )}
          </button>
        </div>
      </div>

      {/* Inline credentials edit form */}
      {editing && (
        <div className={styles.providerEditForm}>
          {isOllama ? (
            <>
              <div className={styles.providerEditField}>
                <label className={styles.providerEditLabel}>Endpoint URL</label>
                <input
                  type="url"
                  className={styles.providerEditInput}
                  placeholder="http://localhost:11434/v1"
                  value={endpoint}
                  onChange={(e) => setEndpoint(e.target.value)}
                />
              </div>
              <div className={styles.providerEditField}>
                <label className={styles.providerEditLabel}>Model name</label>
                <input
                  type="text"
                  className={styles.providerEditInput}
                  placeholder="llama3:latest"
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                />
              </div>
            </>
          ) : (
            <>
              <div className={styles.providerEditField}>
                <label className={styles.providerEditLabel}>API key</label>
                <input
                  type="password"
                  className={styles.providerEditInput}
                  placeholder="Enter API key…"
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  autoComplete="off"
                />
              </div>
              <div className={styles.providerEditField}>
                <label className={styles.providerEditLabel}>Model (optional)</label>
                <input
                  type="text"
                  className={styles.providerEditInput}
                  placeholder="Leave blank for default"
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                />
              </div>
            </>
          )}
          {saveError && <div className={styles.providerEditError}>{saveError}</div>}
          <div className={styles.providerEditActions}>
            {provider.enabled && (
              <button
                className={styles.providerDisconnectBtn}
                onClick={handleDisconnect}
                type="button"
              >
                Disconnect
              </button>
            )}
            <button
              className={styles.healthCheckBtn}
              onClick={() => { setEditing(false); setSaveError(null); }}
              type="button"
            >
              Cancel
            </button>
            <button
              className={styles.providerSaveBtn}
              onClick={handleSaveCredentials}
              disabled={saving || (!apiKey && !endpoint && !model)}
              type="button"
            >
              {saving ? 'Saving…' : 'Save'}
            </button>
          </div>
        </div>
      )}

      {/* Expanded model list */}
      {expanded && (
        <div className={styles.providerModels}>
          {modelsLoading ? (
            <div className={styles.providerModelsLoading}>Loading models…</div>
          ) : models.length === 0 ? (
            <div className={styles.providerModelsEmpty}>No models found for this provider.</div>
          ) : (
            <div className={styles.providerModelList}>
              {models.map(m => (
                <div key={m.name} className={styles.providerModelItem}>
                  <span className={styles.providerModelName}>{m.name}</span>
                  <div className={styles.providerModelMeta}>
                    {m.family && <span className={styles.modelMeta}>{m.family}</span>}
                    {m.parameter_size && <span className={styles.modelMeta}>{m.parameter_size}</span>}
                    {m.quantization_level && <span className={styles.modelMeta}>{m.quantization_level}</span>}
                    {m.size && <span className={styles.modelMeta}>{(m.size / 1e9).toFixed(1)}GB</span>}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
