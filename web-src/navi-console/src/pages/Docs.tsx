import { 
  Book, 
  BookOpen, 
  Terminal, 
  Copy, 
  Check, 
  Info,
  LayoutDashboard,
  MessageSquare,
  FolderOpen,
  Puzzle,
  GitPullRequest,
  Gauge,
  Calendar,
  Settings,
  Palette,
  ScrollText,
  Bug,
  ShieldAlert,
  Zap,
  Globe,
  Rocket
} from 'lucide-react';
import { useState } from 'react';
import { JsonPanel } from '@/components/JsonPanel';
import { StatusCard } from '@/components/StatusCard';
import { useSystemStatus, useOperatorOverview } from '@/api/status';
import styles from './Docs.module.css';

const REPO_DOCS = [
  { path: 'docs/design/navi-console.md', label: 'Console Design' },
  { path: 'docs/specs/navi-console-v1.md', label: 'Console Spec V1' },
  { path: 'docs/adr/ADR-010-navi-console-gateway-hosting.md', label: 'ADR: Console Hosting' },
  { path: 'docs/specs/gateway-api.md', label: 'Gateway API Spec' },
  { path: 'docs/specs/configuration.md', label: 'Configuration Spec' },
  { path: 'docs/canonical/skills.md', label: 'Skills Canonical' },
  { path: 'docs/canonical/conceptual-design-overview.md', label: 'Conceptual Overview' },
];

const PAGE_CARDS = [
  { id: 'overview', title: 'Overview', icon: LayoutDashboard, desc: 'Live runtime dashboard showing system health, agent status, and active chat metrics.' },
  { id: 'chat', title: 'Chat', icon: MessageSquare, desc: 'Direct operator messaging interface for interacting with the agent in real-time.' },
  { id: 'chats', title: 'Chats', icon: FolderOpen, desc: 'Inventory of chats with detailed inspection of runs and runtime state.' },
  { id: 'capabilities', title: 'Capabilities', icon: Puzzle, desc: 'Detailed inspection of plugins, skills, tools, and connectors available to the system.' },
  { id: 'proposals', title: 'Proposals', icon: GitPullRequest, desc: 'Governance approval queue for risky or high-stakes agent actions.' },
  { id: 'usage', title: 'Usage', icon: Gauge, desc: 'Operational summary of token consumption, cost estimates, and tool usage patterns.' },
  { id: 'scheduler', title: 'Scheduler', icon: Calendar, desc: 'Inspection of background jobs, cron triggers, and scheduler readiness.' },
  { id: 'docs', title: 'Docs', icon: BookOpen, desc: 'Static repository documentation references and known Console gaps.' },
  { id: 'config', title: 'Config', icon: Settings, desc: 'Identity, LLM provider, and experience layer configuration inspection.' },
  { id: 'appearance', title: 'Appearance', icon: Palette, desc: 'User interface preferences, theme selection, and owner identity branding.' },
  { id: 'logs', title: 'Logs', icon: ScrollText, desc: 'Unified live event feed showing activity, errors, and system traces.' },
  { id: 'debug', title: 'Debug', icon: Bug, desc: 'Raw diagnostic signals, runtime metrics, and advanced system event streams.' },
];

function errorMsg(err: Error | null): string | null {
  return err ? err.message : null;
}

