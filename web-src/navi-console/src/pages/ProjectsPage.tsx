import { useState, useMemo, useRef } from 'react';
import {
  Button,
  Select,
  SelectValue,
  Popover,
  ListBox,
  ListBoxItem,
  DialogTrigger,
  Modal,
  Dialog,
  Form,
  MenuTrigger,
  Menu,
  MenuItem,
} from 'react-aria-components';
import {
  Plus, FolderKanban, CheckCircle2, AlertCircle, PlayCircle, Code, ArrowUpDown, Check,
  X, Settings, Folder, Star,
  ArrowLeft, MoreHorizontal, Trash2, Archive, ChevronDown
} from 'lucide-react';
import { ICON_MAP, PRESET_COLORS } from '@/lib/iconMap';

import {
  useProject,
  useProjects,
  useCreateProject,
  useUpdateProject,
  useArchiveProject,
  useDeleteProject,
  useBindProjectWorkspace,
  useUnbindProjectWorkspace,
  useProjectReadiness,
  useProjectTasks,
  useProjectChats,
} from '@/api/projects';
import { useWorkspaces } from '@/api/workspaces';
import { useNavigate, useRoute } from '@/app/router';
import { isDraftChatId, openNewChat } from '@/lib/newChat';
import { ChatInput } from '@/components/chat/ChatInput';
import { Card } from '@/components/ui/Card';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { EmptyState } from '@/components/ui/EmptyState';
import { TimeAgo } from '@/components/ui/TimeAgo';
import { pickProjectDescriptionLabel, pickProjectNameLabel } from '@/lib/projectFormLabels';
import styles from './ProjectsPage.module.css';

interface ProjectsPageProps {
  projectId?: string;
}

export function ProjectsPage({ projectId }: ProjectsPageProps) {
  if (!projectId) {
    return <ProjectsList />;
  }

  return <ProjectWorkbench projectId={projectId} />;
}

function ProjectsList() {
  const navigate = useNavigate();
  const { data, isLoading } = useProjects();
  const [sortParam, setSortParam] = useState<'recent' | 'created' | 'alphabetical'>('recent');

  const projects = data?.items || [];

  const sortedProjects = useMemo(() => {
    return [...projects].sort((a, b) => {
      if (sortParam === 'recent') {
        const dateA = a.updated_at ? new Date(a.updated_at).getTime() : 0;
        const dateB = b.updated_at ? new Date(b.updated_at).getTime() : 0;
        return dateB - dateA;
      } else if (sortParam === 'created') {
        const dateA = a.created_at ? new Date(a.created_at).getTime() : 0;
        const dateB = b.created_at ? new Date(b.created_at).getTime() : 0;
        return dateB - dateA;
      } else {
        return (a.title || '').localeCompare(b.title || '');
      }
    });
  }, [projects, sortParam]);

  if (isLoading) {
    return <div className={styles.loading}>Loading projects...</div>;
  }

  return (
    <div className={styles.container}>
      <div className={styles.listHeader}>
        <h1 className={styles.listTitle}>Projects</h1>
        <div className={styles.listActions}>
          <Select 
            selectedKey={sortParam} 
            onSelectionChange={(key) => setSortParam(key as any)}
            aria-label="Sort projects"
            className={styles.sortSelect}
          >
            <Button className={styles.sortButton}>
              <ArrowUpDown size={18} />
            </Button>
            <Popover className={styles.popover} placement="bottom end">
              <ListBox className={styles.listBox}>
                <ListBoxItem id="recent" className={styles.listBoxItem}>
                  Recent {sortParam === 'recent' && <Check size={16} />}
                </ListBoxItem>
                <ListBoxItem id="created" className={styles.listBoxItem}>
                  Created {sortParam === 'created' && <Check size={16} />}
                </ListBoxItem>
                <ListBoxItem id="alphabetical" className={styles.listBoxItem}>
                  Alphabetical {sortParam === 'alphabetical' && <Check size={16} />}
                </ListBoxItem>
              </ListBox>
            </Popover>
          </Select>

          <CreateProjectModal />
        </div>
      </div>

      {sortedProjects.length > 0 ? (
        <div className={styles.projectGrid}>
          {sortedProjects.map(project => (
            <div 
              key={project.project_id} 
              className={styles.projectCard}
              onClick={() => navigate(`/projects/${project.project_id}`)}
            >
              <h3 className={styles.projectCardTitle}>{project.title}</h3>
              <p className={styles.projectCardDesc}>{project.description || 'No description provided.'}</p>
              <div className={styles.projectCardTime}>
                <TimeAgo date={project.updated_at || project.created_at} />
              </div>
            </div>
          ))}
        </div>
      ) : (
        <EmptyState
          icon={<FolderKanban size={24} />}
          title="No projects yet"
          description="Create your first project to get started."
        />
      )}
    </div>
  );
}

