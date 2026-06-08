import { useEffect, useMemo, useState } from 'react';
import {
  Archive,
  Boxes,
  ChevronLeft,
  CheckCircle2,
  Folder,
  FolderOpen,
  Plus,
  RotateCcw,
  Server,
  ShieldCheck,
  ShieldOff,
} from 'lucide-react';
import { EmptyState } from '@/components/ui/EmptyState';
import { JsonPanel } from '@/components/JsonPanel';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { useToast } from '@/components/ui/Toast';
import { useConfirm } from '@/components/ui/ConfirmDialog';
import {
  activeRules,
  defaultAllowedActions,
  useActiveWorkspace,
  useArchiveWorkspace,
  useCreateWhitelistRule,
  useCreateWorkspace,
  useDeleteWorkspace,
  useRevokeWhitelistRule,
  useSetActiveWorkspace,
  useSetWorkspaceMode,
  useUpdateWorkspace,
  useWorkspacePathChildren,
  useWorkspacePathRoots,
  useWhitelistRules,
  useWorkspaces,
  useWorkspaceMode,
} from '@/api/workspaces';
import { useProjects } from '@/api/projects';
import type { AllowedActions, Project, WhitelistRule, Workspace, WorkspacePathEntry } from '@/types/api';
import styles from './Workspaces.module.css';

type StatusVariant = 'success' | 'warning' | 'danger' | 'accent' | 'muted';
type ActionKey = keyof AllowedActions;

const actionKeys: ActionKey[] = ['read', 'write', 'create', 'modify', 'rename_move', 'delete', 'execute'];

declare global {
  interface Window {
    showDirectoryPicker?: () => Promise<{ name?: string }>;
  }
}

