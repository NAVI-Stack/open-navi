import { useEffect, useRef, useState } from 'react';
import clsx from 'clsx';
import { ChevronDown, ChevronRight, Cloud, HardDrive, X } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { useLlmProviders, configureLlmProvider, disableLlmProvider } from '@/api/config';
import { useNavigate } from '@/app/router';
import type { LlmProviderDescriptor } from '@/types/api';
import styles from './AddModelModal.module.css';

// ─── Provider edit form ───────────────────────────────────────────────────────

interface ProviderEditFormProps {
  provider: LlmProviderDescriptor;
  onSave: () => void;
  onCancel: () => void;
}

function ProviderEditForm({ provider, onSave, onCancel }: ProviderEditFormProps) {
  const [apiKey, setApiKey] = useState('');
  const [model, setModel] = useState('');
  const [endpoint, setEndpoint] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isOllama = provider.key === 'ollama';

  async function handleSave() {
    setSaving(true);
    setError(null);
    try {
      await configureLlmProvider(provider.key, {
        ...(apiKey ? { api_key: apiKey } : {}),
        ...(model ? { model } : {}),
        ...(endpoint ? { endpoint } : {}),
      });
      onSave();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  }

  const canSave = !saving && (!!apiKey || !!endpoint || !!model);

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && canSave) {
      e.preventDefault();
      void handleSave();
    }
  }

  return (
    <div className={styles.editForm}>
      {isOllama ? (
        <>
          <div className={styles.fieldGroup}>
            <label className={styles.fieldLabel}>Endpoint URL</label>
            <input
              type="url"
              className={styles.fieldInput}
              placeholder="http://localhost:11434/v1"
              value={endpoint}
              onChange={(e) => setEndpoint(e.target.value)}
              onKeyDown={handleKeyDown}
              autoFocus
            />
          </div>
          <div className={styles.fieldGroup}>
            <label className={styles.fieldLabel}>Model name</label>
            <input
              type="text"
              className={styles.fieldInput}
              placeholder="llama3:latest"
              value={model}
              onChange={(e) => setModel(e.target.value)}
              onKeyDown={handleKeyDown}
            />
          </div>
        </>
      ) : (
        <>
          <div className={styles.fieldGroup}>
            <label className={styles.fieldLabel}>API key</label>
            <input
              type="password"
              className={styles.fieldInput}
              placeholder="Enter API key…"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              onKeyDown={handleKeyDown}
              autoFocus
              autoComplete="off"
            />
          </div>
          <div className={styles.fieldGroup}>
            <label className={styles.fieldLabel}>Model (optional)</label>
            <input
              type="text"
              className={styles.fieldInput}
              placeholder="Leave blank for default"
              value={model}
              onChange={(e) => setModel(e.target.value)}
              onKeyDown={handleKeyDown}
            />
          </div>
        </>
      )}
      {error && <div className={styles.editError}>{error}</div>}
      <div className={styles.editActions}>
        <button type="button" className={styles.editCancelBtn} onClick={onCancel} disabled={saving}>
          Cancel
        </button>
        <button
          type="button"
          className={styles.editSaveBtn}
          onClick={() => void handleSave()}
          disabled={!canSave}
        >
          {saving ? 'Saving…' : 'Save'}
        </button>
      </div>
    </div>
  );
}

// ─── Provider row ─────────────────────────────────────────────────────────────

interface ProviderRowProps {
  provider: LlmProviderDescriptor;
  editingKey: string | null;
  isDisconnecting: boolean;
  onEdit: (key: string) => void;
  onCancelEdit: () => void;
  onSaved: () => void;
  onDisconnect: (key: string) => void;
}

function ProviderRow({ provider, editingKey, isDisconnecting, onEdit, onCancelEdit, onSaved, onDisconnect }: ProviderRowProps) {
  const isEditing = editingKey === provider.key;
  const configured = provider.configured ?? provider.enabled;
  const isOllama = provider.key === 'ollama';

  const dotClass =
    !configured ? styles.dotUnk :
    provider.healthy === true ? styles.dotOk :
    provider.healthy === false ? styles.dotBad :
    styles.dotUnk;

  const Icon = isOllama ? HardDrive : Cloud;

  return (
    <div className={clsx(styles.providerRow, isEditing && styles.providerRowEditing)}>
      <div className={styles.providerRowMain}>
        <span className={clsx(styles.statusDot, dotClass)} />
        <Icon size={14} className={styles.providerIcon} aria-hidden />
        <div className={styles.providerInfo}>
          <span className={styles.providerName}>{provider.display_name}</span>
          {!configured && (
            <span className={styles.notConfiguredLabel}>Not configured</span>
          )}
        </div>
        <div className={styles.providerActions}>
          {configured && (
            <button
              type="button"
              className={styles.disconnectBtn}
              onClick={() => onDisconnect(provider.key)}
              disabled={isDisconnecting}
              title="Disconnect provider"
            >
              {isDisconnecting ? 'Disconnecting…' : 'Disconnect'}
            </button>
          )}
          <button
            type="button"
            className={clsx(styles.editBtn, isEditing && styles.editBtnActive)}
            onClick={() => (isEditing ? onCancelEdit() : onEdit(provider.key))}
            aria-expanded={isEditing}
          >
            {isEditing ? (
              <><ChevronDown size={12} /> Cancel</>
            ) : (
              <><ChevronRight size={12} /> {configured ? 'Edit' : 'Configure'}</>
            )}
          </button>
        </div>
      </div>
      {isEditing && (
        <ProviderEditForm
          provider={provider}
          onSave={onSaved}
          onCancel={onCancelEdit}
        />
      )}
    </div>
  );
}