export function Docs() {
  const systemStatus = useSystemStatus();
  const operatorOverview = useOperatorOverview();
  const [copiedPath, setCopiedPath] = useState<string | null>(null);

  const copyToClipboard = (path: string) => {
    navigator.clipboard.writeText(path);
    setCopiedPath(path);
    setTimeout(() => setCopiedPath(null), 2000);
  };

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <h2 className={styles.heading}>Docs</h2>
        <p className={styles.subheading}>Understand the NAVI Console architecture and runtime environment.</p>
      </header>

      {/* Guide Section */}
      <section className={styles.section}>
        <h3 className={styles.sectionTitle}><Book size={20} /> Console Guide</h3>
        <div className={styles.grid}>
          {PAGE_CARDS.map(card => (
            <div key={card.id} className={styles.card}>
              <div className={styles.cardHeader}>
                <card.icon size={20} />
                <span className={styles.cardTitle}>{card.title}</span>
              </div>
              <p className={styles.cardDescription}>{card.desc}</p>
            </div>
          ))}
        </div>
      </section>

      {/* Architecture Section */}
      <section className={styles.section}>
        <h3 className={styles.sectionTitle}><Globe size={20} /> Runtime / Gateway</h3>
        <div className={styles.grid}>
          <div className={styles.card}>
            <div className={styles.cardHeader}>
              <Rocket size={20} />
              <span className={styles.cardTitle}>Gateway Hosting</span>
            </div>
            <p className={styles.cardDescription}>
              The Console is a static React application served directly by the NAVI Gateway. 
              It communicates with the gateway via same-origin REST and WebSocket APIs.
            </p>
          </div>
          <div className={styles.card}>
            <div className={styles.cardHeader}>
              <Info size={20} />
              <span className={styles.cardTitle}>Onboarding</span>
            </div>
            <p className={styles.cardDescription}>
              First-run experiences are managed by the gateway. If no identity exists, users are 
              redirected to the onboarding flow before accessing the main console surfaces.
            </p>
          </div>
          <div className={styles.card}>
            <div className={styles.cardHeader}>
              <ShieldAlert size={20} />
              <span className={styles.cardTitle}>Governance</span>
            </div>
            <p className={styles.cardDescription}>
              Proposals are generated by the agent when it encounters "risky" fields or actions. 
              The console provides a secure surface for operators to approve or deny these requests.
            </p>
          </div>
        </div>
      </section>

      {/* Repo Docs Section */}
      <section className={styles.section}>
        <h3 className={styles.sectionTitle}><BookOpen size={20} /> Repository Documentation</h3>
        <p className={styles.cardDescription}>
          The following files contain detailed specifications and ADRs. Live rendering is not yet 
          implemented; please reference these paths in your IDE.
        </p>
        <div className={styles.docList}>
          {REPO_DOCS.map(doc => (
            <div key={doc.path} className={styles.docItem}>
              <div className={styles.docInfo}>
                <span className={styles.docPath}>{doc.path}</span>
                <span className={styles.docLabel}>{doc.label}</span>
              </div>
              <button 
                className={styles.copyButton} 
                onClick={() => copyToClipboard(doc.path)}
                title="Copy path"
              >
                {copiedPath === doc.path ? <Check size={14} /> : <Copy size={14} />}
              </button>
            </div>
          ))}
        </div>
      </section>

      {/* Known Gaps Section */}
      <section className={styles.section}>
        <h3 className={styles.sectionTitle}><Zap size={20} /> Known Gaps</h3>
        <div className={styles.gapPanel}>
          <p className={styles.cardDescription}>
            The NAVI Ecosystem is evolving rapidly. The following features are currently out of scope 
            or awaiting backend implementation:
          </p>
          <ul className={styles.gapList}>
            <li className={styles.gapItem}><strong>Scheduler CRUD:</strong> Direct task management APIs are absent; view-only mode enabled.</li>
            <li className={styles.gapItem}><strong>Live Docs:</strong> Markdown rendering of repo files is not implemented in the console.</li>
            <li className={styles.gapItem}><strong>Usage Accuracy:</strong> Cost and token tracking depend on backend field availability.</li>
            <li className={styles.gapItem}><strong>Graph Resilience:</strong> Capability graph may be unavailable during intensive background expansions.</li>
            <li className={styles.gapItem}><strong>Public Hosting:</strong> Non-loopback console hosting is currently unsupported for security.</li>
          </ul>
        </div>
      </section>

      {/* Raw Inspection Section */}
      <section className={styles.inspection}>
        <h3 className={styles.sectionTitle}><Terminal size={20} /> Raw Inspection</h3>
        <div className={styles.inspectionGrid}>
          <StatusCard title="System Status (/api/status)" loading={systemStatus.isLoading} error={errorMsg(systemStatus.error)}>
            <JsonPanel data={systemStatus.data} defaultExpanded={false} />
          </StatusCard>
          <StatusCard title="Operator Overview (/api/operator/overview)" loading={operatorOverview.isLoading} error={errorMsg(operatorOverview.error)}>
            <JsonPanel data={operatorOverview.data} defaultExpanded={false} />
          </StatusCard>
        </div>
      </section>

      <footer style={{ marginTop: '2rem', textAlign: 'center', opacity: 0.5 }}>
        <p style={{ fontSize: '0.75rem' }}>NAVI Console Runtime Docs &bull; V1.0.0</p>
      </footer>
    </div>
  );
}