export function Workspaces() {
  const workspacesQuery = useWorkspaces();
  const projectsQuery = useProjects();
  const modeQuery = useWorkspaceMode();
  const activeQuery = useActiveWorkspace();
  const setMode = useSetWorkspaceMode();
  const setActiveWorkspace = useSetActiveWorkspace();
  const createWorkspace = useCreateWorkspace();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);

  const workspaces = safeArray(workspacesQuery.data?.items);
  const projects = safeArray(projectsQuery.data?.items);
  const activeWorkspaceId = activeQuery.data?.active_workspace_id ?? workspacesQuery.data?.active_workspace_id ?? '';
  const selectedWorkspace = useMemo(() => {
    if (selectedId) return workspaces.find((workspace) => workspace.workspace_id === selectedId) ?? null;
    if (activeWorkspaceId) return workspaces.find((workspace) => workspace.workspace_id === activeWorkspaceId) ?? null;
    return workspaces[0] ?? null;
  }, [activeWorkspaceId, selectedId, workspaces]);
  const mode = modeQuery.data?.mode ?? workspacesQuery.data?.mode ?? 'unknown';

  const handleCreateWorkspace = async () => {
    const workspace = await createWorkspace.mutateAsync({
      name: 'New Workspace',
      description: '',
      workspace_kind: 'general',
      local_roots: [],
      repo_roots: [],
      protected_paths: [],
      allowed_actions: defaultAllowedActions,
      boundary_policy: { out_of_scope_default: 'prompt' },
      audit_enabled: true,
    });
    setSelectedId(workspace.workspace_id);
    setShowCreate(false);
  };

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div>
          <h2 className={styles.heading}>Workspaces</h2>
          <p className={styles.subtle}>Execution boundaries, active scope, protected paths, and durable allow rules.</p>
        </div>
        <button className={styles.primaryButton} onClick={() => setShowCreate((value) => !value)} type="button">
          <Plus size={14} />
          {showCreate ? 'Close' : 'New Workspace'}
        </button>
      </div>

      <div className={styles.modeBar}>
        <div className={styles.modeGroup} role="group" aria-label="Workspace operating mode">
          {(['global', 'scoped', 'hybrid'] as const).map((item) => (
            <button
              key={item}
              className={`${styles.modeButton} ${mode === item ? styles.active : ''}`}
              onClick={() => setMode.mutate(item)}
              disabled={setMode.isPending}
              type="button"
            >
              {item}
            </button>
          ))}
        </div>
        <div className={styles.activeSummary}>
          <StatusBadge label={`mode: ${mode}`} variant={modeVariant(mode)} />
          <StatusBadge label={`active: ${activeWorkspaceLabel(activeQuery.data?.workspace, activeWorkspaceId)}`} variant={activeWorkspaceId ? 'success' : 'warning'} />
          <StatusBadge label={`selection: ${activeQuery.data?.active_workspace_selection ?? 'none'}`} variant="muted" />
        </div>
      </div>

      {showCreate && (
        <div className={styles.createPanel}>
          <span>Use the generated draft, then edit its boundary and controls in the detail pane.</span>
          <button className={styles.primaryButton} onClick={handleCreateWorkspace} disabled={createWorkspace.isPending} type="button">
            <CheckCircle2 size={14} />
            {createWorkspace.isPending ? 'Creating...' : 'Create Draft'}
          </button>
        </div>
      )}

      <div className={styles.layout}>
        <div className={styles.listPane}>
          {workspacesQuery.isLoading && <div className={styles.loadingRow}>Loading workspaces...</div>}
          {workspacesQuery.error && <div className={styles.errorRow}>{errorMessage(workspacesQuery.error)}</div>}
          {!workspacesQuery.isLoading && !workspacesQuery.error && workspaces.length === 0 && (
            <div className={styles.emptyStateWrap}>
              <EmptyState icon={<ShieldOff size={24} />} title="No active workspace boundaries." />
            </div>
          )}
          {workspaces.map((workspace) => (
            <WorkspaceListItem
              key={workspace.workspace_id}
              workspace={workspace}
              project={projectForWorkspace(workspace, projects)}
              active={workspace.workspace_id === (selectedWorkspace?.workspace_id ?? '')}
              governing={workspace.workspace_id === activeWorkspaceId}
              onSelect={() => setSelectedId(workspace.workspace_id)}
              onActivate={() => setActiveWorkspace.mutate(workspace.workspace_id)}
            />
          ))}
        </div>

        <div className={styles.detailPane}>
          {selectedWorkspace ? (
            <WorkspaceDetail workspace={selectedWorkspace} projects={projects} />
          ) : (
            <div className={styles.emptyStateWrap}>
              <EmptyState icon={<Boxes size={32} />} title="Select a workspace" />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function WorkspaceListItem({
  workspace,
  project,
  active,
  governing,
  onSelect,
  onActivate,
}: {
  workspace: Workspace;
  project?: Project;
  active: boolean;
  governing: boolean;
  onSelect: () => void;
  onActivate: () => void;
}) {
  return (
    <button className={`${styles.workspaceItem} ${active ? styles.selected : ''}`} onClick={onSelect} type="button">
      <div className={styles.itemHeader}>
        <span className={styles.itemTitle}>{workspace.name}</span>
        {governing && <StatusBadge label="active" variant="success" />}
      </div>
      <div className={styles.itemSubtitle}>{workspace.workspace_id}</div>
      <div className={styles.itemMeta}>
        <StatusBadge label={workspace.status ?? 'unknown'} variant={statusVariant(workspace.status)} />
        <StatusBadge label={workspace.workspace_kind ?? 'general'} variant="accent" />
      </div>
      <div className={styles.itemSubtitle}>{project ? `project: ${project.title}` : 'no project binding'}</div>
      {!governing && (
        <span
          className={styles.activateLink}
          onClick={(event) => {
            event.stopPropagation();
            onActivate();
          }}
        >
          Make active
        </span>
      )}
    </button>
  );
}

function WorkspaceDetail({ workspace, projects }: { workspace: Workspace; projects: Project[] }) {
  const toast = useToast();
  const confirmDialog = useConfirm();
  const updateWorkspace = useUpdateWorkspace();
  const archiveWorkspace = useArchiveWorkspace();
  const deleteWorkspace = useDeleteWorkspace();
  const setActiveWorkspace = useSetActiveWorkspace();
  const whitelistQuery = useWhitelistRules(workspace.workspace_id);
  const createWhitelistRule = useCreateWhitelistRule();
  const revokeWhitelistRule = useRevokeWhitelistRule();
  const [draft, setDraft] = useState(() => workspaceDraft(workspace));
  const [newRuleScope, setNewRuleScope] = useState('');
  const [pathDraft, setPathDraft] = useState('');
  const [pathPickerNote, setPathPickerNote] = useState('');
  const [serverBrowserOpen, setServerBrowserOpen] = useState(false);
  const [serverBrowserPath, setServerBrowserPath] = useState('');
  const project = projectForWorkspace(workspace, projects);
  const rules = safeArray(whitelistQuery.data?.items ?? workspace.whitelist_rules);
  const pathRootsQuery = useWorkspacePathRoots(serverBrowserOpen);
  const pathChildrenQuery = useWorkspacePathChildren(serverBrowserPath, serverBrowserOpen && Boolean(serverBrowserPath));
  const serverBrowserItems = serverBrowserPath
    ? safeArray(pathChildrenQuery.data?.items)
    : safeArray(pathRootsQuery.data?.items);

  useEffect(() => {
    setDraft(workspaceDraft(workspace));
    setNewRuleScope('');
  }, [workspace]);

  const resetDraft = () => setDraft(workspaceDraft(workspace));

  const handleSave = async () => {
    await updateWorkspace.mutateAsync({
      workspaceId: workspace.workspace_id,
      payload: {
        name: draft.name,
        description: draft.description,
        workspace_kind: draft.workspace_kind,
        status: draft.status,
        local_roots: listFromTextarea(draft.local_roots),
        repo_roots: listFromTextarea(draft.repo_roots),
        protected_paths: listFromTextarea(draft.protected_paths),
        allowed_actions: draft.allowed_actions,
        boundary_policy: { out_of_scope_default: draft.out_of_scope_default },
        audit_enabled: draft.audit_enabled,
        tags: listFromTextarea(draft.tags),
        related_project_id: draft.related_project_id,
        notes: draft.notes,
      },
    });
  };

  const handleArchive = async () => {
    const ok = await confirmDialog({
      title: 'Archive workspace?',
      description: `"${workspace.name}" will be archived. You can restore it later.`,
      confirmLabel: 'Archive',
    });
    if (!ok) return;
    await archiveWorkspace.mutateAsync(workspace.workspace_id);
  };

  const handleDelete = async () => {
    const ok = await confirmDialog({
      title: 'Delete workspace?',
      description: `Permanently delete "${workspace.name}". This only succeeds when no project or durable resource still references it.`,
      confirmLabel: 'Delete',
      danger: true,
    });
    if (!ok) return;
    try {
      await deleteWorkspace.mutateAsync(workspace.workspace_id);
    } catch (err) {
      toast.error(`Delete failed: ${errorMessage(err)}`);
    }
  };

  const handleAddRule = async () => {
    const scope = newRuleScope.trim();
    if (!scope) return;
    await createWhitelistRule.mutateAsync({ workspaceId: workspace.workspace_id, scope, actionTypes: ['all'] });
    setNewRuleScope('');
  };

  const addPath = (key: 'local_roots' | 'repo_roots' | 'protected_paths') => {
    const path = pathDraft.trim();
    if (!path) return;
    setDraft((current) => ({
      ...current,
      [key]: appendLine(current[key], path),
    }));
    setPathDraft('');
  };

  const handleBrowseFolder = async () => {
    setPathPickerNote('');
    if (!window.showDirectoryPicker) {
      setPathPickerNote('This browser does not expose a directory picker. Paste the absolute path instead.');
      return;
    }
    try {
      const handle = await window.showDirectoryPicker();
      setPathPickerNote(
        handle.name
          ? `Selected "${handle.name}", but browsers do not expose its absolute path to web pages. Paste the full path to add it.`
          : 'Folder selected, but browsers do not expose absolute paths to web pages. Paste the full path to add it.',
      );
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') return;
      setPathPickerNote(`Folder picker failed: ${errorMessage(err)}`);
    }
  };

  const handleToggleServerBrowser = () => {
    setServerBrowserOpen((open) => !open);
    setPathPickerNote('Server browse lists folders visible to NaviD. In Docker, that means container paths and mounted host folders.');
  };

  const handleUseServerPath = (entry: WorkspacePathEntry) => {
    setPathDraft(entry.path);
    setPathPickerNote(`Using server-visible path: ${entry.path}`);
  };

  return (
    <>
      <div className={styles.detailHeader}>
        <div>
          <h3 className={styles.detailTitle}>{workspace.name}</h3>
          <div className={styles.badgeList}>
            <StatusBadge label={workspace.status ?? 'unknown'} variant={statusVariant(workspace.status)} />
            <StatusBadge label={workspace.workspace_kind ?? 'general'} variant="accent" />
            <StatusBadge label={`rules: ${activeRules(rules).length}`} variant="muted" />
          </div>
        </div>
        <div className={styles.actions}>
          <button className={styles.actionBtn} onClick={() => setActiveWorkspace.mutate(workspace.workspace_id)} disabled={setActiveWorkspace.isPending} type="button">
            <ShieldCheck size={14} />
            Activate
          </button>
          <button className={`${styles.actionBtn} ${styles.danger}`} onClick={handleArchive} disabled={archiveWorkspace.isPending} type="button">
            <Archive size={14} />
            Archive
          </button>
          <button className={`${styles.actionBtn} ${styles.danger}`} onClick={handleDelete} disabled={deleteWorkspace.isPending} type="button">
            <ShieldOff size={14} />
            Delete
          </button>
        </div>
      </div>

      <div className={styles.boundaryMap}>
        <BoundaryNode title="Workspace" value={workspace.name} tone="accent" />
        <BoundaryNode title="Local Roots" value={String(listFromTextarea(draft.local_roots).length)} tone="good" />
        <BoundaryNode title="Repo Roots" value={String(listFromTextarea(draft.repo_roots).length)} tone="good" />
        <BoundaryNode title="Protected" value={String(listFromTextarea(draft.protected_paths).length)} tone="warn" />
        <BoundaryNode title="Allow Rules" value={String(activeRules(rules).length)} tone="neutral" />
      </div>

      <div className={styles.grid}>
        <section className={styles.panel}>
          <div className={styles.panelHeader}>
            <h4>Identity</h4>
            <FolderOpen size={15} />
          </div>
          <div className={styles.formGrid}>
            <label className={styles.field}>
              <span>Name</span>
              <input value={draft.name} onChange={(event) => setDraft((value) => ({ ...value, name: event.target.value }))} />
            </label>
            <label className={styles.field}>
              <span>Kind</span>
              <select value={draft.workspace_kind} onChange={(event) => setDraft((value) => ({ ...value, workspace_kind: event.target.value }))}>
                <option value="general">general</option>
                <option value="project">project</option>
                <option value="repository">repository</option>
                <option value="functional">functional</option>
              </select>
            </label>
            <label className={styles.field}>
              <span>Status</span>
              <select value={draft.status} onChange={(event) => setDraft((value) => ({ ...value, status: event.target.value }))}>
                <option value="active">active</option>
                <option value="inactive">inactive</option>
                <option value="archived">archived</option>
                <option value="suspended">suspended</option>
              </select>
            </label>
            <label className={styles.field}>
              <span>Project binding</span>
              <select value={draft.related_project_id} onChange={(event) => setDraft((value) => ({ ...value, related_project_id: event.target.value }))}>
                <option value="">none</option>
                {projects.map((item) => (
                  <option key={item.project_id} value={item.project_id}>{item.title}</option>
                ))}
              </select>
            </label>
            <label className={styles.fieldWide}>
              <span>Description</span>
              <textarea value={draft.description} onChange={(event) => setDraft((value) => ({ ...value, description: event.target.value }))} />
            </label>
            <label className={styles.checkboxRow}>
              <input type="checkbox" checked={draft.audit_enabled} onChange={(event) => setDraft((value) => ({ ...value, audit_enabled: event.target.checked }))} />
              <span>Audit enabled</span>
            </label>
          </div>
        </section>

        <section className={styles.panel}>
          <div className={styles.panelHeader}>
            <h4>Allowed Actions</h4>
            <ShieldCheck size={15} />
          </div>
          <div className={styles.actionGrid}>
            {actionKeys.map((key) => (
              <label key={key} className={styles.actionToggle}>
                <input
                  type="checkbox"
                  checked={Boolean(draft.allowed_actions[key])}
                  onChange={(event) => setDraft((value) => ({
                    ...value,
                    allowed_actions: { ...value.allowed_actions, [key]: event.target.checked },
                  }))}
                />
                <span>{key}</span>
              </label>
            ))}
          </div>
          <label className={styles.field}>
            <span>Out-of-scope default</span>
            <select value={draft.out_of_scope_default} onChange={(event) => setDraft((value) => ({ ...value, out_of_scope_default: event.target.value }))}>
              <option value="prompt">prompt</option>
              <option value="deny">deny</option>
            </select>
          </label>
        </section>
      </div>

      <section className={styles.panel}>
        <div className={styles.panelHeader}>
          <h4>Boundary Roots</h4>
          <Boxes size={15} />
        </div>
        <div className={styles.pathAdder}>
          <input value={pathDraft} onChange={(event) => setPathDraft(event.target.value)} placeholder="Absolute folder path" />
          <button className={styles.actionBtn} onClick={handleBrowseFolder} type="button">Browser...</button>
          <button className={styles.actionBtn} onClick={handleToggleServerBrowser} type="button">
            <Server size={14} />
            Server
          </button>
          <button className={styles.actionBtn} onClick={() => addPath('local_roots')} disabled={!pathDraft.trim()} type="button">Add Local</button>
          <button className={styles.actionBtn} onClick={() => addPath('repo_roots')} disabled={!pathDraft.trim()} type="button">Add Repo</button>
          <button className={styles.actionBtn} onClick={() => addPath('protected_paths')} disabled={!pathDraft.trim()} type="button">Protect</button>
        </div>
        {pathPickerNote && <div className={styles.note}>{pathPickerNote}</div>}
        {serverBrowserOpen && (
          <div className={styles.serverBrowser}>
            <div className={styles.serverBrowserHeader}>
              <div>
                <strong>{serverBrowserPath || 'Server-visible roots'}</strong>
                <span>{pathRootsQuery.data?.in_container ? 'NaviD is running in a container; host folders must be mounted first.' : 'NaviD is running natively; these are local service paths.'}</span>
              </div>
              {serverBrowserPath && (
                <button
                  className={styles.actionBtn}
                  onClick={() => setServerBrowserPath(pathChildrenQuery.data?.parent ?? '')}
                  disabled={!pathChildrenQuery.data?.parent}
                  type="button"
                >
                  <ChevronLeft size={14} />
                  Up
                </button>
              )}
            </div>
            {pathRootsQuery.isLoading || pathChildrenQuery.isLoading ? <div className={styles.loadingRow}>Loading folders...</div> : null}
            {pathRootsQuery.error && !serverBrowserPath ? <div className={styles.errorRow}>{errorMessage(pathRootsQuery.error)}</div> : null}
            {pathChildrenQuery.error && serverBrowserPath ? <div className={styles.errorRow}>{errorMessage(pathChildrenQuery.error)}</div> : null}
            {!pathRootsQuery.isLoading && !pathChildrenQuery.isLoading && serverBrowserItems.length === 0 && (
              <div className={styles.loadingRow}>No child folders visible here.</div>
            )}
            {serverBrowserItems.map((entry) => (
              <div className={styles.serverBrowserRow} key={entry.path}>
                <button className={styles.folderButton} onClick={() => setServerBrowserPath(entry.path)} type="button">
                  <Folder size={14} />
                  <span>{entry.name}</span>
                  <small>{entry.path}</small>
                </button>
                <button className={styles.actionBtn} onClick={() => handleUseServerPath(entry)} type="button">Use</button>
              </div>
            ))}
          </div>
        )}
        <div className={styles.textareaGrid}>
          <TextareaField label="Local roots" value={draft.local_roots} onChange={(value) => setDraft((current) => ({ ...current, local_roots: value }))} />
          <TextareaField label="Repo roots" value={draft.repo_roots} onChange={(value) => setDraft((current) => ({ ...current, repo_roots: value }))} />
          <TextareaField label="Protected paths" value={draft.protected_paths} onChange={(value) => setDraft((current) => ({ ...current, protected_paths: value }))} />
          <TextareaField label="Tags" value={draft.tags} onChange={(value) => setDraft((current) => ({ ...current, tags: value }))} />
        </div>
        <div className={styles.actions}>
          <button className={styles.primaryButton} onClick={handleSave} disabled={!draft.name.trim() || updateWorkspace.isPending} type="button">
            <CheckCircle2 size={14} />
            {updateWorkspace.isPending ? 'Saving...' : 'Save Workspace'}
          </button>
          <button className={styles.actionBtn} onClick={resetDraft} type="button">
            <RotateCcw size={14} />
            Reset
          </button>
        </div>
      </section>

      <section className={styles.panel}>
        <div className={styles.panelHeader}>
          <h4>Whitelist Rules</h4>
          <ShieldOff size={15} />
        </div>
        <div className={styles.ruleCreate}>
          <input value={newRuleScope} onChange={(event) => setNewRuleScope(event.target.value)} placeholder="Path or glob scope" />
          <button className={styles.actionBtn} onClick={handleAddRule} disabled={!newRuleScope.trim() || createWhitelistRule.isPending} type="button">
            <Plus size={14} />
            Add Rule
          </button>
        </div>
        <RuleList
          rules={rules}
          loading={whitelistQuery.isLoading}
          error={whitelistQuery.error}
          onRevoke={(ruleId) => revokeWhitelistRule.mutate({ workspaceId: workspace.workspace_id, ruleId })}
        />
      </section>

      {project && <div className={styles.note}>Bound project: {project.title}</div>}
      <JsonPanel data={workspace} label="Workspace Payload" />
    </>
  );
}

function TextareaField({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <label className={styles.field}>
      <span>{label}</span>
      <textarea value={value} onChange={(event) => onChange(event.target.value)} />
    </label>
  );
}

function RuleList({
  rules,
  loading,
  error,
  onRevoke,
}: {
  rules: WhitelistRule[];
  loading: boolean;
  error: unknown;
  onRevoke: (ruleId: string) => void;
}) {
  if (loading) return <div className={styles.loadingRow}>Loading whitelist rules...</div>;
  if (error) return <div className={styles.errorRow}>{errorMessage(error)}</div>;
  if (rules.length === 0) return <div className={styles.loadingRow}>No durable allow rules.</div>;
  return (
    <div className={styles.rows}>
      {rules.map((rule) => (
        <div key={rule.rule_id} className={styles.row}>
          <div>
            <strong>{rule.scope}</strong>
            <span>{(rule.action_types ?? ['all']).join(', ')} / {rule.rule_id}</span>
          </div>
          <div className={styles.actions}>
            <StatusBadge label={rule.status ?? 'unknown'} variant={rule.status === 'active' ? 'success' : 'muted'} />
            {rule.status === 'active' && (
              <button className={styles.actionBtn} onClick={() => onRevoke(rule.rule_id)} type="button">Revoke</button>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}

function BoundaryNode({ title, value, tone }: { title: string; value: string; tone: 'accent' | 'good' | 'warn' | 'neutral' }) {
  return (
    <div className={`${styles.boundaryNode} ${styles[tone]}`}>
      <span>{title}</span>
      <strong>{value}</strong>
    </div>
  );
}

function workspaceDraft(workspace: Workspace) {
  return {
    name: workspace.name,
    description: workspace.description ?? '',
    workspace_kind: workspace.workspace_kind ?? 'general',
    status: workspace.status ?? 'active',
    local_roots: (workspace.local_roots ?? []).join('\n'),
    repo_roots: (workspace.repo_roots ?? []).join('\n'),
    protected_paths: (workspace.protected_paths ?? []).join('\n'),
    allowed_actions: workspace.allowed_actions ?? defaultAllowedActions,
    out_of_scope_default: workspace.boundary_policy?.out_of_scope_default ?? 'prompt',
    audit_enabled: Boolean(workspace.audit_enabled),
    tags: (workspace.tags ?? []).join('\n'),
    related_project_id: workspace.related_project_id ?? '',
    notes: workspace.notes ?? '',
  };
}

function listFromTextarea(value: string): string[] {
  return value.split('\n').map((item) => item.trim()).filter(Boolean);
}

function appendLine(value: string, next: string): string {
  const existing = listFromTextarea(value);
  if (existing.includes(next)) return existing.join('\n');
  return [...existing, next].join('\n');
}

function projectForWorkspace(workspace: Workspace, projects: Project[]): Project | undefined {
  return projects.find((project) => (
    project.workspace_id === workspace.workspace_id ||
    project.project_id === workspace.related_project_id
  ));
}

function activeWorkspaceLabel(workspace: Workspace | undefined, workspaceId: string): string {
  if (workspace?.name) return workspace.name;
  return workspaceId || 'none';
}

function statusVariant(value: unknown): StatusVariant {
  switch (String(value ?? '').toLowerCase()) {
    case 'active':
      return 'success';
    case 'inactive':
    case 'suspended':
      return 'warning';
    case 'archived':
      return 'danger';
    default:
      return 'muted';
  }
}

function modeVariant(value: unknown): StatusVariant {
  switch (String(value ?? '').toLowerCase()) {
    case 'global':
      return 'warning';
    case 'scoped':
      return 'success';
    case 'hybrid':
      return 'accent';
    default:
      return 'muted';
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function safeArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}
