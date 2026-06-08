import { useCallback, useEffect, useMemo, useState } from 'react';
import { Plus, RefreshCw, Search, Trash2, X } from 'lucide-react';
import { invokeSkillInterface } from '@/api/extensions';
import type { ToolInterfaceNode, UISurfaceNode } from '@/api/capabilities';
import { JsonPanel } from '@/components/JsonPanel';
import { EmptyState } from '@/components/ui/EmptyState';
import { useToast } from '@/components/ui/Toast';
import styles from './EntityManagerPanel.module.css';

// EntityManagerSchema mirrors the `schema` object of an entity_manager UI surface.
interface EntityManagerSchema {
  type?: string;
  entity?: string;
  list?: { action: string; result_path?: string; primary_field?: string; secondary_fields?: string[] };
  detail?: { action: string; id_field?: string };
  search?: { action: string; query_field?: string };
  forms?: {
    create?: { action: string };
    update?: { action: string; id_field?: string };
  };
  import_export?: { import_action?: string; export_action?: string };
}

interface ActionBinding {
  id?: string;
  label?: string;
  interface?: string;
  confirm?: boolean;
}

interface Props {
  surface: UISurfaceNode;
  toolInterfaces: ToolInterfaceNode[];
}

function findActionInterface(bindings: ActionBinding[], id: string): { iface: string; confirm: boolean } | null {
  const b = bindings.find((a) => a.id === id && a.interface);
  return b ? { iface: b.interface!, confirm: !!b.confirm } : null;
}

type Row = Record<string, unknown>;

function asArray(value: unknown): Row[] {
  if (Array.isArray(value)) return value as Row[];
  return [];
}

function readResultRows(payload: unknown, resultPath?: string): Row[] {
  if (payload == null) return [];
  if (resultPath && typeof payload === 'object') {
    const v = (payload as Record<string, unknown>)[resultPath];
    if (v !== undefined) return asArray(v);
  }
  return asArray(payload);
}

// inputSchemaFor finds the JSON schema for a given interface name from the
// plugin's tool interfaces, used to drive create/update forms.
function inputSchemaFor(toolInterfaces: ToolInterfaceNode[], ifaceName: string): Record<string, unknown> | null {
  for (const t of toolInterfaces) {
    if (t.interfaceName === ifaceName && t.inputSchema) return t.inputSchema as Record<string, unknown>;
  }
  return null;
}

export function EntityManagerPanel({ surface, toolInterfaces }: Props) {
  const schema = (surface.schema ?? {}) as EntityManagerSchema;
  const skillId = surface.ownerId || '';

  if (schema.type !== 'entity_manager') {
    return (
      <div className={styles.fallback}>
        <p className={styles.fallbackMsg}>Custom UI type "{schema.type ?? 'unknown'}" is not supported yet.</p>
        <JsonPanel data={surface.schema} label="UI surface schema" />
      </div>
    );
  }

  return (
    <EntityManager
      schema={schema}
      skillId={skillId}
      toolInterfaces={toolInterfaces}
      entityLabel={schema.entity ?? 'item'}
      bindings={(surface.actionBindings ?? []) as ActionBinding[]}
    />
  );
}

