import { useState, type ComponentType, type FormEvent, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  FileText,
  Monitor,
  Puzzle,
  Settings2,
  ShieldCheck,
  Wrench,
  Zap,
} from 'lucide-react';
import type {
  CapabilityStatus,
  ConnectorNode,
  DocNode,
  SkillNode,
  ToolInterfaceNode,
  UISurfaceNode,
} from '@/api/capabilities';
import { setupConnector } from '@/api/extensions';
import { ConnectorSyncPanel } from '@/components/intake/ConnectorSyncPanel';
import { JsonPanel } from '@/components/JsonPanel';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { EntityManagerPanel } from '@/components/plugin-ui/EntityManagerPanel';
import type { PluginResources } from './resources';
import { statusVariant } from './status';
import styles from './PluginDetailPage.module.css';

interface IconProps {
  size?: number;
  className?: string;
}

export interface PluginSection {
  id: string;
  label: string;
  icon: ComponentType<IconProps>;
  // count returns how many items the section holds (0 ⇒ section is skipped,
  // unless `alwaysShow` is set).
  count: (r: PluginResources) => number;
  alwaysShow?: boolean;
  render: (r: PluginResources) => ReactNode;
}

function Row({
  title,
  subtitle,
  badges,
  children,
}: {
  title: string;
  subtitle?: string;
  badges?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className={styles.resourceRow}>
      <div className={styles.resourceHead}>
        <div>
          <div className={styles.resourceTitle}>{title}</div>
          {subtitle && <div className={styles.resourceSubtitle}>{subtitle}</div>}
        </div>
        {badges && <div className={styles.badgeRow}>{badges}</div>}
      </div>
      {children}
    </div>
  );
}

function statusBadges(node: { status?: CapabilityStatus }): ReactNode {
  const s = node.status;
  if (!s) return null;
  const keys = ['lifecycle', 'validation', 'availability'] as const;
  return keys
    .filter((k) => s[k] && s[k] !== 'unknown')
    .map((k) => <StatusBadge key={k} size="sm" label={s[k]} variant={statusVariant(s[k])} />);
}

const skillsSection: PluginSection = {
  id: 'skills',
  label: 'Skills',
  icon: Puzzle,
  count: (r) => r.skills.length,
  render: (r) => (
    <>
      {r.skills.map((skill: SkillNode) => (
        <Row
          key={skill.id}
          title={skill.displayName || skill.skillId}
          subtitle={skill.description}
          badges={statusBadges(skill)}
        >
          {skill.interfaces.length > 0 && (
            <div className={styles.chips}>
              {skill.interfaces.map((i) => (
                <span key={i} className={styles.chip}>
                  {i}
                </span>
              ))}
            </div>
          )}
          {(skill.statusReasons ?? []).length > 0 && (
            <ul className={styles.reasons}>
              {skill.statusReasons!.map((reason, i) => (
                <li key={i}>{reason}</li>
              ))}
            </ul>
          )}
        </Row>
      ))}
    </>
  ),
};

const toolInterfacesSection: PluginSection = {
  id: 'toolInterfaces',
  label: 'Tool Calls',
  icon: Wrench,
  count: (r) => r.toolInterfaces.length,
  render: (r) => (
    <>
      {r.toolInterfaces.map((tool: ToolInterfaceNode) => (
        <Row
          key={tool.id}
          title={tool.displayName || tool.canonicalInterfaceId}
          subtitle={tool.description}
          badges={statusBadges(tool)}
        >
          {tool.inputSchema && <JsonPanel data={tool.inputSchema} label="Input schema" />}
          {tool.outputSchema && <JsonPanel data={tool.outputSchema} label="Output schema" />}
        </Row>
      ))}
    </>
  ),
};

const connectorsSection: PluginSection = {
  id: 'connectors',
  label: 'Connectors',
  icon: Zap,
  count: (r) => r.connectors.length,
  render: (r) => (
    <>
      {r.connectors.map((c: ConnectorNode) => (
        <Row
          key={c.id}
          title={c.displayName || c.driverId || c.id}
          subtitle={c.category}
          badges={statusBadges(c)}
        >
          {/* CIP P5: per-connector sync state (policy, last pass, recent passes,
              editable policy, backfill request) — rides in the connector detail. */}
          <ConnectorSyncPanel connectorId={c.instanceId || c.driverId || c.id} />
          {c.setupSchema && <ConnectorSetupPanel connector={c} />}
        </Row>
      ))}
    </>
  ),
};

interface ConnectorSetupParam {
  key?: string;
  label?: string;
  description?: string;
  secret?: boolean;
  placeholder?: string;
  validation_hint?: string;
}

interface ConnectorSetupSchema {
  type?: string;
  display_name?: string;
  required_params?: ConnectorSetupParam[];
  optional_params?: ConnectorSetupParam[];
  setup_hint?: string;
}

