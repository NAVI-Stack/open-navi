import {
  useOperatorOverview,
  usePresence,
  useErrorSummary,
  useSystemStatus,
  useAgentStatus,
  useLlmActive,
} from '@/api/status';
import { useNavigate } from '@/app/router';
import { useChats, useChatThread } from '@/api/chats';
import { useSkills } from '@/api/extensions';
import { useRuntimeMetrics } from '@/api/debug';
import { useLiveEventsContext } from '@/hooks/LiveEventsContext';
import { StatusCard } from '@/components/StatusCard';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { JsonPanel } from '@/components/JsonPanel';
import { EmptyState } from '@/components/ui/EmptyState';
import { NaviApiError } from '@/api/errors';
import { 
  Radio, 
  AlertCircle, 
  GitPullRequest, 
  Activity, 
  List, 
  Zap,
  ExternalLink,
  Cpu,
  MessageSquare,
  Clock,
  Settings,
  ShieldCheck,
  Terminal,
  Signal
} from 'lucide-react';
import styles from './Overview.module.css';

function errorMsg(err: Error | null): string | null {
  if (!err) return null;
  if (err instanceof NaviApiError) return `[${err.status}] ${err.message}`;
  return err.message;
}

export function Overview() {
  const navigate = useNavigate();
  // Independent hooks for resilience
  const overview = useOperatorOverview();
  const systemStatus = useSystemStatus();
  const agentStatus = useAgentStatus();
  const llmActive = useLlmActive();
  const presence = usePresence();
  const errors = useErrorSummary();
  const chats = useChats();
  const skills = useSkills();
  const runtimeMetrics = useRuntimeMetrics();
  const { events, connectionState } = useLiveEventsContext();

  const activeChatId = chats.data?.[0]?.chat_id;
  const activeChat = useChatThread(activeChatId ?? null);

  const lastEvent = events.length > 0 ? events[0] : null;

  return (
    <div className={styles.page}>
      <h2 className={styles.heading}>Overview</h2>
      <div className={styles.grid}>
        {/* Row 1: Connection & Active Context */}
        <StatusCard title="Live Event Connection" loading={false} icon={<Signal size={16} />}>
          <div className={styles.rows}>
            <Row label="State">
              <StatusBadge 
                label={connectionState} 
                variant={
                  connectionState === 'connected' ? 'success' : 
                  connectionState === 'waiting' || connectionState === 'connecting' ? 'muted' : 
                  connectionState === 'reconnecting' ? 'warning' : 'danger'
                } 
              />
            </Row>
            <Row label="Events" value={String(events.length)} />
            {lastEvent && (
              <Row label="Last Event" value={new Date(lastEvent.receivedAt).toLocaleTimeString()} />
            )}
          </div>
        </StatusCard>

        <StatusCard 
          title="Active Chat" 
          loading={chats.isLoading || activeChat.isLoading} 
          error={errorMsg(activeChat.error)}
          headerAction={activeChatId ? <a href="/chats" className={styles.cardLink} onClick={(e) => { e.preventDefault(); navigate(`/chats/${activeChatId}`); }}><ExternalLink size={14} /></a> : undefined}
          icon={<MessageSquare size={16} />}
        >
          {activeChat.data ? (
            <div className={styles.rows}>
              <Row label="ID" value={activeChat.data.chat_id} />
              <Row label="Title" value={activeChat.data.title || 'Untitled'} />
              <Row label="Status">
                <StatusBadge label={activeChat.data.status || 'unknown'} variant="accent" size="sm" />
              </Row>
            </div>
          ) : !activeChat.isLoading && (
            <EmptyState icon={<MessageSquare size={20} />} message="No active chat" />
          )}
        </StatusCard>

        <StatusCard title="LLM Status" loading={llmActive.isLoading} error={errorMsg(llmActive.error)} icon={<Cpu size={16} />}>
          {llmActive.data ? (
            <div className={styles.rows}>
              <Row label="Provider" value={llmActive.data.provider || 'none'} />
              <Row label="Model" value={llmActive.data.model || 'none'} />
              <Row label="Status">
                <StatusBadge 
                  label={llmActive.data.status || 'inactive'} 
                  variant={llmActive.data.status === 'active' || llmActive.data.status === 'ready' ? 'success' : 'muted'} 
                />
              </Row>
            </div>
          ) : !llmActive.isLoading && (
            <EmptyState icon={<Cpu size={20} />} message="LLM not configured" />
          )}
        </StatusCard>

        {/* Row 2: Core System & Governance */}
        <StatusCard title="System Status" loading={systemStatus.isLoading} error={errorMsg(systemStatus.error)} icon={<Settings size={16} />}>
          {systemStatus.data && (
            <div className={styles.rows}>
              <Row label="Gateway" value={systemStatus.data.gateway.version ?? 'unknown'} />
              <Row label="Setup" value={systemStatus.data.setup.complete ? 'Complete' : 'Incomplete'} />
              <Row label="Connectors">
                <span className={styles.connCounts}>
                  <span>{systemStatus.data.connectors.running} running</span>
                  {systemStatus.data.connectors.degraded > 0 && (
                    <StatusBadge label={`${systemStatus.data.connectors.degraded} degraded`} variant="warning" />
                  )}
                  {systemStatus.data.connectors.error > 0 && (
                    <StatusBadge label={`${systemStatus.data.connectors.error} error`} variant="danger" />
                  )}
                </span>
              </Row>
            </div>
          )}
        </StatusCard>

        <StatusCard title="Agent Status" loading={agentStatus.isLoading} error={errorMsg(agentStatus.error)} icon={<Radio size={16} />}>
          {agentStatus.data && (
            <div className={styles.rows}>
              <Row label="State">
                <StatusBadge 
                  label={agentStatus.data.state} 
                  size="md" 
                  variant={agentStatus.data.state === 'running' ? 'accent' : 'muted'} 
                />
              </Row>
              {agentStatus.data.uptime_since && (
                <Row label="Uptime" value={new Date(agentStatus.data.uptime_since).toLocaleString()} />
              )}
              {agentStatus.data.turns_processed !== undefined && (
                <Row label="Turns" value={String(agentStatus.data.turns_processed)} />
              )}
              {agentStatus.data.current_detail && (
                <Row label="Detail" value={agentStatus.data.current_detail} />
              )}
            </div>
          )}
        </StatusCard>

        <StatusCard title="Governor" loading={systemStatus.isLoading} error={errorMsg(systemStatus.error)} icon={<ShieldCheck size={16} />}>
          {systemStatus.data?.governor && (
            <div className={styles.rows}>
              <Row label="Status">
                {systemStatus.data.governor.tripped
                  ? <StatusBadge label="Tripped" variant="danger" />
                  : <StatusBadge label="Operational" variant="success" />}
              </Row>
              <Row label="Budget">
                <span className={styles.mono}>
                  {systemStatus.data.governor.budget_used} / {systemStatus.data.governor.budget_max}
                </span>
              </Row>
              <Row label="Cost">
                <span className={styles.mono}>
                  ${systemStatus.data.governor.cost_used.toFixed(2)} / ${systemStatus.data.governor.cost_ceiling.toFixed(2)}
                </span>
              </Row>
            </div>
          )}
        </StatusCard>

        {/* Row 3: Operator Queue & History */}
        <StatusCard 
          title="Recent Chats" 
          loading={chats.isLoading} 
          error={errorMsg(chats.error)}
          headerAction={<a href="/chats" className={styles.cardLink} onClick={(e) => { e.preventDefault(); navigate('/chats'); }}><ExternalLink size={14} /></a>}
          icon={<Clock size={16} />}
        >
          {chats.data && chats.data.length > 0 ? (
            <div className={styles.list}>
              {chats.data.slice(0, 5).map((s) => (
                <div key={s.chat_id || s.id} className={styles.listItem}>
                  <div className={styles.listItemMain}>
                    <span className={styles.listItemTitle}>{s.title || 'Untitled'}</span>
                    <span className={styles.listItemSub}>{s.chat_id || s.id}</span>
                  </div>
                  <div className={styles.listItemSide}>
                    <StatusBadge label={s.status || 'unknown'} size="sm" variant="muted" />
                    {s.updated_at && <span className={styles.listItemTime}>{new Date(s.updated_at).toLocaleTimeString()}</span>}
                  </div>
                </div>
              ))}
            </div>
          ) : !chats.isLoading && (
            <EmptyState icon={<List size={20} />} message="No recent chats" />
          )}
        </StatusCard>

        <StatusCard 
          title="Pending Proposals" 
          loading={overview.isLoading} 
          error={errorMsg(overview.error)}
          headerAction={<a href="/proposals" className={styles.cardLink} onClick={(e) => { e.preventDefault(); navigate('/proposals'); }}><ExternalLink size={14} /></a>}
          icon={<GitPullRequest size={16} />}
        >
          {overview.data?.proposals?.items && overview.data.proposals.items.length > 0 ? (
            <div className={styles.proposalList}>
              <div className={styles.proposalCount}>
                <GitPullRequest size={14} />
                <span>{overview.data.proposals.pending_count} pending</span>
              </div>
              {overview.data.proposals.items.slice(0, 5).map((p) => (
                <div key={p.id || p.proposal_id} className={styles.proposalRow}>
                  <span className={styles.proposalAction}>{p.proposed_action || 'Unnamed Proposal'}</span>
                  {p.priority && <StatusBadge label={p.priority} size="sm" variant={p.priority === 'blocking' ? 'danger' : 'muted'} />}
                </div>
              ))}
            </div>
          ) : !overview.isLoading && (
            <EmptyState icon={<GitPullRequest size={20} />} message="No pending proposals" />
          )}
        </StatusCard>

        <StatusCard title="Capabilities Summary" loading={skills.isLoading} error={errorMsg(skills.error)} icon={<Zap size={16} />}>
          {skills.data?.items ? (
             <div className={styles.rows}>
               <Row label="Total" value={String(skills.data.items.length)} />
               <Row label="Enabled" value={String(skills.data.items.filter(s => s.enabled).length)} />
               <JsonPanel data={skills.data} label="Raw Skills" defaultExpanded={false} />
             </div>
          ) : !skills.isLoading && (
            <EmptyState icon={<Zap size={20} />} message="No skills registered" />
          )}
        </StatusCard>

        {/* Row 4: Live Activity & Presence */}
        <StatusCard 
          title="Live Activity" 
          loading={overview.isLoading} 
          error={errorMsg(overview.error)} 
          className={styles.wide}
          headerAction={<a href="/debug" className={styles.cardLink} onClick={(e) => { e.preventDefault(); navigate('/debug'); }}><ExternalLink size={14} /></a>}
          icon={<Activity size={16} />}
        >
          {overview.data?.activity?.items && overview.data.activity.items.length > 0 ? (
            <div className={styles.activityFeed}>
              {overview.data.activity.items.slice(0, 8).map((item, i) => (
                <div key={i} className={styles.activityRow}>
                   <span className={styles.activityTime}>{new Date(item.at).toLocaleTimeString()}</span>
                   <span className={styles.activityType} data-type={item.type}>{item.type}</span>
                   <span className={styles.activitySummary}>{item.summary}</span>
                </div>
              ))}
            </div>
          ) : !overview.isLoading && (
            <EmptyState icon={<Activity size={20} />} message="No recent activity" />
          )}
        </StatusCard>

        <StatusCard title="Presence" loading={presence.isLoading} error={errorMsg(presence.error)} icon={<Radio size={16} />}>
          {presence.data && Object.keys(presence.data).length > 0 ? (
            <div className={styles.rows}>
              <Row label="Entities" value={String(Object.keys(presence.data).length)} />
              <JsonPanel data={presence.data} label="Presence Registry" />
            </div>
          ) : !presence.isLoading && (
            <EmptyState icon={<Radio size={20} />} message="No presence data" />
          )}
        </StatusCard>

        <StatusCard 
          title="Errors (24h)" 
          loading={errors.isLoading} 
          error={errorMsg(errors.error)}
          icon={<AlertCircle size={16} />}
        >
          {errors.data?.items && errors.data.items.length > 0 ? (
            <div className={styles.errorList}>
              {errors.data.items.slice(0, 5).map((item, i) => (
                <div key={i} className={styles.errorRow}>
                  <span className={styles.errorType}>{item.type ?? item.component ?? 'unknown'}</span>
                  <span className={styles.errorCount}>{item.count}</span>
                </div>
              ))}
            </div>
          ) : !errors.isLoading && (
            <EmptyState icon={<AlertCircle size={20} />} message="No recent errors" />
          )}
        </StatusCard>

      </div>

      {/* Raw Inspection Section */}
      <div className={styles.inspection}>
        <div className={styles.inspectionHeader}>
          <Terminal size={14} />
          <span>Raw Inspection</span>
        </div>
        <div className={styles.inspectionGrid}>
          <JsonPanel data={systemStatus.data} label="/api/status" />
          <JsonPanel data={overview.data} label="/api/operator/overview" />
          <JsonPanel data={runtimeMetrics.data} label="/api/debug/runtime-metrics" />
        </div>
      </div>
    </div>
  );
}

function Row({ label, value, children }: { label: string; value?: string; children?: React.ReactNode }) {
  return (
    <div className={styles.row}>
      <span className={styles.rowLabel}>{label}</span>
      <span className={styles.rowValue}>{children ?? value}</span>
    </div>
  );
}
