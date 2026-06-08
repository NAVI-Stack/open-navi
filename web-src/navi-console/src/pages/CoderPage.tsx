import type { ReactNode } from 'react';
import {
  Braces,
  CheckCircle2,
  Code2,
  FileCode,
  FolderKanban,
  GitBranch,
  GitPullRequest,
  ListChecks,
  MessageSquare,
  PlayCircle,
  Settings,
  TerminalSquare,
} from 'lucide-react';
import clsx from 'clsx';
import { useNavigate } from '@/app/router';
import { EmptyState } from '@/components/ui/EmptyState';
import styles from './CoderPage.module.css';

type CoderSection = 'workspace' | 'projects' | 'repos' | 'tasks' | 'runs' | 'reviews' | 'settings';

interface CoderPageProps {
  section?: string;
}

const sections: Array<{ key: CoderSection; label: string; href: string; icon: ReactNode }> = [
  { key: 'workspace', label: 'Workspace', href: '/coder', icon: <Code2 size={15} /> },
  { key: 'projects', label: 'Projects', href: '/coder/projects', icon: <FolderKanban size={15} /> },
  { key: 'repos', label: 'Repositories', href: '/coder/repos', icon: <GitBranch size={15} /> },
  { key: 'tasks', label: 'Tasks', href: '/coder/tasks', icon: <ListChecks size={15} /> },
  { key: 'runs', label: 'Runs', href: '/coder/runs', icon: <PlayCircle size={15} /> },
  { key: 'reviews', label: 'Reviews', href: '/coder/reviews', icon: <GitPullRequest size={15} /> },
  { key: 'settings', label: 'Settings', href: '/coder/settings', icon: <Settings size={15} /> },
];

export function CoderPage({ section }: CoderPageProps) {
  const navigate = useNavigate();
  const activeSection = normalizeSection(section);

  const handleNavigate = (event: React.MouseEvent<HTMLAnchorElement>, href: string) => {
    event.preventDefault();
    navigate(href);
  };

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div>
          <h1 className={styles.title}>NAVI Coder</h1>
          <p className={styles.subtitle}>
            Coding workspace for repository context, task runs, validation evidence, diffs, and review handoff.
          </p>
        </div>
        <div className={styles.headerBadges} aria-label="Coder readiness">
          <span className={styles.badge}>Workspace shell</span>
          <span className={styles.badgeMuted}>API binding pending</span>
        </div>
      </header>

      <nav className={styles.sectionNav} aria-label="Coder sections">
        {sections.map((item) => (
          <a
            key={item.key}
            href={item.href}
            className={clsx(styles.sectionLink, activeSection === item.key && styles.sectionLinkActive)}
            aria-current={activeSection === item.key ? 'page' : undefined}
            onClick={(event) => handleNavigate(event, item.href)}
          >
            {item.icon}
            <span>{item.label}</span>
          </a>
        ))}
      </nav>

      <div className={styles.workspaceGrid}>
        <aside className={styles.contextPanel} aria-label="Coder project and repository context">
          <PanelHeader icon={<FolderKanban size={16} />} title="Project Context" />
          <EmptyState
            icon={<FolderKanban size={22} />}
            title={contextTitle(activeSection)}
            description={contextDescription(activeSection)}
          />
        </aside>

        <main className={styles.workbenchPanel} aria-label="Coder workbench">
          <div className={styles.workbenchHeader}>
            <div>
              <span className={styles.kicker}>{sectionLabel(activeSection)}</span>
              <h2 className={styles.panelTitle}>{workbenchTitle(activeSection)}</h2>
            </div>
            <span className={styles.statusPill}>Empty</span>
          </div>

          <div className={styles.pipeline}>
            <PipelineStep icon={<ListChecks size={15} />} title="Task" text="Brief, scope, and acceptance target." />
            <PipelineStep icon={<Braces size={15} />} title="Plan" text="Implementation steps and workspace binding." />
            <PipelineStep icon={<FileCode size={15} />} title="Diff" text="Changed files and reviewable patch." />
            <PipelineStep icon={<CheckCircle2 size={15} />} title="Validation" text="Build, test, lint, and run evidence." />
          </div>

          <div className={styles.emptyWorkbench}>
            <EmptyState
              icon={emptyIcon(activeSection)}
              title={emptyTitle(activeSection)}
              description={emptyDescription(activeSection)}
            />
          </div>
        </main>

        <aside className={styles.reviewPanel} aria-label="Coder conversation and review">
          <PanelHeader icon={<MessageSquare size={16} />} title="Review Handoff" />
          <div className={styles.reviewStack}>
            <PlaceholderRow icon={<TerminalSquare size={15} />} title="Run log" text="Execution output will stream here once Coder runs are connected." />
            <PlaceholderRow icon={<GitPullRequest size={15} />} title="Review state" text="Review decisions, branch status, and promotion controls will attach to Coder runs." />
            <PlaceholderRow icon={<MessageSquare size={15} />} title="Conversation" text="Task-specific discussion can live beside the code and validation panels." />
          </div>
        </aside>
      </div>
    </div>
  );
}