function ConnectorSetupPanel({ connector }: { connector: ConnectorNode }) {
  const queryClient = useQueryClient();
  const schema = connector.setupSchema as ConnectorSetupSchema | undefined;
  const requiredParams = Array.isArray(schema?.required_params) ? schema.required_params : [];
  const optionalParams = Array.isArray(schema?.optional_params) ? schema.optional_params : [];
  const fields = [...requiredParams, ...optionalParams].filter((field) => field.key);
  const requiredKeys = new Set(requiredParams.map((field) => field.key).filter(Boolean));
  const connectorType = schema?.type || connector.driverId || connector.instanceId || '';
  const displayName = schema?.display_name || connector.displayName || connectorType || 'Connector';
  const [values, setValues] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  if (!connectorType || fields.length === 0) {
    return <JsonPanel data={connector.setupSchema} label="Setup schema" />;
  }

  const onSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    setMessage(null);
    setError(null);
    const params = Object.fromEntries(
      fields
        .map((field) => [field.key!, (values[field.key!] ?? '').trim()] as const)
        .filter(([key, value]) => requiredKeys.has(key) || value !== ''),
    );
    try {
      await setupConnector(connectorType, params);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['capabilities-graph'] }),
        queryClient.invalidateQueries({ queryKey: ['connectors'] }),
        queryClient.invalidateQueries({ queryKey: ['connector-instances'] }),
        queryClient.invalidateQueries({ queryKey: ['connector-health'] }),
      ]);
      setMessage(`${displayName} setup saved.`);
      setValues({});
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className={styles.connectorSetup}>
      {schema?.setup_hint && <p className={styles.setupHint}>{schema.setup_hint}</p>}
      <form className={styles.setupForm} onSubmit={onSubmit}>
        {fields.map((field) => {
          const key = field.key!;
          return (
            <label key={key} className={styles.setupField}>
              <span className={styles.setupLabel}>
                {field.label || key}
                {requiredKeys.has(key) && (
                  <span className={styles.requiredMark} aria-hidden="true">
                    *
                  </span>
                )}
              </span>
              <input
                aria-label={field.label || key}
                value={values[key] ?? ''}
                onChange={(event) => setValues((current) => ({ ...current, [key]: event.target.value }))}
                type={field.secret ? 'password' : 'text'}
                placeholder={field.placeholder}
                required={requiredKeys.has(key)}
                autoComplete={field.secret ? 'off' : undefined}
              />
              {(field.description || field.validation_hint) && (
                <span className={styles.setupHelp}>{field.description || field.validation_hint}</span>
              )}
            </label>
          );
        })}
        <div className={styles.setupActions}>
          <button className={styles.actionBtn} type="submit" disabled={submitting}>
            {submitting ? 'Configuring...' : `Configure ${displayName}`}
          </button>
        </div>
      </form>
      {message && <div className={styles.okBanner}>{message}</div>}
      {error && <div className={styles.errorBanner}>{error}</div>}
      <JsonPanel data={connector.setupSchema} label="Setup schema" />
    </div>
  );
}

const docsSection: PluginSection = {
  id: 'docs',
  label: 'Docs',
  icon: FileText,
  count: (r) => r.docs.length,
  render: (r) => (
    <>
      {r.docs.map((d: DocNode) => (
        <Row key={d.id} title={d.displayName || d.path || d.id} subtitle={d.path} />
      ))}
    </>
  ),
};

const configSection: PluginSection = {
  id: 'config',
  label: 'Configuration',
  icon: Settings2,
  count: (r) => (r.plugin.configSchema && Object.keys(r.plugin.configSchema).length > 0 ? 1 : 0),
  render: (r) => <JsonPanel data={r.plugin.configSchema} label="Config schema" defaultExpanded />,
};

const uiSurfacesSection: PluginSection = {
  id: 'uiSurfaces',
  label: 'Custom UI',
  icon: Monitor,
  count: (r) => r.uiSurfaces.length,
  render: (r) => (
    <>
      {r.uiSurfaces.map((surface: UISurfaceNode) => (
        <div key={surface.id} className={styles.uiSurface}>
          <div className={styles.resourceTitle}>{surface.title || surface.displayName}</div>
          {surface.description && <div className={styles.resourceSubtitle}>{surface.description}</div>}
          <EntityManagerPanel surface={surface} toolInterfaces={r.toolInterfaces} />
        </div>
      ))}
    </>
  ),
};

const statusSection: PluginSection = {
  id: 'status',
  label: 'Validation & Status',
  icon: ShieldCheck,
  alwaysShow: true,
  count: () => 1,
  render: (r) => {
    const s = (r.plugin.status ?? {}) as Record<string, unknown>;
    const entries = Object.entries(s).filter(
      ([, v]) => typeof v === 'string' && v && v !== 'none',
    ) as [string, string][];
    return (
      <div className={styles.statusGrid}>
        {entries.map(([k, v]) => (
          <div key={k} className={styles.statusCell}>
            <span className={styles.statusKey}>{k}</span>
            <StatusBadge size="sm" label={v} variant={statusVariant(v)} />
          </div>
        ))}
        {(r.plugin.statusReasons ?? []).length > 0 && (
          <ul className={styles.reasons}>
            {r.plugin.statusReasons!.map((reason, i) => (
              <li key={i}>{reason}</li>
            ))}
          </ul>
        )}
      </div>
    );
  },
};

// pluginSectionRegistry is the ordered, extensible list of detail sections.
// Add a new resource type by appending an entry here — no other file changes.
export const pluginSectionRegistry: PluginSection[] = [
  uiSurfacesSection,
  skillsSection,
  toolInterfacesSection,
  connectorsSection,
  configSection,
  docsSection,
  statusSection,
];