// ─── Known provider order ─────────────────────────────────────────────────────

const KNOWN_PROVIDER_ORDER = ['anthropic', 'openai', 'openrouter', 'ollama'];

function mergeKnownProviders(apiProviders: LlmProviderDescriptor[]): LlmProviderDescriptor[] {
  const byKey = new Map(apiProviders.map((p) => [p.key, p]));
  return KNOWN_PROVIDER_ORDER.map((key) =>
    byKey.get(key) ?? {
      key,
      display_name: key.charAt(0).toUpperCase() + key.slice(1),
      kind: (key === 'ollama' ? 'local' : key === 'openrouter' ? 'proxy' : 'cloud') as LlmProviderDescriptor['kind'],
      enabled: false,
      configured: false,
    },
  );
}

// ─── AddModelModal ────────────────────────────────────────────────────────────

interface AddModelModalProps {
  onClose: () => void;
}

export function AddModelModal({ onClose }: AddModelModalProps) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { data: providersData } = useLlmProviders();
  const providers = mergeKnownProviders(providersData?.providers ?? []);

  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [disconnecting, setDisconnecting] = useState<string | null>(null);
  const dialogRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const previousFocus = document.activeElement as HTMLElement | null;
    dialogRef.current?.focus();

    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        if (editingKey) {
          setEditingKey(null);
        } else {
          onClose();
        }
      }
    }
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('keydown', onKey);
      previousFocus?.focus();
    };
  }, [onClose, editingKey]);

  function handleOverlayClick(e: React.MouseEvent) {
    if (e.target === e.currentTarget) onClose();
  }

  function handleOpenSettings() {
    onClose();
    navigate('/settings');
  }

  function invalidateProviderData() {
    void queryClient.invalidateQueries({ queryKey: ['llm', 'providers'] });
    void queryClient.invalidateQueries({ queryKey: ['llm', 'profiles'] });
    void queryClient.invalidateQueries({ queryKey: ['llm', 'active'] });
  }

  function handleSaved() {
    setEditingKey(null);
    invalidateProviderData();
  }

  async function handleDisconnect(key: string) {
    setDisconnecting(key);
    try {
      await disableLlmProvider(key);
      invalidateProviderData();
    } catch {
      // Silently ignore — provider list will revert on next fetch
    } finally {
      setDisconnecting(null);
    }
  }

  return (
    <div
      className={styles.overlay}
      onClick={handleOverlayClick}
      role="presentation"
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label="Provider configuration"
        className={styles.dialog}
        tabIndex={-1}
      >
        {/* Header */}
        <div className={styles.header}>
          <div className={styles.headerText}>
            <h2 className={styles.title}>Provider configuration</h2>
            <p className={styles.subtitle}>
              Configure LLM providers — changes take effect immediately.
            </p>
          </div>
          <button
            type="button"
            className={styles.closeBtn}
            onClick={onClose}
            aria-label="Close"
          >
            <X size={14} />
          </button>
        </div>

        {/* Body */}
        <div className={styles.body}>
          <div className={styles.sectionTitle}>Providers</div>
          <div className={styles.providerList}>
            {providers.map((p) => (
              <ProviderRow
                key={p.key}
                provider={p}
                editingKey={editingKey}
                isDisconnecting={disconnecting === p.key}
                onEdit={setEditingKey}
                onCancelEdit={() => setEditingKey(null)}
                onSaved={handleSaved}
                onDisconnect={handleDisconnect}
              />
            ))}
          </div>
        </div>

        {/* Footer */}
        <div className={styles.footer}>
          <button type="button" className={styles.cancelBtn} onClick={onClose}>
            Close
          </button>
          <button
            type="button"
            className={styles.settingsBtn}
            onClick={handleOpenSettings}
          >
            Open LLM settings
          </button>
        </div>
      </div>
    </div>
  );
}
