import { useRun } from '@/api/runs';
import { useNavigate } from '@/app/router';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { EmptyState } from '@/components/ui/EmptyState';
import { TimeAgo } from '@/components/ui/TimeAgo';
import { JsonPanel } from '@/components/JsonPanel';
import { useToast } from '@/components/ui/Toast';
import { ArrowLeft, Activity, Copy, AlertTriangle } from 'lucide-react';
import styles from './RunDetailPage.module.css';

type RunRecord = Record<string, unknown>;

function str(v: unknown): string | undefined {
  if (typeof v === 'string' && v.trim() !== '') return v;
  if (typeof v === 'number') return String(v);
  return undefined;
}

function list(v: unknown): string[] {
  if (Array.isArray(v)) return v.map(String).filter(Boolean);
  return [];
}

function statusVariant(status?: string): 'running' | 'completed' | 'failed' | 'default' {
  switch (status) {
    case 'running':
    case 'in_progress':
      return 'running';
    case 'completed':
    case 'success':
      return 'completed';
    case 'failed':
    case 'error':
      return 'failed';
    default:
      return 'default';
  }
}

function formatDuration(startIso?: string, endIso?: string): string | undefined {
  if (!startIso) return undefined;
  const start = new Date(startIso).getTime();
  const end = endIso ? new Date(endIso).getTime() : Date.now();
  if (Number.isNaN(start) || Number.isNaN(end) || end < start) return undefined;
  const secs = Math.round((end - start) / 1000);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  const rem = secs % 60;
  if (mins < 60) return `${mins}m ${rem}s`;
  const hrs = Math.floor(mins / 60);
  return `${hrs}h ${mins % 60}m`;
}