function normalizeSection(section?: string): CoderSection {
  if (section === 'projects' || section === 'repos' || section === 'tasks' || section === 'runs' || section === 'reviews' || section === 'settings') {
    return section;
  }
  return 'workspace';
}

function sectionLabel(section: CoderSection): string {
  return sections.find((item) => item.key === section)?.label ?? 'Workspace';
}

function contextTitle(section: CoderSection): string {
  if (section === 'repos') return 'No repositories connected';
  if (section === 'projects') return 'No Coder projects yet';
  return 'No active Coder context';
}

function contextDescription(section: CoderSection): string {
  if (section === 'repos') return 'Approved repositories will appear here after backend Coder repository APIs are connected.';
  if (section === 'projects') return 'Coder projects will anchor repositories, tasks, runs, reviews, and validation evidence.';
  return 'Select a Coder project or repository once the Coder domain APIs are available.';
}

function workbenchTitle(section: CoderSection): string {
  switch (section) {
    case 'projects':
      return 'Project workspace';
    case 'repos':
      return 'Repository workspace';
    case 'tasks':
      return 'Task queue';
    case 'runs':
      return 'Run timeline';
    case 'reviews':
      return 'Review queue';
    case 'settings':
      return 'Coder settings';
    default:
      return 'Workspace overview';
  }
}

function emptyTitle(section: CoderSection): string {
  switch (section) {
    case 'projects':
      return 'No Coder projects to show';
    case 'repos':
      return 'No repositories to show';
    case 'tasks':
      return 'No coding tasks queued';
    case 'runs':
      return 'No Coder runs yet';
    case 'reviews':
      return 'No reviews awaiting handoff';
    case 'settings':
      return 'Coder settings are not connected yet';
    default:
      return 'Coder workspace is ready for backend binding';
  }
}

function emptyDescription(section: CoderSection): string {
  switch (section) {
    case 'projects':
      return 'Project cards will appear here when NAVI Coder project APIs are wired.';
    case 'repos':
      return 'Repository bindings, branches, and workspace scope will appear here without using plugin-manager UI.';
    case 'tasks':
      return 'Task briefs, acceptance criteria, and planning state will appear here when task APIs are available.';
    case 'runs':
      return 'Run status, steps, logs, and validation results will appear here as Coder runs execute.';
    case 'reviews':
      return 'Diff review, validation evidence, and pull request handoff state will appear here.';
    case 'settings':
      return 'Coder-specific repository, validation, review, and governance settings will be configured here later.';
    default:
      return 'This shell reserves space for project context, task runs, diffs, validation output, and review handoff.';
  }
}

function emptyIcon(section: CoderSection): ReactNode {
  switch (section) {
    case 'projects':
      return <FolderKanban size={24} />;
    case 'repos':
      return <GitBranch size={24} />;
    case 'tasks':
      return <ListChecks size={24} />;
    case 'runs':
      return <PlayCircle size={24} />;
    case 'reviews':
      return <GitPullRequest size={24} />;
    case 'settings':
      return <Settings size={24} />;
    default:
      return <Code2 size={24} />;
  }
}

function PanelHeader({ icon, title }: { icon: ReactNode; title: string }) {
  return (
    <div className={styles.panelHeader}>
      <span className={styles.panelHeaderIcon}>{icon}</span>
      <h2>{title}</h2>
    </div>
  );
}

function PipelineStep({ icon, title, text }: { icon: ReactNode; title: string; text: string }) {
  return (
    <div className={styles.pipelineStep}>
      <span className={styles.pipelineIcon}>{icon}</span>
      <div>
        <strong>{title}</strong>
        <span>{text}</span>
      </div>
    </div>
  );
}

function PlaceholderRow({ icon, title, text }: { icon: ReactNode; title: string; text: string }) {
  return (
    <div className={styles.placeholderRow}>
      <span className={styles.placeholderIcon}>{icon}</span>
      <div>
        <strong>{title}</strong>
        <span>{text}</span>
      </div>
    </div>
  );
}
