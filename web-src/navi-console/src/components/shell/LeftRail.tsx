import { Fragment, useCallback, useState, type ReactNode } from 'react';
import { Button, Tooltip, TooltipTrigger, Disclosure, DisclosurePanel, MenuTrigger, Popover, Menu, MenuItem } from 'react-aria-components';
import {
  FolderKanban,
  MessageSquare,
  Code2,
  Play,
  Puzzle,
  Settings,
  Bug,
  Plus,
  PanelLeftClose,
  ChevronRight,
  Boxes,
  FileCode,
  GitPullRequest,
  Activity,
  CalendarClock,
  BookOpen,
  LayoutDashboard,
  MoreVertical,
  Archive,
  Trash2,
  Edit,
  FolderUp,
  FolderMinus,
} from 'lucide-react';
import { useRoute, useNavigate } from '@/app/router';
import { useChats, archiveChat, deleteChat } from '@/api/chats';
import { useProjects, useProjectChats } from '@/api/projects';
import { isDraftChatId, isOnDraftPath, openNewChat } from '@/lib/newChat';
import { ICON_MAP } from '@/lib/iconMap';
import clsx from 'clsx';
import { NaviProfileButton, type NaviProfileStatus } from './NaviProfileButton';
import styles from './LeftRail.module.css';
import { useQueryClient } from '@tanstack/react-query';
import { useToast } from '@/components/ui/Toast';
import { useConfirm } from '@/components/ui/ConfirmDialog';
import { RenameChatModal, MoveProjectModal } from '@/components/chat/ChatOptionsModals';
import { updateChat } from '@/api/chats';
import type { Project } from '@/types/api';

interface LeftRailProps {
  collapsed: boolean;
  onToggleCollapsed: () => void;
  onExpand: () => void;
  profileStatus: NaviProfileStatus;
  profileDetail?: string;
}