// Re-exported for backward compatibility
export { ICON_MAP, PRESET_COLORS };

function IconColorPopover({
  icon, color, onIconChange, onColorChange,
}: {
  icon: string; color: string;
  onIconChange: (v: string) => void; onColorChange: (v: string) => void;
}) {
  const colorInputRef = useRef<HTMLInputElement>(null);
  const IconComp = ICON_MAP[icon] ?? Folder;
  return (
    <DialogTrigger>
      <Button className={styles.iconTrigger} aria-label={`Icon: ${icon}. Click to change.`}>
        <IconComp size={16} style={{ color }} />
      </Button>
      <Popover className={styles.iconColorPopover} placement="bottom start">
        <Dialog style={{ outline: 'none' }} aria-label="Select icon and color">
          <div className={styles.popColorRow}>
            {PRESET_COLORS.map((c) => (
              <button
                key={c.hex}
                type="button"
                title={c.name}
                className={`${styles.popSwatch} ${color === c.hex ? styles.popSwatchActive : ''}`}
                style={{ background: c.hex }}
                onClick={() => onColorChange(c.hex)}
              />
            ))}
            <button
              type="button"
              className={styles.rainbowBtn}
              title="Custom color"
              onClick={() => colorInputRef.current?.click()}
            />
            <input
              ref={colorInputRef}
              type="color"
              value={color}
              onChange={(e) => onColorChange(e.target.value)}
              className={styles.hiddenColorInput}
            />
          </div>
          <div className={styles.popDivider} />
          <div className={styles.popIconGrid}>
            {Object.entries(ICON_MAP).map(([name, Comp]) => (
              <button
                key={name}
                type="button"
                title={name}
                className={`${styles.popIconCell} ${name === icon ? styles.popIconCellActive : ''}`}
                onClick={() => onIconChange(name)}
              >
                <Comp size={16} style={{ color: name === icon ? color : undefined }} />
              </button>
            ))}
          </div>
        </Dialog>
      </Popover>
    </DialogTrigger>
  );
}

const MEMORY_OPTIONS = [
  {
    value: 'default',
    label: 'Default',
    desc: 'Project can access memories from outside chats, and vice versa.',
  },
  {
    value: 'project_only',
    label: 'Project-only',
    desc: "Project can only access its own memories. Its memories are hidden from outside chats.",
  },
];