export function RunDetailPage({ runId }: { runId: string }) {
  const navigate = useNavigate();
  const toast = useToast();
  const { data, isLoading, error } = useRun(runId);

  const copy = (value: string) => {
    navigator.clipboard?.writeText(value).then(
      () => toast.success('Copied to clipboard'),
      () => toast.error('Could not copy'),
    );
  };

  if (isLoading) {
    return <div className={styles.loading}>Loading run…</div>;
  }

  if (error || !data) {
    return (
      <div className={styles.container}>
        <button className={styles.backLink} onClick={() => navigate('/runs')} type="button">
          <ArrowLeft size={14} /> Back to runs
        </button>
        <EmptyState
          icon={<Activity size={24} />}
          title="Run details unavailable"
          description="This run could not be found or the backend could not be reached."
        />
      </div>
    );
  }

  const run = data as RunRecord;
  const status = str(run.status) ?? str(run.outcome) ?? 'unknown';
  const runId_ = str(run.run_id) ?? runId;
  const type = str(run.type);
  const startedAt = str(run.started_at) ?? str(run.created_at);
  const endedAt = str(run.ended_at);
  const duration = formatDuration(startedAt, endedAt);
  const chatId = str(run.correlation_id) ?? str(run.chat_id);
  const runtimeSessionId = str(run.runtime_session_id);
  const workspaceId = str(run.workspace_id);
  const provider = str(run.llm_provider);
  const model = str(run.llm_model);
  const proposalId = str(run.proposal_id);
  const artifactId = str(run.artifact_id);
  const skillIds = list(run.skill_ids);
  const connectorIds = list(run.connector_ids);
  const failureClass = str(run.failure_class);
  const failureReason = str(run.failure_reason);
  const recoveryStatus = str(run.recovery_status);
  const approvalRequired = run.approval_required === true;
  const approvalOutcome = str(run.approval_outcome);
  const affected = Array.isArray(run.affected_entities) ? (run.affected_entities as unknown[]) : [];

  const isFailed = statusVariant(status) === 'failed';

  return (
    <div className={styles.container}>
      <button className={styles.backLink} onClick={() => navigate('/runs')} type="button">
        <ArrowLeft size={14} /> Back to runs
      </button>

      <header className={styles.header}>
        <div className={styles.headerMain}>
          <h1 className={styles.title}>Run {runId_.split('-')[0]}</h1>
          <button className={styles.copyId} onClick={() => copy(runId_)} type="button" title="Copy full run ID">
            <span className={styles.mono}>{runId_}</span>
            <Copy size={12} />
          </button>
        </div>
        <StatusBadge label={status} variant={statusVariant(status)} pulse={statusVariant(status) === 'running'} />
      </header>

      <section className={styles.metaGrid}>
        {type && <Field label="Type" value={type} />}
        {duration && <Field label="Duration" value={duration} />}
        {startedAt && (
          <Field label="Started" value={<TimeAgo date={startedAt} />} />
        )}
        {endedAt && <Field label="Ended" value={<TimeAgo date={endedAt} />} />}
        {(provider || model) && (
          <Field label="Model" value={[provider, model].filter(Boolean).join(' · ')} />
        )}
      </section>

      {isFailed && (failureReason || failureClass) && (
        <section className={styles.failurePanel}>
          <div className={styles.failureHead}>
            <AlertTriangle size={16} />
            <span>What went wrong</span>
          </div>
          {failureClass && <div className={styles.failureClass}>{failureClass.replace(/_/g, ' ')}</div>}
          {failureReason && <p className={styles.failureReason}>{failureReason}</p>}
          {recoveryStatus && recoveryStatus !== 'none' && (
            <div className={styles.recovery}>Recovery: {recoveryStatus.replace(/_/g, ' ')}</div>
          )}
        </section>
      )}

      <section className={styles.cards}>
        <LinkCard
          label="Chat"
          value={chatId ? `chat:${chatId.split('-')[0]}` : 'System'}
          onPress={chatId ? () => navigate(`/chats/${chatId}`) : undefined}
        />
        <LinkCard
          label="Artifact"
          value={artifactId ? artifactId.split('-')[0] : '—'}
          onPress={artifactId ? () => navigate('/artifacts') : undefined}
        />
        {runtimeSessionId && <InfoCard label="Runtime session" value={runtimeSessionId} onCopy={() => copy(runtimeSessionId)} />}
        {workspaceId && <InfoCard label="Workspace" value={workspaceId} onCopy={() => copy(workspaceId)} />}
      </section>

      {(approvalRequired || proposalId) && (
        <section className={styles.section}>
          <h2 className={styles.sectionTitle}>Governance</h2>
          <div className={styles.kvRow}>
            <span className={styles.kvKey}>Approval required</span>
            <span className={styles.kvVal}>{approvalRequired ? 'Yes' : 'No'}</span>
          </div>
          {approvalOutcome && approvalOutcome !== '' && (
            <div className={styles.kvRow}>
              <span className={styles.kvKey}>Approval outcome</span>
              <span className={styles.kvVal}>{approvalOutcome.replace(/_/g, ' ')}</span>
            </div>
          )}
          {proposalId && (
            <div className={styles.kvRow}>
              <span className={styles.kvKey}>Proposal</span>
              <button className={styles.kvLink} onClick={() => navigate('/proposals')} type="button">
                {proposalId.split('-')[0]}
              </button>
            </div>
          )}
        </section>
      )}

      {(skillIds.length > 0 || connectorIds.length > 0) && (
        <section className={styles.section}>
          <h2 className={styles.sectionTitle}>Capabilities used</h2>
          <div className={styles.chips}>
            {skillIds.map((id) => (
              <span key={`skill-${id}`} className={styles.chip}>skill: {id}</span>
            ))}
            {connectorIds.map((id) => (
              <span key={`conn-${id}`} className={styles.chip}>connector: {id}</span>
            ))}
          </div>
        </section>
      )}

      {affected.length > 0 && (
        <section className={styles.section}>
          <h2 className={styles.sectionTitle}>Affected entities</h2>
          <div className={styles.chips}>
            {affected.map((e, i) => (
              <span key={i} className={styles.chip}>{typeof e === 'string' ? e : JSON.stringify(e)}</span>
            ))}
          </div>
        </section>
      )}

      <section className={styles.section}>
        <JsonPanel data={run} label="Developer details" />
      </section>
    </div>
  );
}

function Field({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className={styles.field}>
      <span className={styles.fieldLabel}>{label}</span>
      <span className={styles.fieldValue}>{value}</span>
    </div>
  );
}

function LinkCard({ label, value, onPress }: { label: string; value: string; onPress?: () => void }) {
  return (
    <button className={styles.card} onClick={onPress} disabled={!onPress} type="button" data-clickable={!!onPress}>
      <span className={styles.cardLabel}>{label}</span>
      <span className={styles.cardValue}>{value}</span>
    </button>
  );
}

function InfoCard({ label, value, onCopy }: { label: string; value: string; onCopy: () => void }) {
  return (
    <div className={styles.card}>
      <span className={styles.cardLabel}>{label}</span>
      <button className={styles.cardCopy} onClick={onCopy} type="button" title={`Copy ${label}`}>
        <span className={styles.mono}>{value.split('-')[0]}</span>
        <Copy size={11} />
      </button>
    </div>
  );
}