export function LeftRail({
  collapsed,
  onToggleCollapsed,
  onExpand,
  profileStatus,
  profileDetail,
}: LeftRailProps) {
  const { route, params, path } = useRoute();
  const navigate = useNavigate();
  const chatsQuery = useChats();
  const projectsQuery = useProjects();
  const queryClient = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();

  // TODO: Implement actual pinned chats capability
  const pinnedChats: any[] = []; // Stubbed for now
  const [pinnedExpanded, setPinnedExpanded] = useState(true);
  const [recentsExpanded, setRecentsExpanded] = useState(true);
  const [projectsExpanded, setProjectsExpanded] = useState(true);
  const [expandedProjects, setExpandedProjects] = useState<Set<string>>(new Set());

  const [editingChat, setEditingChat] = useState<any | null>(null);
  const [modalType, setModalType] = useState<'rename' | 'move' | null>(null);

  const toggleProject = useCallback((projectId: string) => {
    setExpandedProjects(prev => {
      const next = new Set(prev);
      if (next.has(projectId)) {
        next.delete(projectId);
      } else {
        next.add(projectId);
      }
      return next;
    });
  }, []);

  const handleMenuAction = async (action: string | number, chatId: string) => {
    if (action === 'archive') {
      const previousChats = queryClient.getQueryData<any[]>(['chats', undefined]);
      if (previousChats) {
        queryClient.setQueryData(['chats', undefined], previousChats.filter(c => c.chat_id !== chatId));
      }
      try {
        await archiveChat(chatId);
        queryClient.invalidateQueries({ queryKey: ['chats'] });
        queryClient.invalidateQueries({ queryKey: ['project-chats'] });
      } catch (e) {
        console.error('Failed to archive chat', e);
        if (previousChats) queryClient.setQueryData(['chats', undefined], previousChats);
        toast.error('Failed to archive chat');
      }
    } else if (action === 'delete') {
      const ok = await confirm({
        title: 'Delete chat?',
        description: 'This permanently removes the chat and its transcript. This cannot be undone.',
        confirmLabel: 'Delete',
        danger: true,
      });
      if (!ok) return;
      const previousChats = queryClient.getQueryData<any[]>(['chats', undefined]);
      if (previousChats) {
        queryClient.setQueryData(['chats', undefined], previousChats.filter(c => c.chat_id !== chatId));
      }
      try {
        await deleteChat(chatId);
        queryClient.invalidateQueries({ queryKey: ['chats'] });
        queryClient.invalidateQueries({ queryKey: ['project-chats'] });
        if (params.chatId === chatId) {
          navigate('/chats');
        }
      } catch (e) {
        console.error('Failed to delete chat', e);
        if (previousChats) queryClient.setQueryData(['chats', undefined], previousChats);
        toast.error('Failed to delete chat');
      }
    } else if (action === 'rename') {
      const chat = chatsQuery.data?.find(c => c.chat_id === chatId);
      if (chat) {
        setEditingChat(chat);
        setModalType('rename');
      }
    } else if (action === 'move') {
      const chat = chatsQuery.data?.find(c => c.chat_id === chatId);
      if (chat) {
        setEditingChat(chat);
        setModalType('move');
      }
    } else if (action === 'remove-project') {
      try {
        await updateChat(chatId, { project_id: null });
        queryClient.invalidateQueries({ queryKey: ['chats'] });
        queryClient.invalidateQueries({ queryKey: ['project-chats'] });
      } catch (e) {
        console.error('Failed to remove from project', e);
        toast.error('Failed to remove from project');
      }
    }
  };

  const handleNewChat = useCallback(() => {
    const projectId = params.projectId;
    const alreadyDraft = Boolean(
      isDraftChatId(params.chatId) ||
      (route === 'chats' && path === '/chats/new') ||
      (route === 'projects' && projectId && isOnDraftPath(path, projectId))
    );
    openNewChat(navigate, { projectId, replace: alreadyDraft });
  }, [navigate, params.chatId, params.projectId, path, route]);

  const handleNewProjectChat = useCallback((projectId: string, e: React.MouseEvent) => {
    e.stopPropagation();
    const alreadyDraft = Boolean(
      route === 'projects' && params.projectId === projectId && isDraftChatId(params.chatId)
    );
    openNewChat(navigate, { projectId, replace: alreadyDraft });
  }, [navigate, route, params.projectId, params.chatId]);

  const handleHome = useCallback(() => {
    navigate('/chats');
  }, [navigate]);

  const handleProfilePress = useCallback(() => {
    if (collapsed) {
      onExpand();
      return;
    }
    navigate('/chats');
  }, [collapsed, navigate, onExpand]);

  const navItem = (
    path: string,
    icon: ReactNode,
    label: string,
    activeRoute: string,
    matchExact: boolean = false
  ) => {
    const isActive = matchExact ? route === activeRoute : route.startsWith(activeRoute);

    const content = (
      <a 
        key={path}
        href={path}
        className={clsx(styles.navItem, isActive && styles.navItemActive)}
        onClick={(e) => {
          e.preventDefault();
          navigate(path);
        }}
      >
        <div className={styles.navIcon}>{icon}</div>
        {!collapsed && <span className={styles.navLabel}>{label}</span>}
      </a>
    );

    if (collapsed) {
      return (
        <TooltipTrigger key={path} delay={200}>
          {content}
          <Tooltip placement="right" className="navi-tooltip">
            {label}
          </Tooltip>
        </TooltipTrigger>
      );
    }

    return content;
  };

  const projects = projectsQuery.data?.items || [];

  return (
    <div className={clsx(styles.rail, collapsed && styles.railCollapsed)}>
      <div className={styles.brand}>
        <NaviProfileButton
          status={profileStatus}
          detail={profileDetail}
          actionLabel={collapsed ? 'Expand navigation panel' : 'Open NAVI overview'}
          onPress={handleProfilePress}
        />
        {!collapsed && (
          <Fragment key="expanded-brand">
            <a
              className={styles.brandHome}
              href="/chats"
              aria-label="NAVI Console overview"
              onClick={(e) => {
                e.preventDefault();
                handleHome();
              }}
            >
              <span className={styles.brandMark}>NAVI</span>
              <span className={styles.brandSub}>Console</span>
            </a>
            <Button
              className={styles.collapseBtn}
              onPress={onToggleCollapsed}
              aria-label="Collapse navigation panel"
            >
              <PanelLeftClose size={16} />
            </Button>
          </Fragment>
        )}
      </div>

      <div className={styles.primaryAction}>
        {collapsed ? (
          <TooltipTrigger key="new-chat-collapsed" delay={200}>
            <Button 
              className={clsx('navi-button navi-button-primary', styles.newChatBtnCollapsed)} 
              onPress={handleNewChat}
              aria-label="New Conversation"
            >
              <Plus size={18} />
            </Button>
            <Tooltip placement="right" className="navi-tooltip">
              New Conversation
            </Tooltip>
          </TooltipTrigger>
        ) : (
          <Button 
            className={clsx('navi-button navi-button-primary', styles.newChatBtn)} 
            onPress={handleNewChat}
          >
            <Plus size={16} />
            New Conversation
          </Button>
        )}
      </div>

      <div className={styles.navGroups}>
        {/* STANDALONE OVERVIEW */}
        <div className={styles.groupContent}>
          {navItem('/overview', <LayoutDashboard size={18} />, 'Overview', 'overview')}
          {navItem('/coder', <Code2 size={18} />, 'Coder', 'coder')}
        </div>

        {/* Chat */}
        <Disclosure defaultExpanded className={styles.disclosure}>
          <Button slot="trigger" className={clsx(styles.groupHeader, collapsed && styles.hidden)}>
            Chat
            <ChevronRight size={14} className={styles.groupChevron} />
          </Button>
          <DisclosurePanel className={styles.groupContent}>
            {navItem('/artifacts', <FileCode size={18} />, 'Artifacts', 'artifacts')}
            {collapsed && navItem('/chats', <MessageSquare size={18} />, 'Chats', 'chats', true)}
            {collapsed && navItem('/projects', <FolderKanban size={18} />, 'Projects', 'projects')}

            {/* ── Projects fold (replaces old Projects nav link) ── */}
            {!collapsed && projects.length > 0 && (
              <Disclosure
                isExpanded={projectsExpanded}
                onExpandedChange={setProjectsExpanded}
                className={styles.recentChatsDisclosure}
              >
                <div className={styles.recentChatsHeader}>
                  <Button slot="trigger" className={styles.recentChatsTrigger}>
                    <ChevronRight size={12} className={styles.recentChatsChevron} />
                    <span>Projects</span>
                  </Button>
                  {projectsExpanded && (
                    <a
                      href="/projects"
                      className={styles.viewAllBtn}
                      onClick={(e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        navigate('/projects');
                      }}
                    >
                      View all <ChevronRight size={10} style={{ marginLeft: '2px', flexShrink: 0 }} />
                    </a>
                  )}
                </div>
                <DisclosurePanel className={styles.recentChatsPanel}>
                  {projects.map((project) => (
                    <ProjectAccordionItem
                      key={project.project_id}
                      project={project}
                      isExpanded={expandedProjects.has(project.project_id)}
                      onToggle={() => toggleProject(project.project_id)}
                      isActiveProject={route === 'projects' && params.projectId === project.project_id}
                      activeChatId={params.chatId}
                      onChatClick={(chatId) => navigate(`/projects/${project.project_id}/chats/${chatId}`)}
                      onNewChat={(e) => handleNewProjectChat(project.project_id, e)}
                      onProjectClick={() => navigate(`/projects/${project.project_id}`)}
                      onChatMenuAction={handleMenuAction}
                    />
                  ))}
                </DisclosurePanel>
              </Disclosure>
            )}
            
            {/* Collapsible Pinned Chats sub-section */}
            {!collapsed && pinnedChats.length > 0 && (
              <Disclosure 
                isExpanded={pinnedExpanded} 
                onExpandedChange={setPinnedExpanded} 
                className={styles.recentChatsDisclosure}
              >
                <div className={styles.recentChatsHeader}>
                  <Button slot="trigger" className={styles.recentChatsTrigger}>
                    <ChevronRight size={12} className={styles.recentChatsChevron} />
                    <span>Pinned</span>
                  </Button>
                </div>
                <DisclosurePanel className={styles.recentChatsPanel}>
                  {pinnedChats.map((chat, index) => (
                    <div key={chat.chat_id || `pinned-chat-${index}`} className={styles.recentChatRow}>
                      <a
                        href={`/chats/${chat.chat_id}`}
                        className={styles.recentChatLink}
                        onClick={(e) => {
                          e.preventDefault();
                          navigate(`/chats/${chat.chat_id}`);
                        }}
                      >
                        <MessageSquare size={12} style={{ marginRight: '6px', opacity: 0.6, flexShrink: 0 }} />
                        <span className={styles.recentChatTitle}>{chat.title || 'Untitled Chat'}</span>
                      </a>
                      <MenuTrigger>
                        <Button aria-label="Chat options" className={`navi-button-icon ${styles.kebabBtn}`}>
                          <MoreVertical size={14} />
                        </Button>
                        <Popover className="navi-popover" placement="bottom end">
                          <Menu
                            onAction={(key) => handleMenuAction(key, chat.chat_id)}
                            style={{ outline: 'none' }}
                          >
                            <MenuItem className="navi-menu-item" id="rename">
                              <Edit size={14} /> Rename
                            </MenuItem>
                            <MenuItem className="navi-menu-item" id="move">
                              <FolderUp size={14} /> Move to project
                            </MenuItem>
                            {chat.project_id && (
                              <MenuItem className="navi-menu-item" id="remove-project">
                                <FolderMinus size={14} /> Remove from project
                              </MenuItem>
                            )}
                            <MenuItem className="navi-menu-item" id="archive">
                              <Archive size={14} /> Archive
                            </MenuItem>
                            <MenuItem className="navi-menu-item navi-menu-item-danger" id="delete">
                              <Trash2 size={14} /> Delete
                            </MenuItem>
                          </Menu>
                        </Popover>
                      </MenuTrigger>
                    </div>
                  ))}
                </DisclosurePanel>
              </Disclosure>
            )}

            {/* Collapsible Recent Chats sub-section */}
            {!collapsed && chatsQuery.data && chatsQuery.data.length > 0 && (
              <Disclosure 
                isExpanded={recentsExpanded} 
                onExpandedChange={setRecentsExpanded} 
                className={styles.recentChatsDisclosure}
              >
                <div className={styles.recentChatsHeader}>
                  <Button slot="trigger" className={styles.recentChatsTrigger}>
                    <ChevronRight size={12} className={styles.recentChatsChevron} />
                    <span>Recent</span>
                  </Button>
                  {recentsExpanded && (
                    <a 
                      href="/chats"
                      className={styles.viewAllBtn}
                      onClick={(e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        navigate('/chats');
                      }}
                    >
                      View all <ChevronRight size={10} style={{ marginLeft: '2px', flexShrink: 0 }} />
                    </a>
                  )}
                </div>
                <DisclosurePanel className={styles.recentChatsPanel}>
                  {chatsQuery.data.slice(0, 5).map((chat, index) => (
                    <div key={chat.chat_id || `recent-chat-${index}`} className={styles.recentChatRow}>
                      <a
                        href={`/chats/${chat.chat_id}`}
                        className={styles.recentChatLink}
                        onClick={(e) => {
                          e.preventDefault();
                          navigate(`/chats/${chat.chat_id}`);
                        }}
                      >
                        <MessageSquare size={12} style={{ marginRight: '6px', opacity: 0.6, flexShrink: 0 }} />
                        <span className={styles.recentChatTitle}>{chat.title || 'Untitled Chat'}</span>
                      </a>
                      <MenuTrigger>
                        <Button aria-label="Chat options" className={`navi-button-icon ${styles.kebabBtn}`}>
                          <MoreVertical size={14} />
                        </Button>
                        <Popover className="navi-popover" placement="bottom end">
                          <Menu
                            onAction={(key) => handleMenuAction(key, chat.chat_id)}
                            style={{ outline: 'none' }}
                          >
                            <MenuItem className="navi-menu-item" id="rename">
                              <Edit size={14} /> Rename
                            </MenuItem>
                            <MenuItem className="navi-menu-item" id="move">
                              <FolderUp size={14} /> Move to project
                            </MenuItem>
                            {chat.project_id && (
                              <MenuItem className="navi-menu-item" id="remove-project">
                                <FolderMinus size={14} /> Remove from project
                              </MenuItem>
                            )}
                            <MenuItem className="navi-menu-item" id="archive">
                              <Archive size={14} /> Archive
                            </MenuItem>
                            <MenuItem className="navi-menu-item navi-menu-item-danger" id="delete">
                              <Trash2 size={14} /> Delete
                            </MenuItem>
                          </Menu>
                        </Popover>
                      </MenuTrigger>
                    </div>
                  ))}
                </DisclosurePanel>
              </Disclosure>
            )}
          </DisclosurePanel>
        </Disclosure>

        {/* CONTROL */}
        <Disclosure defaultExpanded className={styles.disclosure}>
          <Button slot="trigger" className={clsx(styles.groupHeader, collapsed && styles.hidden)}>
            Control
            <ChevronRight size={14} className={styles.groupChevron} />
          </Button>
          <DisclosurePanel className={styles.groupContent}>
            {navItem('/workspaces', <Boxes size={18} />, 'Workspaces', 'workspaces')}
            {navItem('/runs', <Play size={18} />, 'Runs', 'runs')}
            {navItem('/proposals', <GitPullRequest size={18} />, 'Proposals', 'proposals')}
            {navItem('/usage', <Activity size={18} />, 'Usage', 'usage')}
            {navItem('/scheduler', <CalendarClock size={18} />, 'Scheduler', 'scheduler')}
            {navItem('/plugins/skills', <Puzzle size={18} />, 'Plugins', 'plugins')}
          </DisclosurePanel>
        </Disclosure>

        {/* SYSTEM */}
        <Disclosure defaultExpanded className={styles.disclosure}>
          <Button slot="trigger" className={clsx(styles.groupHeader, collapsed && styles.hidden)}>
            System
            <ChevronRight size={14} className={styles.groupChevron} />
          </Button>
          <DisclosurePanel className={styles.groupContent}>
            {navItem('/docs', <BookOpen size={18} />, 'Docs', 'docs')}
            {navItem('/settings', <Settings size={18} />, 'Settings', 'settings')}
            {navItem('/debug', <Bug size={18} />, 'Debug', 'debug')}
          </DisclosurePanel>
        </Disclosure>
      </div>

      {modalType === 'rename' && (
        <RenameChatModal
          chat={editingChat}
          isOpen={true}
          onClose={() => { setModalType(null); setEditingChat(null); }}
        />
      )}
      {modalType === 'move' && (
        <MoveProjectModal
          chat={editingChat}
          isOpen={true}
          onClose={() => { setModalType(null); setEditingChat(null); }}
        />
      )}
    </div>
  );
}