function SettingsCogPopover({
  memoryScope, setMemoryScope, workspaceId, setWorkspaceId, workspaces,
}: {
  memoryScope: string; setMemoryScope: (v: string) => void;
  workspaceId: string; setWorkspaceId: (v: string) => void;
  workspaces: Array<{ workspace_id: string; name: string }>;
}) {
  return (
    <DialogTrigger>
      <Button className={styles.headerIconBtn} aria-label="Project settings">
        <Settings size={14} />
      </Button>
      <Popover className={styles.settingsPopover} placement="bottom end">
        <Dialog style={{ outline: 'none' }} aria-label="Project settings options">
          {/* Memory */}
          <div className={styles.settingsSection}>
            <div className={styles.settingsSectionHead}>
              <span className={styles.settingsSectionTitle}>Memory</span>
              <span className={styles.settingsSectionNote}>Note that this setting can't be changed later.</span>
            </div>
            {MEMORY_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                type="button"
                className={`${styles.settingsOption} ${memoryScope === opt.value ? styles.settingsOptionActive : ''}`}
                onClick={() => setMemoryScope(opt.value)}
              >
                <div className={styles.settingsOptionBody}>
                  <span className={styles.settingsOptionLabel}>{opt.label}</span>
                  <span className={styles.settingsOptionDesc}>{opt.desc}</span>
                </div>
                {memoryScope === opt.value && <Check size={13} className={styles.settingsOptionCheck} />}
              </button>
            ))}
          </div>

          {/* Workspace — only shown when workspaces exist */}
          {workspaces.length > 0 && (
            <>
              <div className={styles.settingsDivider} />
              <div className={styles.settingsSection}>
                <div className={styles.settingsSectionHead}>
                  <span className={styles.settingsSectionTitle}>Workspace</span>
                </div>
                <div className={styles.settingRow}>
                  <Select
                    selectedKey={workspaceId}
                    onSelectionChange={(key) => setWorkspaceId(key as string)}
                    aria-label="Workspace"
                    className={styles.settingSelectContainer}
                  >
                    <Button className={styles.settingSelectBtn}>
                      <SelectValue />
                      <ChevronDown size={14} className={styles.settingSelectChevron} />
                    </Button>
                    <Popover className={styles.settingSelectPopover} placement="bottom start">
                      <ListBox className={styles.settingSelectListBox}>
                        <ListBoxItem id="" textValue="None" className={styles.settingSelectListBoxItem}>
                          <span>None</span>
                          {workspaceId === '' && <Check size={13} className={styles.settingsOptionCheck} />}
                        </ListBoxItem>
                        {workspaces.map((ws) => (
                          <ListBoxItem
                            key={ws.workspace_id}
                            id={ws.workspace_id}
                            textValue={ws.name}
                            className={styles.settingSelectListBoxItem}
                          >
                            <span>{ws.name}</span>
                            {workspaceId === ws.workspace_id && <Check size={13} className={styles.settingsOptionCheck} />}
                          </ListBoxItem>
                        ))}
                      </ListBox>
                    </Popover>
                  </Select>
                </div>
              </div>
            </>
          )}
        </Dialog>
      </Popover>
    </DialogTrigger>
  );
}

function CreateProjectModal() {
  const [isOpen, setIsOpen] = useState(false);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [icon, setIcon] = useState('folder');
  const [color, setColor] = useState('#3b82f6');
  const [memoryScope, setMemoryScope] = useState('default');
  const [workspaceId, setWorkspaceId] = useState('');

  const nameLabel = useMemo(() => pickProjectNameLabel(), []);
  const descriptionLabel = useMemo(() => pickProjectDescriptionLabel(), []);
  const createProject = useCreateProject();
  const navigate = useNavigate();
  const { data: workspacesData } = useWorkspaces();
  const workspaces = workspacesData?.items ?? [];

  const resetForm = () => {
    setTitle('');
    setDescription('');
    setIcon('folder');
    setColor('#3b82f6');
    setMemoryScope('default');
    setWorkspaceId('');
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;
    try {
      const res = await createProject.mutateAsync({
        title: title.trim(),
        description: description.trim(),
        icon,
        color,
        memory_scope: memoryScope,
        project_kind: 'general',
        workspace_id: workspaceId || undefined,
      });
      setIsOpen(false);
      resetForm();
      navigate(`/projects/${res.project_id}`);
    } catch (err) {
      console.error('Failed to create project:', err);
    }
  };

  return (
    <DialogTrigger isOpen={isOpen} onOpenChange={(open) => { setIsOpen(open); if (!open) resetForm(); }}>
      <Button className={styles.newProjectButton}>New project</Button>
      <Modal className={styles.modalOverlay} isDismissable>
        <Dialog className={styles.modalContent} aria-label="Create a new project">
          {({ close }) => (
            <Form onSubmit={handleSubmit}>
              <div className={styles.modalHeader}>
                <h2 className={styles.modalTitle}>New project</h2>
                <div className={styles.modalHeaderActions}>
                  <SettingsCogPopover
                    memoryScope={memoryScope} setMemoryScope={setMemoryScope}
                    workspaceId={workspaceId} setWorkspaceId={setWorkspaceId}
                    workspaces={workspaces}
                  />
                  <Button className={styles.headerIconBtn} onPress={close} aria-label="Close">
                    <X size={14} />
                  </Button>
                </div>
              </div>

              <div className={styles.formGroup}>
                <label className={styles.formLabel}>{nameLabel}</label>
                <div className={styles.nameFieldWrap}>
                  <IconColorPopover
                    icon={icon} color={color}
                    onIconChange={setIcon} onColorChange={setColor}
                  />
                  <input
                    className={styles.nameInput}
                    value={title}
                    onChange={(e) => setTitle(e.target.value)}
                    placeholder="Name your project"
                    required
                    autoFocus
                  />
                </div>
              </div>

              <div className={styles.formGroup}>
                <label className={styles.formLabel}>{descriptionLabel}</label>
                <textarea
                  className={styles.formTextarea}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Scope and purpose (optional)"
                  rows={3}
                />
              </div>

              <div className={styles.modalActions}>
                <Button
                  type="submit"
                  className="navi-button navi-button-primary"
                  isDisabled={createProject.isPending || !title.trim()}
                >
                  {createProject.isPending ? 'Creating…' : 'Create project'}
                </Button>
              </div>
            </Form>
          )}
        </Dialog>
      </Modal>
    </DialogTrigger>
  );
}