function EntityManager({
  schema,
  skillId,
  toolInterfaces,
  entityLabel,
  bindings,
}: {
  schema: EntityManagerSchema;
  skillId: string;
  toolInterfaces: ToolInterfaceNode[];
  entityLabel: string;
  bindings: ActionBinding[];
}) {
  const toast = useToast();
  const [rows, setRows] = useState<Row[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [creating, setCreating] = useState(false);

  const primaryField = schema.list?.primary_field ?? 'name';
  const secondaryFields = schema.list?.secondary_fields ?? [];
  const idField = schema.detail?.id_field ?? schema.forms?.update?.id_field ?? 'id';

  const runList = useCallback(async () => {
    if (!schema.list?.action || !skillId) return;
    setLoading(true);
    setError(null);
    try {
      const res = await invokeSkillInterface(skillId, schema.list.action, {});
      setRows(readResultRows(res.payload, schema.list.result_path));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [schema.list?.action, schema.list?.result_path, skillId]);

  const runSearch = useCallback(async () => {
    if (!query.trim()) {
      runList();
      return;
    }
    if (!schema.search?.action || !skillId) return;
    setLoading(true);
    setError(null);
    try {
      const field = schema.search.query_field ?? 'query';
      const res = await invokeSkillInterface(skillId, schema.search.action, { [field]: query.trim() });
      // search results may use a different container key; try common ones.
      const payload = res.payload as Record<string, unknown> | undefined;
      const rowsOut = payload?.results
        ? asArray(payload.results)
        : readResultRows(res.payload, schema.list?.result_path);
      setRows(rowsOut);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [query, schema.search?.action, schema.search?.query_field, schema.list?.result_path, skillId, runList]);

  useEffect(() => {
    runList();
  }, [runList]);

  const deleteAction = useMemo(() => findActionInterface(bindings, 'delete'), [bindings]);

  const handleDelete = useCallback(
    async (row: Row) => {
      if (!deleteAction) return;
      const id = row[idField];
      if (deleteAction.confirm && !window.confirm(`Delete this ${entityLabel}? This cannot be undone.`)) return;
      try {
        await invokeSkillInterface(skillId, deleteAction.iface, { [idField]: id });
        runList();
      } catch (err) {
        toast.error(`Delete failed: ${err instanceof Error ? err.message : String(err)}`);
      }
    },
    [deleteAction, entityLabel, idField, skillId, runList, toast],
  );

  const createSchema = useMemo(
    () => (schema.forms?.create ? inputSchemaFor(toolInterfaces, schema.forms.create.action) : null),
    [schema.forms?.create, toolInterfaces],
  );

  return (
    <div className={styles.manager}>
      <div className={styles.toolbar}>
        {schema.search?.action && (
          <div className={styles.searchBox}>
            <Search size={14} />
            <input
              value={query}
              placeholder={`Search ${entityLabel}s…`}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') runSearch();
              }}
            />
          </div>
        )}
        <div className={styles.toolbarActions}>
          <button className={styles.btn} onClick={runList} disabled={loading} type="button">
            <RefreshCw size={14} className={loading ? styles.spin : undefined} />
            Refresh
          </button>
          {schema.forms?.create && createSchema && (
            <button className={`${styles.btn} ${styles.btnPrimary}`} onClick={() => setCreating(true)} type="button">
              <Plus size={14} />
              New {entityLabel}
            </button>
          )}
        </div>
      </div>

      {error && <div className={styles.error}>{error}</div>}

      {loading && rows.length === 0 ? (
        <div className={styles.loading}>Loading {entityLabel}s…</div>
      ) : rows.length === 0 ? (
        <EmptyState title={`No ${entityLabel}s`} description={`This plugin has no ${entityLabel}s yet.`} />
      ) : (
        <ul className={styles.list}>
          {rows.map((row, i) => (
            <li key={String(row[idField] ?? i)} className={styles.row}>
              <div className={styles.rowMain}>
                <span className={styles.rowPrimary}>{String(row[primaryField] ?? row[idField] ?? '—')}</span>
                {secondaryFields.length > 0 && (
                  <span className={styles.rowSecondary}>
                    {secondaryFields
                      .map((f) => row[f])
                      .filter((v) => v !== undefined && v !== null && v !== '')
                      .map(String)
                      .join(' · ')}
                  </span>
                )}
              </div>
              {deleteAction && (
                <button
                  className={styles.iconBtn}
                  title={`Delete ${entityLabel}`}
                  onClick={() => handleDelete(row)}
                  type="button"
                >
                  <Trash2 size={14} />
                </button>
              )}
            </li>
          ))}
        </ul>
      )}

      {creating && schema.forms?.create && createSchema && (
        <CreateForm
          entityLabel={entityLabel}
          inputSchema={createSchema}
          onCancel={() => setCreating(false)}
          onSubmit={async (values) => {
            try {
              await invokeSkillInterface(skillId, schema.forms!.create!.action, values);
              setCreating(false);
              runList();
            } catch (err) {
              toast.error(`Create failed: ${err instanceof Error ? err.message : String(err)}`);
            }
          }}
        />
      )}
    </div>
  );
}

interface SchemaProperty {
  type?: string;
  description?: string;
  enum?: string[];
}

// CreateForm renders a minimal form from a JSON-schema object's top-level
// string/enum/number properties. Complex array/object fields are skipped
// (the underlying skill still accepts them via the API).
function CreateForm({
  entityLabel,
  inputSchema,
  onCancel,
  onSubmit,
}: {
  entityLabel: string;
  inputSchema: Record<string, unknown>;
  onCancel: () => void;
  onSubmit: (values: Record<string, unknown>) => void;
}) {
  const properties = (inputSchema.properties ?? {}) as Record<string, SchemaProperty>;
  const required = (inputSchema.required ?? []) as string[];
  const fields = Object.entries(properties).filter(([, p]) =>
    ['string', 'integer', 'number', 'boolean'].includes(p.type ?? 'string'),
  );
  const [values, setValues] = useState<Record<string, unknown>>({});

  return (
    <div className={styles.modalBackdrop} onClick={onCancel}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.modalHeader}>
          <h4>New {entityLabel}</h4>
          <button className={styles.iconBtn} onClick={onCancel} type="button" aria-label="Close">
            <X size={16} />
          </button>
        </div>
        <form
          className={styles.form}
          onSubmit={(e) => {
            e.preventDefault();
            const cleaned: Record<string, unknown> = {};
            for (const [k, v] of Object.entries(values)) {
              if (v !== '' && v !== undefined) cleaned[k] = v;
            }
            onSubmit(cleaned);
          }}
        >
          {fields.map(([name, prop]) => (
            <label key={name} className={styles.field}>
              <span className={styles.fieldLabel}>
                {name}
                {required.includes(name) && <span className={styles.req}> *</span>}
              </span>
              {prop.enum ? (
                <select
                  value={String(values[name] ?? '')}
                  onChange={(e) => setValues((v) => ({ ...v, [name]: e.target.value }))}
                >
                  <option value="">—</option>
                  {prop.enum.map((opt) => (
                    <option key={opt} value={opt}>
                      {opt}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  type={prop.type === 'integer' || prop.type === 'number' ? 'number' : 'text'}
                  required={required.includes(name)}
                  value={String(values[name] ?? '')}
                  placeholder={prop.description}
                  onChange={(e) =>
                    setValues((v) => ({
                      ...v,
                      [name]:
                        prop.type === 'integer' || prop.type === 'number'
                          ? e.target.value === ''
                            ? ''
                            : Number(e.target.value)
                          : e.target.value,
                    }))
                  }
                />
              )}
            </label>
          ))}
          <div className={styles.formActions}>
            <button className={styles.btn} type="button" onClick={onCancel}>
              Cancel
            </button>
            <button className={`${styles.btn} ${styles.btnPrimary}`} type="submit">
              Create
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