/* ── Project Accordion Item ── */

interface ProjectAccordionItemProps {
  project: Project;
  isExpanded: boolean;
  onToggle: () => void;
  isActiveProject: boolean;
  activeChatId?: string;
  onChatClick: (chatId: string) => void;
  onNewChat: (e: React.MouseEvent) => void;
  onProjectClick: () => void;
  onChatMenuAction: (action: string | number, chatId: string) => void;
}

function ProjectAccordionItem({
  project,
  isExpanded,
  onToggle,
  isActiveProject,
  activeChatId,
  onChatClick,
  onNewChat,
  onProjectClick,
  onChatMenuAction,
}: ProjectAccordionItemProps) {
  const IconComp = ICON_MAP[project.icon || 'folder'] || FolderKanban;

  return (
    <div className={styles.projectEntry}>
      <div
        className={clsx(styles.projectRow, isActiveProject && styles.projectRowActive)}
        onClick={onToggle}
        role="button"
        tabIndex={0}
        aria-expanded={isExpanded}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            onToggle();
          }
        }}
      >
        <ChevronRight
          size={12}
          className={clsx(styles.projectChevron, isExpanded && styles.projectChevronExpanded)}
        />
        <IconComp
          size={16}
          className={styles.projectIcon}
          style={{ color: project.color || 'var(--navi-text-secondary)' }}
        />
        <span className={styles.projectName}>{project.title}</span>
        <Button
          aria-label="New chat in project"
          className={`navi-button-icon ${styles.projectActionBtn}`}
          onPress={() => {
            // React Aria onPress doesn't have stopPropagation on the native event,
            // but the button intercepts click before it reaches the parent div.
          }}
          // Use native click handler for stopPropagation
        >
          <span onClick={onNewChat}><Plus size={14} /></span>
        </Button>
        <MenuTrigger>
          <Button
            aria-label="Project options"
            className={`navi-button-icon ${styles.projectActionBtn}`}
          >
            <MoreVertical size={14} />
          </Button>
          <Popover className="navi-popover" placement="bottom end">
            <Menu
              onAction={(key) => {
                if (key === 'open') onProjectClick();
              }}
              style={{ outline: 'none' }}
            >
              <MenuItem className="navi-menu-item" id="open">
                <FolderKanban size={14} /> Open project
              </MenuItem>
            </Menu>
          </Popover>
        </MenuTrigger>
      </div>
      {isExpanded && (
        <ProjectChatsList
          projectId={project.project_id}
          activeChatId={activeChatId}
          isActiveProject={isActiveProject}
          onChatClick={onChatClick}
          onChatMenuAction={onChatMenuAction}
        />
      )}
    </div>
  );
}

