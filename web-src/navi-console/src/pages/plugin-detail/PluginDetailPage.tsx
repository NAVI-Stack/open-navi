import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  AlertTriangle,
  ChevronRight,
  Power,
  PowerOff,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react';
import { useCapabilitiesGraph } from '@/api/capabilities';
import {
  disablePlugin,
  enablePlugin,
  reloadPlugin,
  validatePlugin,
  type PluginValidation,
} from '@/api/extensions';
import { useNavigate } from '@/app/router';
import { EmptyState } from '@/components/ui/EmptyState';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { collectPluginResources, findPluginNode } from './resources';
import { pluginSectionRegistry } from './sectionRegistry';
import { statusVariant } from './status';
import styles from './PluginDetailPage.module.css';

interface Props {
  pluginId: string;
}

export function PluginDetailPage({ pluginId }: Props) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const graphQuery = useCapabilitiesGraph();
  const [busy, setBusy] = useState<string | null>(null);
  const [validation, setValidation] = useState<PluginValidation | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const back = () => navigate('/plugins');

  if (graphQuery.isLoading) {
    return <div className={styles.loading}>Loading plugin…</div>;
  }

  if (graphQuery.isError || !graphQuery.data) {
    return (
      <div className={styles.container}>
        <Breadcrumb onBack={back} name={pluginId} />
        <EmptyState
          icon={<AlertTriangle size={24} />}
          title="Could not load capabilities"
          description={graphQuery.error instanceof Error ? graphQuery.error.message : 'The capability graph is unavailable.'}
          action={{ label: 'Retry', onPress: () => graphQuery.refetch() }}
        />
      </div>
    );
  }

  const node = findPluginNode(graphQuery.data, pluginId);
  if (!node) {
    return (
      <div className={styles.container}>
        <Breadcrumb onBack={back} name={pluginId} />
        <EmptyState
          icon={<AlertTriangle size={24} />}
          title="Plugin not found"
          description={`No plugin matching "${pluginId}" is registered.`}
          action={{ label: 'Back to Extensions', onPress: back }}
        />
      </div>
    );
  }

  const resources = collectPluginResources(graphQuery.data, node);
  const rawId = node.pluginId || pluginId;
  const enabled = (node.status?.lifecycle ?? '') !== 'disabled';

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['capabilities-graph'] });

  const runAction = async (key: string, fn: () => Promise<unknown>) => {
    setBusy(key);
    setActionError(null);
    try {
      await fn();
      refresh();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  const handleValidate = async () => {
    setBusy('validate');
    setActionError(null);
    try {
      const result = await validatePlugin(rawId);
      setValidation(result);
      refresh();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  const sections = pluginSectionRegistry.filter((s) => s.alwaysShow || s.count(resources) > 0);
  const hasResources =
    resources.skills.length +
      resources.toolInterfaces.length +
      resources.connectors.length +
      resources.uiSurfaces.length +
      resources.docs.length >
    0;

  return (
    <div className={styles.container}>
      <Breadcrumb onBack={back} name={node.displayName || rawId} />

      <header className={styles.header}>
        <div className={styles.titleRow}>
          <h1 className={styles.title}>{node.displayName || rawId}</h1>
          <div className={styles.badgeRow}>
            <StatusBadge label={enabled ? 'enabled' : 'disabled'} variant={enabled ? 'success' : 'danger'} />
            {node.status?.validation && (
              <StatusBadge label={node.status.validation} variant={statusVariant(node.status.validation)} />
            )}
            {node.trustTier && <StatusBadge label={node.trustTier} variant="muted" />}
          </div>
        </div>
        {node.description && <p className={styles.description}>{node.description}</p>}
        <div className={styles.metaRow}>
          {node.version && <span className={styles.meta}>v{node.version}</span>}
          {node.kind && <span className={styles.meta}>{node.kind}</span>}
          {node.rawRef?.rootDir && <span className={styles.metaPath}>{node.rawRef.rootDir}</span>}
        </div>

        <div className={styles.actions}>
          <button
            className={styles.actionBtn}
            onClick={() => runAction('disable', () => disablePlugin(rawId))}
            disabled={!enabled || busy !== null}
            type="button"
          >
            <PowerOff size={14} /> Disable
          </button>
          <button
            className={styles.actionBtn}
            onClick={() => runAction('enable', () => enablePlugin(rawId))}
            disabled={enabled || busy !== null}
            type="button"
          >
            <Power size={14} /> Enable
          </button>
          <button className={styles.actionBtn} onClick={handleValidate} disabled={busy !== null} type="button">
            <ShieldCheck size={14} className={busy === 'validate' ? styles.spin : undefined} /> Validate
          </button>
          <button
            className={styles.actionBtn}
            onClick={() => runAction('reload', () => reloadPlugin(rawId))}
            disabled={busy !== null}
            type="button"
          >
            <RefreshCw size={14} className={busy === 'reload' ? styles.spin : undefined} /> Reload
          </button>
        </div>

        {actionError && <div className={styles.errorBanner}>{actionError}</div>}
        {validation && (
          <div className={validation.valid ? styles.okBanner : styles.errorBanner}>
            {validation.valid ? (
              'Plugin manifest is valid.'
            ) : (
              <>
                <strong>Validation failed:</strong>
                <ul className={styles.reasons}>
                  {validation.reasons.map((reason, i) => (
                    <li key={i}>{reason}</li>
                  ))}
                </ul>
              </>
            )}
          </div>
        )}
      </header>

      {!hasResources && (
        <EmptyState
          title="No inspectable resources"
          description="This plugin does not expose skills, tools, connectors, docs, or custom UI."
        />
      )}

      <div className={styles.sections}>
        {sections.map((section) => {
          const Icon = section.icon;
          const n = section.count(resources);
          return (
            <section key={section.id} className={styles.section}>
              <h2 className={styles.sectionTitle}>
                <Icon size={16} className={styles.sectionIcon} />
                {section.label}
                {!section.alwaysShow && <span className={styles.sectionCount}>{n}</span>}
              </h2>
              <div className={styles.sectionBody}>{section.render(resources)}</div>
            </section>
          );
        })}
      </div>
    </div>
  );
}

function Breadcrumb({ onBack, name }: { onBack: () => void; name: string }) {
  return (
    <nav className={styles.breadcrumb} aria-label="Breadcrumb">
      <button className={styles.crumbLink} onClick={onBack} type="button">
        Extensions
      </button>
      <ChevronRight size={14} className={styles.crumbSep} />
      <span className={styles.crumbCurrent}>{name}</span>
    </nav>
  );
}
