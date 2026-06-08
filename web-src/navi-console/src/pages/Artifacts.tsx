import { useEffect, useState } from 'react';
import { 
  Archive, 
  Search, 
  FileText, 
  Database,
  Code, 
  History, 
  Download, 
  Info, 
  ExternalLink, 
  Filter,
  FolderTree,
  FolderSync,
  Plus,
  Monitor,
  RefreshCw,
  ShieldCheck
} from 'lucide-react';
import { 
  useArtifacts, 
  useArtifact, 
  getArtifactContent,
  restoreArtifactVersion,
  createArtifactExport
} from '@/api/artifacts';
import { 
  useWorkspaces, 
  useCreateWorkspace, 
  useDeleteWorkspace,
  useWorkspace
} from '@/api/workspaces';
import { JsonPanel } from '@/components/JsonPanel';
import { useToast } from '@/components/ui/Toast';
import { useConfirm } from '@/components/ui/ConfirmDialog';
import clsx from 'clsx';
import styles from './Artifacts.module.css';

type Tab = 'content' | 'metadata' | 'history' | 'workspace';
type ViewMode = 'artifacts' | 'workspaces';

const isTauri = () => typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;

export function Artifacts() {
  const [viewMode, setViewMode] = useState<ViewMode>('artifacts');
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | null>(null);
  const [selectedVersionId, setSelectedVersionId] = useState<string | null>(null);
  const [content, setContent] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<Tab>('content');
  const [search, setSearch] = useState('');

  const { data: artifacts = [], isLoading: listLoading, refetch: refetchList } = useArtifacts();
  const { data: workspacesData, isLoading: workspacesLoading, refetch: refetchWorkspaces } = useWorkspaces();
  const workspaces = workspacesData?.items ?? [];

  const { data: selectedArtifact, isLoading: detailLoading, refetch: refetchDetail } = useArtifact(selectedId);
  const { data: selectedWorkspace, isLoading: workspaceLoading } = useWorkspace(selectedWorkspaceId);

  const toast = useToast();
  const confirmDialog = useConfirm();

  const createWorkspace = useCreateWorkspace();
  const deleteWorkspace = useDeleteWorkspace();

  useEffect(() => {
    if (viewMode === 'artifacts' && artifacts.length > 0 && !selectedId) {
      setSelectedId(artifacts[0].id);
    } else if (viewMode === 'workspaces' && workspaces.length > 0 && !selectedWorkspaceId) {
      setSelectedWorkspaceId(workspaces[0].workspace_id);
    }
  }, [artifacts, workspaces, selectedId, selectedWorkspaceId, viewMode]);

  useEffect(() => {
    if (viewMode === 'workspaces') {
      setActiveTab('metadata');
    } else {
      setActiveTab('content');
    }
  }, [viewMode]);

  useEffect(() => {
    setSelectedVersionId(null);
    setContent(null);
  }, [selectedId]);

  useEffect(() => {
    if (selectedId && activeTab === 'content') {
      loadContent(selectedId, selectedVersionId || undefined);
    }
  }, [selectedId, selectedVersionId, activeTab]);

  async function loadContent(id: string, versionId?: string) {
    try {
      const rawContent = await getArtifactContent(id, versionId);
      setContent(rawContent);
    } catch (err) {
      console.error('Failed to load artifact content:', err);
      setContent('Error loading content');
    }
  }

  const filteredArtifacts = artifacts.filter(a => 
    a.title.toLowerCase().includes(search.toLowerCase()) ||
    a.subtype.toLowerCase().includes(search.toLowerCase()) ||
    a.id.toLowerCase().includes(search.toLowerCase())
  );

  const filteredWorkspaces = workspaces.filter(w => 
    w.name.toLowerCase().includes(search.toLowerCase()) ||
    w.workspace_id.toLowerCase().includes(search.toLowerCase())
  );

  async function handleLinkLocal() {
    let path = '';
    
    if (isTauri()) {
      try {
        const selected = await (window as unknown as { __TAURI_INTERNALS__?: { invoke?: (command: string, args: unknown) => Promise<unknown> } })
          .__TAURI_INTERNALS__?.invoke?.('plugin:dialog|open', {
            options: {
              directory: true,
              multiple: false,
              title: 'Select Workspace Folder',
            },
        });
        if (selected && typeof selected === 'string') {
          path = selected;
        } else {
          return; // Cancelled
        }
      } catch (err) {
        console.error('Failed to open native dialog:', err);
        path = prompt('Native picker failed. Enter Local Path manually:') || '';
      }
    } else {
      path = prompt('Local Path (e.g. C:/my-project):') || '';
    }

    if (!path) return;
    const trimmedPath = path.replace(/[\\/]+$/, '');
    const workspaceName = trimmedPath.split(/[\\/]/).pop() || trimmedPath || 'Local Workspace';

    try {
      await createWorkspace.mutateAsync({
        name: workspaceName,
        local_roots: [path],
        workspace_kind: 'general',
        status: 'active'
      });
      refetchWorkspaces();
    } catch (err) {
      toast.error('Failed to link workspace: ' + (err instanceof Error ? err.message : String(err)));
    }
  }

  async function handleDeleteWorkspace(id: string) {
    const ok = await confirmDialog({
      title: 'Unlink workspace?',
      description: 'NAVI will no longer have access to this workspace. The underlying files are not deleted.',
      confirmLabel: 'Unlink',
      danger: true,
    });
    if (!ok) return;
    try {
      await deleteWorkspace.mutateAsync(id);
      setSelectedWorkspaceId(null);
      refetchWorkspaces();
    } catch (err) {
      toast.error('Failed to delete workspace: ' + (err instanceof Error ? err.message : String(err)));
    }
  }

  async function handleRestore(versionId: string) {
    if (!selectedId) return;
    const ok = await confirmDialog({
      title: 'Restore this version?',
      description: 'This creates a new head version from the selected one. Current content is preserved in history.',
      confirmLabel: 'Restore',
    });
    if (!ok) return;
    try {
      await restoreArtifactVersion(selectedId, versionId);
      refetchDetail();
      refetchList();
      setSelectedVersionId(null);
      setActiveTab('content');
      toast.success('Version restored');
    } catch (err) {
      toast.error('Failed to restore version: ' + (err instanceof Error ? err.message : String(err)));
    }
  }

  async function handleExport() {
    if (!selectedId || !selectedArtifact) return;
    const format = prompt('Enter export format (e.g. markdown, json, csv):', (selectedArtifact.content_format as string) || 'markdown');
    if (!format) return;
    try {
      await createArtifactExport(selectedId, format, 'download');
      toast.success('Export request submitted. Check background tasks or direct downloads if supported.');
    } catch (err) {
      toast.error('Failed to create export: ' + (err instanceof Error ? err.message : String(err)));
    }
  }

  const getTypeIcon = (type: string) => {
    switch (type.toLowerCase()) {
      case 'document': return <FileText size={18} />;
      case 'data': return <Database size={18} />;
      case 'code': return <Code size={18} />;
      default: return <Archive size={18} />;
    }
  };

  return (
    <div className={styles.container}>
      <header className={styles.header}>
        <h1 className={styles.title}>Artifacts</h1>
        <div className={styles.detailActions}>
          <button className={styles.btnSecondary} onClick={() => viewMode === 'artifacts' ? refetchList() : refetchWorkspaces()}>
            <RefreshCw size={16} className={(listLoading || workspacesLoading) ? 'animate-spin' : ''} />
            <span>Refresh</span>
          </button>
          {viewMode === 'workspaces' && (
            <button className={styles.btnPrimary} onClick={handleLinkLocal}>
              <Plus size={16} />
              <span>Link Local Folder</span>
            </button>
          )}
        </div>
      </header>

      <main className={styles.main}>
        {/* Sidebar List */}
        <div className={styles.listPanel}>
          <div className={styles.listHeader}>
            <div className={styles.viewToggle}>
              <button 
                className={clsx(styles.toggleBtn, viewMode === 'artifacts' && styles.active)}
                onClick={() => setViewMode('artifacts')}
              >
                <Archive size={14} />
                <span>Artifacts</span>
              </button>
              <button 
                className={clsx(styles.toggleBtn, viewMode === 'workspaces' && styles.active)}
                onClick={() => setViewMode('workspaces')}
              >
                <FolderTree size={14} />
                <span>Workspaces</span>
              </button>
            </div>

            <div className={styles.searchWrapper}>
              <Search className={styles.searchIcon} size={14} />
              <input 
                type="text" 
                placeholder={viewMode === 'artifacts' ? "Search artifacts..." : "Search workspaces..."}
                className={styles.searchInput}
                value={search}
                onChange={e => setSearch(e.target.value)}
              />
            </div>
            <div className={styles.itemMeta}>
              {viewMode === 'artifacts' ? (
                <>
                  <Filter size={12} />
                  <span>{filteredArtifacts.length} artifacts</span>
                </>
              ) : (
                <>
                  <Monitor size={12} />
                  <span>{filteredWorkspaces.length} workspaces</span>
                </>
              )}
            </div>
          </div>
          <div className={styles.listScroll}>
            {viewMode === 'artifacts' ? (
              listLoading && artifacts.length === 0 ? (
                <div className={styles.emptyState}>Loading artifacts...</div>
              ) : filteredArtifacts.length === 0 ? (
                <div className={styles.emptyState}>No artifacts found</div>
              ) : (
                filteredArtifacts.map(artifact => (
                  <div 
                    key={artifact.id}
                    className={clsx(styles.artifactItem, selectedId === artifact.id && styles.active)}
                    onClick={() => setSelectedId(artifact.id)}
                  >
                    <div className={styles.itemHeader}>
                      <span className={styles.itemName}>{artifact.title}</span>
                      <span className={styles.typeTag}>{artifact.subtype}</span>
                    </div>
                    <div className={styles.itemMeta}>
                      {getTypeIcon(artifact.type)}
                      <span>v{artifact.head_version_number}</span>
                      <span>•</span>
                      <span>{artifact.last_updated ? new Date(artifact.last_updated).toLocaleDateString() : 'Never'}</span>
                    </div>
                  </div>
                ))
              )
            ) : (
              workspacesLoading && workspaces.length === 0 ? (
                <div className={styles.emptyState}>Loading workspaces...</div>
              ) : filteredWorkspaces.length === 0 ? (
                <div className={styles.emptyState}>No workspaces linked</div>
              ) : (
                filteredWorkspaces.map(ws => (
                  <div 
                    key={ws.workspace_id}
                    className={clsx(styles.artifactItem, selectedWorkspaceId === ws.workspace_id && styles.active)}
                    onClick={() => setSelectedWorkspaceId(ws.workspace_id)}
                  >
                    <div className={styles.itemHeader}>
                      <span className={styles.itemName}>{ws.name}</span>
                      <span className={styles.typeTag}>{ws.workspace_kind}</span>
                    </div>
                    <div className={styles.itemMeta}>
                      <FolderSync size={14} />
                      <span>{ws.status}</span>
                      <span>•</span>
                      <span>{ws.local_roots?.length || 0} local roots</span>
                    </div>
                  </div>
                ))
              )
            )}
          </div>
        </div>

        {/* Detail View */}
        <div className={styles.detailPanel}>
          {viewMode === 'artifacts' ? (
            selectedArtifact ? (
              <>
                <div className={styles.detailHeader}>
                  <div className={styles.detailTitleArea}>
                    <h2>{selectedArtifact.title}</h2>
                    <div className={styles.itemMeta}>
                      <span className={clsx(styles.badge, styles.badgeSuccess)}>
                        {selectedArtifact.lifecycle_state}
                      </span>
                      {selectedVersionId && (
                        <span className={clsx(styles.badge, styles.badgeWarning)}>
                          Viewing Version {selectedArtifact.versions?.find(v => v.id === selectedVersionId)?.version_number}
                        </span>
                      )}
                      <span>ID: {selectedArtifact.id}</span>
                      <span>•</span>
                      <span>{String(selectedArtifact.content_format ?? selectedArtifact.subtype ?? '')}</span>
                    </div>
                  </div>
                  <div className={styles.detailActions}>
                    <button className={styles.btnSecondary} onClick={handleExport}>
                      <Download size={16} />
                      <span>Export</span>
                    </button>
                    <button className={styles.btnPrimary} onClick={() => window.open(`/api/artifacts/${selectedId}/content`, '_blank')}>
                      <ExternalLink size={16} />
                      <span>Open Raw</span>
                    </button>
                  </div>
                </div>

                <div className={styles.detailContent}>
                  <div className={styles.tabs}>
                    <div 
                      className={clsx(styles.tab, activeTab === 'content' && styles.active)}
                      onClick={() => setActiveTab('content')}
                    >
                      <FileText size={14} style={{ marginRight: 6 }} />
                      Content
                    </div>
                    <div 
                      className={clsx(styles.tab, activeTab === 'metadata' && styles.active)}
                      onClick={() => setActiveTab('metadata')}
                    >
                      <Info size={14} style={{ marginRight: 6 }} />
                      Metadata
                    </div>
                    <div 
                      className={clsx(styles.tab, activeTab === 'history' && styles.active)}
                      onClick={() => setActiveTab('history')}
                    >
                      <History size={14} style={{ marginRight: 6 }} />
                      History
                    </div>
                  </div>

                  <div className={styles.tabContent}>
                    {detailLoading ? (
                      <div className={styles.emptyState}>Loading details...</div>
                    ) : (
                      <>
                        {activeTab === 'content' && (
                          <div className={styles.contentViewer}>
                            {content || 'No content available'}
                          </div>
                        )}

                        {activeTab === 'metadata' && (
                          <div className={styles.metadataGrid}>
                            <div className={styles.metaCard}>
                              <span className={styles.metaLabel}>Canonical Title</span>
                              <span className={styles.metaValue}>{selectedArtifact.canonical_title}</span>
                            </div>
                            <div className={styles.metaCard}>
                              <span className={styles.metaLabel}>Type / Subtype</span>
                              <span className={styles.metaValue}>{selectedArtifact.type} / {selectedArtifact.subtype}</span>
                            </div>
                            <div className={styles.metaCard}>
                              <span className={styles.metaLabel}>Workspace</span>
                              <span className={styles.metaValue}>{selectedArtifact.workspace_id}</span>
                            </div>
                            <div className={styles.metaCard}>
                              <span className={styles.metaLabel}>Project</span>
                              <span className={styles.metaValue}>{selectedArtifact.project_id || 'None'}</span>
                            </div>
                            <div className={styles.metaCard}>
                              <span className={styles.metaLabel}>Sync State</span>
                              <span className={styles.metaValue}>{selectedArtifact.sync_state}</span>
                            </div>
                            <div className={styles.metaCard}>
                              <span className={styles.metaLabel}>Description</span>
                              <span className={styles.metaValue}>{selectedArtifact.description || 'No description provided'}</span>
                            </div>
                            <div className={styles.metaCard}>
                              <span className={styles.metaLabel}>Created At</span>
                              <span className={styles.metaValue}>{new Date((selectedArtifact as any).created_at || '').toLocaleString()}</span>
                            </div>
                            <div className={styles.metaCard}>
                              <span className={styles.metaLabel}>Last Updated</span>
                              <span className={styles.metaValue}>{selectedArtifact.last_updated ? new Date(selectedArtifact.last_updated).toLocaleString() : 'Never'}</span>
                            </div>
                            <div className={styles.metaCard}>
                              <span className={styles.metaLabel}>Owner ID</span>
                              <span className={styles.metaValue}>{selectedArtifact.owner_id}</span>
                            </div>
                            <div className={styles.metaCard}>
                              <span className={styles.metaLabel}>Location</span>
                              <span className={styles.metaValue}>{selectedArtifact.location || 'Virtual'}</span>
                            </div>
                            <div style={{ gridColumn: '1 / -1' }}>
                              <span className={styles.metaLabel}>Attributes</span>
                              <JsonPanel data={selectedArtifact.attributes || {}} />
                            </div>
                          </div>
                        )}

                        {activeTab === 'history' && (
                          <div className={styles.versionList}>
                            {selectedArtifact.versions?.map(v => (
                              <div key={v.id} className={clsx(styles.versionItem, selectedVersionId === v.id && styles.active)}>
                                <div className={styles.versionInfo}>
                                  <span className={styles.versionNum}>
                                    Version {v.version_number}
                                    {v.id === selectedArtifact.current_version_id && (
                                      <span style={{ marginLeft: 8, fontSize: 10, color: 'var(--accent-primary)' }}>(Current)</span>
                                    )}
                                  </span>
                                  <span className={styles.versionDate}>
                                    {new Date(v.created_at).toLocaleString()}
                                  </span>
                                </div>
                                <div className={styles.detailActions}>
                                  <button 
                                    className={styles.btnSecondary} 
                                    style={{ padding: '0.25rem 0.5rem' }}
                                    onClick={() => {
                                      setSelectedVersionId(v.id);
                                      setActiveTab('content');
                                    }}
                                  >
                                    <FileText size={12} />
                                    <span>View</span>
                                  </button>
                                  {v.id !== selectedArtifact.current_version_id && (
                                    <button 
                                      className={styles.btnSecondary} 
                                      style={{ padding: '0.25rem 0.5rem' }}
                                      onClick={() => handleRestore(v.id)}
                                    >
                                      <RefreshCw size={12} />
                                      <span>Restore</span>
                                    </button>
                                  )}
                                </div>
                              </div>
                            )) || (
                              <div className={styles.emptyState}>No version history available</div>
                            )}
                          </div>
                        )}
                      </>
                    )}
                  </div>
                </div>
              </>
            ) : (
              <div className={styles.emptyState}>
                <Archive size={48} />
                <p>Select an artifact to view details</p>
              </div>
            )
          ) : (
            selectedWorkspace ? (
              <>
                <div className={styles.detailHeader}>
                  <div className={styles.detailTitleArea}>
                    <h2>{selectedWorkspace.name}</h2>
                    <div className={styles.itemMeta}>
                      <span className={clsx(styles.badge, styles.badgeSuccess)}>
                        {selectedWorkspace.status}
                      </span>
                      {isTauri() && (
                        <span className={clsx(styles.badge, styles.badgePrimary)}>
                          <ShieldCheck size={12} style={{ marginRight: 4 }} />
                          Native Access Active
                        </span>
                      )}
                      <span>ID: {selectedWorkspace.workspace_id}</span>
                      <span>•</span>
                      <span>{selectedWorkspace.workspace_kind}</span>
                    </div>
                  </div>
                  <div className={styles.detailActions}>
                    <button className={styles.btnSecondary} onClick={() => handleDeleteWorkspace(selectedWorkspace.workspace_id)}>
                      <Archive size={16} />
                      <span>Unlink</span>
                    </button>
                  </div>
                </div>

                <div className={styles.detailContent}>
                  <div className={styles.tabs}>
                    <div 
                      className={clsx(styles.tab, activeTab === 'metadata' && styles.active)}
                      onClick={() => setActiveTab('metadata')}
                    >
                      <Info size={14} style={{ marginRight: 6 }} />
                      Configuration
                    </div>
                  </div>

                  <div className={styles.tabContent}>
                    {workspaceLoading ? (
                      <div className={styles.emptyState}>Loading workspace...</div>
                    ) : (
                      <div className={styles.metadataGrid}>
                        <div className={styles.metaCard}>
                          <span className={styles.metaLabel}>Local Roots</span>
                          <div className={styles.rootList}>
                            {selectedWorkspace.local_roots?.map(root => (
                              <div key={root} className={styles.rootItem}>
                                <Monitor size={14} />
                                <span>{root}</span>
                              </div>
                            )) || <span className={styles.metaValue}>No local roots</span>}
                          </div>
                        </div>
                        <div className={styles.metaCard}>
                          <span className={styles.metaLabel}>Repository Roots</span>
                          <span className={styles.metaValue}>{selectedWorkspace.repo_roots?.join(', ') || 'None'}</span>
                        </div>
                        <div className={styles.metaCard}>
                          <span className={styles.metaLabel}>Audit Enabled</span>
                          <span className={styles.metaValue}>{selectedWorkspace.audit_enabled ? 'Yes' : 'No'}</span>
                        </div>
                        <div className={styles.metaCard}>
                          <span className={styles.metaLabel}>Created By</span>
                          <span className={styles.metaValue}>{selectedWorkspace.created_by}</span>
                        </div>
                        <div className={styles.metaCard}>
                          <span className={styles.metaLabel}>Boundary Policy</span>
                          <span className={styles.metaValue}>{selectedWorkspace.boundary_policy?.out_of_scope_default}</span>
                        </div>
                        <div style={{ gridColumn: '1 / -1' }}>
                          <span className={styles.metaLabel}>Allowed Actions</span>
                          <div className={styles.actionGrid}>
                            {Object.entries(selectedWorkspace.allowed_actions || {}).map(([action, allowed]) => (
                              <div key={action} className={clsx(styles.actionItem, Boolean(allowed) && styles.allowed)}>
                                {action}: {allowed ? 'YES' : 'NO'}
                              </div>
                            ))}
                          </div>
                        </div>
                        <div style={{ gridColumn: '1 / -1' }}>
                          <span className={styles.metaLabel}>Raw Metadata</span>
                          <JsonPanel data={selectedWorkspace.metadata || {}} />
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              </>
            ) : (
              <div className={styles.emptyState}>
                <FolderTree size={48} />
                <p>Select a workspace to view configuration</p>
              </div>
            )
          )}
        </div>
      </main>
    </div>
  );
}