/* ── Project Chats List (lazy-loaded per project) ── */

interface ProjectChatsListProps {
  projectId: string;
  activeChatId?: string;
  isActiveProject: boolean;
  onChatClick: (chatId: string) => void;
  onChatMenuAction: (action: string | number, chatId: string) => void;
}

function ProjectChatsList({
  projectId,
  activeChatId,
  isActiveProject,
  onChatClick,
  onChatMenuAction,
}: ProjectChatsListProps) {
  const { data: chatsData, isLoading } = useProjectChats(projectId);
  const chats = chatsData?.items || [];

  if (isLoading) {
    return (
      <div className={styles.projectChildren}>
        <span className={styles.projectChatsLoading}>Loading…</span>
      </div>
    );
  }

  if (chats.length === 0) {
    return (
      <div className={styles.projectChildren}>
        <span className={styles.projectChatsEmpty}>No chats yet</span>
      </div>
    );
  }

  return (
    <div className={styles.projectChildren}>
      {chats.map((chat, index) => (
        <div
          key={chat.chat_id || `proj-chat-${index}`}
          className={clsx(
            styles.projectChatRow,
            isActiveProject && activeChatId === chat.chat_id && styles.projectChatActive,
          )}
        >
          <a
            href={`/projects/${projectId}/chats/${chat.chat_id}`}
            className={styles.projectChatLink}
            onClick={(e) => {
              e.preventDefault();
              onChatClick(chat.chat_id);
            }}
          >
            <MessageSquare size={12} style={{ marginRight: '6px', opacity: 0.6, flexShrink: 0 }} />
            <span className={styles.recentChatTitle}>{chat.title || 'Untitled Chat'}</span>
          </a>
          <MenuTrigger>
            <Button aria-label="Chat options" className={`navi-button-icon ${styles.kebabBtn}`}>
              <MoreVertical size={14} />
            </Button>
            <Popover className="navi-popover" placement="bottom end">
              <Menu
                onAction={(key) => onChatMenuAction(key, chat.chat_id)}
                style={{ outline: 'none' }}
              >
                <MenuItem className="navi-menu-item" id="rename">
                  <Edit size={14} /> Rename
                </MenuItem>
                <MenuItem className="navi-menu-item" id="move">
                  <FolderUp size={14} /> Move to project
                </MenuItem>
                <MenuItem className="navi-menu-item" id="remove-project">
                  <FolderMinus size={14} /> Remove from project
                </MenuItem>
                <MenuItem className="navi-menu-item" id="archive">
                  <Archive size={14} /> Archive
                </MenuItem>
                <MenuItem className="navi-menu-item navi-menu-item-danger" id="delete">
                  <Trash2 size={14} /> Delete
                </MenuItem>
              </Menu>
            </Popover>
          </MenuTrigger>
        </div>
      ))}
    </div>
  );
}