function ProjectWorkbench({ projectId }: { projectId: string }) {
  const navigate = useNavigate();
  const { params } = useRoute();
  const { data: project, isLoading: isProjectLoading } = useProject(projectId);
  const { data: readiness, isLoading: isReadinessLoading } = useProjectReadiness(projectId);
  const { data: chatsData } = useProjectChats(projectId);
  const { data: tasksData } = useProjectTasks(projectId);
  
  const deleteProject = useDeleteProject();
  const archiveProject = useArchiveProject();
  const updateProject = useUpdateProject();
  const bindProjectWorkspace = useBindProjectWorkspace();
  const unbindProjectWorkspace = useUnbindProjectWorkspace();
  
  const { data: workspacesData } = useWorkspaces();
  const workspaces = workspacesData?.items ?? [];

  const [activeTab, setActiveTab] = useState<'chats' | 'sources'>('chats');
  const [showMore, setShowMore] = useState(false);
  const [composerText, setComposerText] = useState('');
  
  // Settings modal local state
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [settingsTitle, setSettingsTitle] = useState('');
  const [settingsDesc, setSettingsDesc] = useState('');
  const [settingsIcon, setSettingsIcon] = useState('folder');
  const [settingsColor, setSettingsColor] = useState('#3b82f6');
  const [settingsInstructions, setSettingsInstructions] = useState('');
  const [settingsMemory, setSettingsMemory] = useState('default');
  const [settingsWorkspace, setSettingsWorkspace] = useState('');
  
  const [isDeleteConfirmOpen, setIsDeleteConfirmOpen] = useState(false);

  const handleNewProjectChat = () => {
    openNewChat(navigate, { projectId, replace: isDraftChatId(params?.chatId) });
  };

  const handleComposerSubmit = () => {
    if (!composerText.trim()) return;
    const text = composerText.trim();
    setComposerText('');
    openNewChat(navigate, { projectId, compose: text });
  };

  const handleOpenSettings = () => {
    if (!project) return;
    setSettingsTitle(project.title);
    setSettingsDesc(project.description || '');
    setSettingsIcon(project.icon || 'folder');
    setSettingsColor(project.color || '#3b82f6');
    setSettingsInstructions((project.attributes?.instructions as string) || '');
    setSettingsMemory(project.memory_scope || 'default');
    setSettingsWorkspace(project.workspace_id || '');
    setIsSettingsOpen(true);
  };

  const handleSaveSettings = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!settingsTitle.trim() || !project) return;
    try {
      await updateProject.mutateAsync({
        projectId,
        payload: {
          title: settingsTitle.trim(),
          description: settingsDesc.trim(),
          icon: settingsIcon,
          color: settingsColor,
          memory_scope: settingsMemory,
          workspace_id: settingsWorkspace || undefined,
          attributes: {
            ...project.attributes,
            instructions: settingsInstructions.trim(),
          }
        }
      });
      
      // Update workspace binding if changed
      if (settingsWorkspace !== (project.workspace_id || '')) {
        if (settingsWorkspace) {
          await bindProjectWorkspace.mutateAsync({ projectId, workspaceId: settingsWorkspace });
        } else {
          await unbindProjectWorkspace.mutateAsync(projectId);
        }
      }
      
      setIsSettingsOpen(false);
    } catch (err) {
      console.error('Failed to update project:', err);
    }
  };

  const handleArchive = async () => {
    try {
      await archiveProject.mutateAsync(projectId);
    } catch (err) {
      console.error('Failed to archive project:', err);
    }
  };

  const handleDelete = async () => {
    try {
      await deleteProject.mutateAsync(projectId);
      setIsDeleteConfirmOpen(false);
      navigate('/projects');
    } catch (err) {
      console.error('Failed to delete project:', err);
    }
  };

  const handleToggleFavorite = async () => {
    if (!project) return;
    const isFav = !!project.attributes?.favorite;
    try {
      await updateProject.mutateAsync({
        projectId,
        payload: {
          title: project.title,
          attributes: {
            ...project.attributes,
            favorite: !isFav,
          }
        }
      });
    } catch (err) {
      console.error('Failed to toggle favorite:', err);
    }
  };

  if (isProjectLoading) {
    return <div className={styles.loading}>Loading project...</div>;
  }

  if (!project) {
    return <div className={styles.loading}>Project not found</div>;
  }

  const chats = chatsData?.items || [];
  const tasks = tasksData?.items || [];
  const isFavorite = !!project.attributes?.favorite;

  const description = project.description || 'No description provided.';
  const needsCollapsible = description.length > 120;
  const displayDescription = needsCollapsible && !showMore
    ? `${description.slice(0, 120)}...`
    : description;

  const IconComp = ICON_MAP[project.icon || 'folder'] ?? Folder;

  return (
    <div className={styles.container}>
      {/* Back to all projects link */}
      <div className={styles.backContainer}>
        <span className={styles.backLink} onClick={() => navigate('/projects')}>
          <ArrowLeft size={14} style={{ marginRight: '6px' }} />
          All projects
        </span>
      </div>

      {/* Redesigned V2 Header */}
      <div className={styles.headerV2}>
        <div className={styles.headerLeft}>
          <div className={styles.projectIconWrapper} style={{ backgroundColor: `${project.color}15` }}>
            <IconComp size={24} style={{ color: project.color || '#3b82f6' }} />
          </div>
          <div className={styles.titleAndStatus}>
            <h1 className={styles.title}>{project.title}</h1>
            <StatusBadge 
              label={project.status || 'active'} 
              variant={project.status === 'active' ? 'running' : 'default'} 
            />
          </div>
        </div>
        
        <div className={styles.headerRight}>
          <Button
            className={`${styles.headerActionBtn} ${isFavorite ? styles.favorited : ''}`}
            onPress={handleToggleFavorite}
            aria-label={isFavorite ? "Unfavorite project" : "Favorite project"}
          >
            <Star size={16} fill={isFavorite ? "var(--navi-warning, #eab308)" : "none"} style={{ color: isFavorite ? "var(--navi-warning, #eab308)" : "inherit" }} />
          </Button>

          <MenuTrigger>
            <Button className={styles.headerActionBtn} aria-label="More actions">
              <MoreHorizontal size={16} />
            </Button>
            <Popover className={styles.popover} placement="bottom end">
              <Menu 
                className={styles.listBox} 
                style={{ outline: 'none' }}
                onAction={(key) => {
                  if (key === 'edit') handleOpenSettings();
                  else if (key === 'archive') handleArchive();
                  else if (key === 'delete') setIsDeleteConfirmOpen(true);
                }}
              >
                <MenuItem id="edit" className={styles.listBoxItem} textValue="Edit details">
                  <div className={styles.menuItemContent}>
                    <Settings size={14} />
                    <span>Edit details</span>
                  </div>
                </MenuItem>
                <MenuItem id="archive" className={styles.listBoxItem} textValue="Archive project">
                  <div className={styles.menuItemContent}>
                    <Archive size={14} />
                    <span>Archive</span>
                  </div>
                </MenuItem>
                <MenuItem id="delete" className={styles.listBoxItemDelete} textValue="Delete project">
                  <div className={styles.menuItemContent}>
                    <Trash2 size={14} />
                    <span>Delete</span>
                  </div>
                </MenuItem>
              </Menu>
            </Popover>
          </MenuTrigger>
        </div>
      </div>

      {/* Description area */}
      <div className={styles.descriptionSection}>
        <p className={styles.descriptionV2}>
          {displayDescription}
          {needsCollapsible && (
            <button onClick={() => setShowMore(!showMore)} className={styles.showMoreBtn}>
              {showMore ? 'Show less' : 'Show more'}
            </button>
          )}
        </p>
      </div>

      <ChatInput
        value={composerText}
        onChange={setComposerText}
        onSend={handleComposerSubmit}
        placeholder="How can I help you today?"
        className={styles.composerWrapper}
      />

      {/* Tabs / Pills */}
      <div className={styles.tabBar}>
        <button
          type="button"
          className={`${styles.tabBtn} ${activeTab === 'chats' ? styles.tabBtnActive : ''}`}
          onClick={() => setActiveTab('chats')}
        >
          Chats
          <span className={styles.tabBadge}>{chats.length}</span>
        </button>
        <button
          type="button"
          className={`${styles.tabBtn} ${activeTab === 'sources' ? styles.tabBtnActive : ''}`}
          onClick={() => setActiveTab('sources')}
        >
          Sources
          <span className={styles.tabBadge}>
            {project.workspace_id ? '1' : '0'}
          </span>
        </button>
      </div>

      {/* Main Grid */}
      <div className={styles.grid}>
        {/* Main Column */}
        <div className={styles.mainCol}>
          {activeTab === 'chats' ? (
            <section className={styles.section}>
              <div className={styles.sectionHeader}>
                <h2 className={styles.sectionTitle}>Chats</h2>
                <Button className="navi-button navi-button-ghost" onPress={handleNewProjectChat}>
                  <Plus size={16} /> New
                </Button>
              </div>
              
              {chats.length > 0 ? (
                <div className={styles.list}>
                  {chats.map(chat => (
                    <Card key={chat.chat_id} hoverable onClick={() => navigate(`/projects/${projectId}/chats/${chat.chat_id}`)}>
                      <div className={styles.listItem}>
                        <div className={styles.listMain}>
                          <span className={styles.listTitle}>{chat.title || 'Untitled Chat'}</span>
                          <span className={styles.listMeta}><TimeAgo date={chat.updated_at} /></span>
                        </div>
                        <StatusBadge label={chat.status || 'idle'} />
                      </div>
                    </Card>
                  ))}
                </div>
              ) : (
                <Card className={styles.emptyCard}>
                  No chats in this project yet.
                </Card>
              )}
            </section>
          ) : (
            <section className={styles.section}>
              <div className={styles.sectionHeader}>
                <h2 className={styles.sectionTitle}>Sources</h2>
              </div>
              <div className={styles.sourcesList}>
                {project.workspace_id ? (
                  <Card className={styles.sourceCard}>
                    <div className={styles.sourceCardHeader}>
                      <Folder size={18} className={styles.sourceCardIcon} />
                      <div className={styles.sourceCardMeta}>
                        <span className={styles.sourceCardTitle}>Workspace Bound</span>
                        <span className={styles.sourceCardDesc}>ID: {project.workspace_id}</span>
                      </div>
                      <Button
                        className="navi-button navi-button-ghost"
                        onPress={() => navigate('/workspaces')}
                      >
                        Manage
                      </Button>
                    </div>
                  </Card>
                ) : (
                  <Card className={styles.emptyCard}>
                    No sources connected. Bind a workspace in Settings to add sources.
                  </Card>
                )}
              </div>
            </section>
          )}

          {/* Readiness Section at the bottom */}
          <section className={styles.section}>
            <h2 className={styles.sectionTitle}>Readiness</h2>
            {isReadinessLoading ? (
              <div className={styles.loadingSmall}>Checking readiness...</div>
            ) : readiness ? (
              <div className={styles.readinessCards}>
                <Card variant={readiness.ready_for_chat ? 'elevated' : 'default'} className={styles.readyCard}>
                  <div className={styles.readyCardHeader}>
                    {readiness.ready_for_chat ? (
                      <CheckCircle2 size={18} className={styles.iconSuccess} />
                    ) : (
                      <AlertCircle size={18} className={styles.iconWarning} />
                    )}
                    <span className={styles.readyCardTitle}>Workspace</span>
                  </div>
                  <p className={styles.readyCardDesc}>
                    {readiness.workspace_id 
                      ? 'Workspace is bound and accessible.' 
                      : 'No workspace bound to this project.'}
                  </p>
                </Card>

                <Card variant="default" className={styles.readyCard}>
                  <div className={styles.readyCardHeader}>
                    <Code size={18} className={styles.iconDefault} />
                    <span className={styles.readyCardTitle}>Context</span>
                  </div>
                  <p className={styles.readyCardDesc}>
                    {readiness.ready_for_coding 
                      ? 'Ready for coding tasks.' 
                      : 'Not fully ready for coding.'}
                  </p>
                </Card>
              </div>
            ) : null}
          </section>
        </div>

        {/* Right Column */}
        <div className={styles.sideCol}>
          {/* Project Tasks */}
          <section className={styles.section}>
            <div className={styles.sectionHeader}>
              <h2 className={styles.sectionTitle}>Tasks</h2>
            </div>
            
            {tasks.length > 0 ? (
              <div className={styles.listCompact}>
                {tasks.map(task => (
                  <Card key={task.task_id} className={styles.taskCard}>
                    <div className={styles.taskHeader}>
                      <span className={styles.taskTitle}>{task.title}</span>
                      <StatusBadge label={task.status || 'pending'} variant={task.status === 'completed' ? 'completed' : task.status === 'running' ? 'running' : 'default'} />
                    </div>
                    <div className={styles.taskMeta}>
                      <span className={styles.taskType}>{task.task_class || 'Task'}</span>
                    </div>
                  </Card>
                ))}
              </div>
            ) : (
              <Card className={styles.emptyCard}>
                No active tasks.
              </Card>
            )}
          </section>

          {/* Recent Runs */}
          <section className={styles.section}>
            <div className={styles.sectionHeader}>
              <h2 className={styles.sectionTitle}>Recent Runs</h2>
            </div>
            <Card className={styles.emptyCard}>
              <PlayCircle size={16} style={{marginRight: '8px', verticalAlign: 'middle'}}/>
              Run history will appear here.
            </Card>
          </section>
        </div>
      </div>

      {/* Durable Settings Modal */}
      <Modal isOpen={isSettingsOpen} onOpenChange={setIsSettingsOpen} className={styles.modalOverlay} isDismissable>
        <Dialog className={styles.modalContent} aria-label="Edit project details">
          {({ close }) => (
            <Form onSubmit={handleSaveSettings}>
              <div className={styles.modalHeader}>
                <h2 className={styles.modalTitle}>Project settings</h2>
                <Button className={styles.headerIconBtn} onPress={close} aria-label="Close">
                  <X size={14} />
                </Button>
              </div>

              <div className={styles.formGroup}>
                <label className={styles.formLabel}>Name</label>
                <div className={styles.nameFieldWrap}>
                  <IconColorPopover
                    icon={settingsIcon} color={settingsColor}
                    onIconChange={setSettingsIcon} onColorChange={setSettingsColor}
                  />
                  <input
                    className={styles.nameInput}
                    value={settingsTitle}
                    onChange={(e) => setSettingsTitle(e.target.value)}
                    placeholder="Name your project"
                    required
                  />
                </div>
              </div>

              <div className={styles.formGroup}>
                <label className={styles.formLabel}>Description</label>
                <textarea
                  className={styles.formTextarea}
                  value={settingsDesc}
                  onChange={(e) => setSettingsDesc(e.target.value)}
                  placeholder="Scope and purpose (optional)"
                  rows={2}
                />
              </div>

              <div className={styles.formGroup}>
                <label className={styles.formLabel}>Instructions</label>
                <textarea
                  className={styles.formTextarea}
                  value={settingsInstructions}
                  onChange={(e) => setSettingsInstructions(e.target.value)}
                  placeholder="Instructions for NAVI specifically for this project (optional)"
                  rows={3}
                />
              </div>

              <div className={styles.formGroup}>
                <label className={styles.formLabel}>Memory</label>
                <div className={styles.memoryScopeGroup}>
                  {MEMORY_OPTIONS.map((opt) => (
                    <button
                      key={opt.value}
                      type="button"
                      className={`${styles.memoryScopeBtn} ${settingsMemory === opt.value ? styles.memoryScopeBtnActive : ''}`}
                      onClick={() => setSettingsMemory(opt.value)}
                    >
                      <span className={styles.memoryScopeLabel}>{opt.label}</span>
                      <span className={styles.memoryScopeDesc}>{opt.desc}</span>
                    </button>
                  ))}
                </div>
              </div>

              {workspaces.length > 0 && (
                <div className={styles.formGroup}>
                  <label className={styles.formLabel}>Workspace</label>
                  <Select
                    selectedKey={settingsWorkspace}
                    onSelectionChange={(key) => setSettingsWorkspace(key as string)}
                    aria-label="Workspace"
                    className={styles.settingSelectContainer}
                  >
                    <Button className={styles.settingSelectBtn}>
                      <SelectValue />
                      <ChevronDown size={14} className={styles.settingSelectChevron} />
                    </Button>
                    <Popover className={styles.settingSelectPopover} placement="bottom start">
                      <ListBox className={styles.settingSelectListBox}>
                        <ListBoxItem id="" textValue="None" className={styles.settingSelectListBoxItem}>
                          <span>None</span>
                          {settingsWorkspace === '' && <Check size={13} className={styles.settingsOptionCheck} />}
                        </ListBoxItem>
                        {workspaces.map((ws) => (
                          <ListBoxItem
                            key={ws.workspace_id}
                            id={ws.workspace_id}
                            textValue={ws.name}
                            className={styles.settingSelectListBoxItem}
                          >
                            <span>{ws.name}</span>
                            {settingsWorkspace === ws.workspace_id && <Check size={13} className={styles.settingsOptionCheck} />}
                          </ListBoxItem>
                        ))}
                      </ListBox>
                    </Popover>
                  </Select>
                </div>
              )}

              <div className={styles.settingsModalFooter}>
                <Button
                  type="button"
                  className={styles.deleteProjectBtn}
                  onPress={() => {
                    setIsSettingsOpen(false);
                    setIsDeleteConfirmOpen(true);
                  }}
                >
                  <Trash2 size={14} style={{ marginRight: '6px' }} />
                  Delete project
                </Button>
                <div className={styles.settingsModalFooterRight}>
                  <Button
                    type="button"
                    className="navi-button navi-button-ghost"
                    onPress={close}
                    style={{ marginRight: '8px' }}
                  >
                    Cancel
                  </Button>
                  <Button
                    type="submit"
                    className="navi-button navi-button-primary"
                    isDisabled={updateProject.isPending}
                  >
                    {updateProject.isPending ? 'Saving...' : 'Save changes'}
                  </Button>
                </div>
              </div>
            </Form>
          )}
        </Dialog>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal isOpen={isDeleteConfirmOpen} onOpenChange={setIsDeleteConfirmOpen} className={styles.modalOverlay} isDismissable>
        <Dialog className={styles.modalContent} aria-label="Confirm Project Deletion">
          {({ close }) => (
            <div>
              <div className={styles.modalHeader}>
                <h2 className={styles.modalTitle} style={{ color: 'var(--navi-error, #ef4444)' }}>Delete Project</h2>
                <Button className={styles.headerIconBtn} onPress={close} aria-label="Close">
                  <X size={14} />
                </Button>
              </div>
              <p className={styles.modalText} style={{ marginBottom: '24px', fontSize: '14px', color: 'var(--navi-text-secondary)' }}>
                Are you sure you want to delete <strong>{project.title}</strong>? This action is permanent and will delete the project metadata, while safety-cleaning tasks, chats, and workspaces.
              </p>
              <div className={styles.modalActions}>
                <Button className="navi-button navi-button-ghost" onPress={close} style={{ marginRight: '8px' }}>
                  Cancel
                </Button>
                <Button
                  className="navi-button"
                  onPress={handleDelete}
                  style={{ backgroundColor: 'var(--navi-error, #ef4444)', color: '#fff', border: 'none' }}
                  isDisabled={deleteProject.isPending}
                >
                  {deleteProject.isPending ? 'Deleting...' : 'Delete Permanently'}
                </Button>
              </div>
            </div>
          )}
        </Dialog>
      </Modal>
    </div>
  );
}
