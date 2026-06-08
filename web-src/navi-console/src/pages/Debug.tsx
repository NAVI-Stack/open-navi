import { useDebugEvents, useRuntimeMetrics, useLlmKbProfiles, useGovernor } from '@/api/debug';
import { useActivity, useErrors } from '@/api/activity';
import { useErrorSummary, useSystemStatus, useOperatorOverview } from '@/api/status';
import { useRuns } from '@/api/runs';
import { useLiveEventsContext } from '@/hooks/LiveEventsContext';
import { DebugSection } from '@/components/DebugSection';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { JsonPanel } from '@/components/JsonPanel';
import { useIntakeSyncLog } from '@/api/intake';
import { VaultDriftPanel } from '@/components/intake/VaultDriftPanel';
import styles from './Debug.module.css';

export function Debug() {
  const metrics = useRuntimeMetrics();
  const debugEvents = useDebugEvents();
  const governor = useGovernor();
  const errors = useErrors();
  const errorSummary = useErrorSummary();
  const runs = useRuns(20);
  const kbProfiles = useLlmKbProfiles();
  const activity = useActivity(20);
  const live = useLiveEventsContext();
  const systemStatus = useSystemStatus();
  const overview = useOperatorOverview();
  const intakeLog = useIntakeSyncLog(undefined, 25);

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h2 className={styles.heading}>Debug</h2>
        <div className={styles.headerActions}>
          <StatusBadge 
            label={live.connectionState} 
            variant={live.connectionState === 'connected' ? 'success' : 'warning'} 
          />
        </div>
      </div>

      <div className={styles.grid}>
        {/* CIP P5: Intake sync log (operator view of every connector pass). */}
        <DebugSection
          title="Intake Sync Log"
          loading={intakeLog.isLoading}
          error={intakeLog.error}
          empty={!intakeLog.data || intakeLog.data.length === 0}
          emptyMessage="No intake passes recorded yet."
        >
          {(intakeLog.data ?? []).map((e) => (
            <div key={e.id} className={styles.metricRow}>
              <span className={styles.metricLabel}>
                {e.connector_id} · {e.job_mode}
              </span>
              <span className={styles.metricValue}>
                <StatusBadge
                  size="sm"
                  label={e.terminal_status ?? '—'}
                  variant={e.terminal_status === 'completed' ? 'success' : e.terminal_status === 'cost_ceiling' || e.terminal_status === 'consent_rejected' ? 'danger' : 'warning'}
                />{' '}
                adm {e.records_admitted ?? 0} · dist {e.records_distilled ?? 0} · syn {e.records_synthesized ?? 0}
              </span>
            </div>
          ))}
        </DebugSection>

        {/* CIP P5: Vault drift report (UI hook; populated by the Vault deliverable). */}
        <DebugSection title="Vault Drift Report" defaultExpanded>
          <VaultDriftPanel />
        </DebugSection>

        {/* Runtime Metrics */}
        <DebugSection 
          title="Runtime Metrics" 
          loading={metrics.isLoading} 
          error={metrics.error} 
          data={metrics.data}
          empty={!metrics.data || typeof metrics.data !== 'object' || metrics.data === null || Object.keys(metrics.data).length === 0}
        >
          {metrics.data && typeof metrics.data === 'object' && metrics.data !== null && Object.entries(metrics.data).map(([key, val]) => (
            <div key={key} className={styles.metricRow}>
              <span className={styles.metricLabel}>{key}</span>
              <span className={styles.metricValue}>{val !== null && val !== undefined ? String(val) : 'null'}</span>
            </div>
          ))}
        </DebugSection>

        {/* Governor */}
        <DebugSection 
          title="Governor State" 
          loading={governor.isLoading} 
          error={governor.error} 
          data={governor.data}
        >
          {governor.data && typeof governor.data === 'object' && (
            <div className={styles.rows}>
              <div className={styles.metricRow}>
                <span className={styles.metricLabel}>Tripped</span>
                <span className={styles.metricValue}>{governor.data.tripped ? 'YES' : 'NO'}</span>
              </div>
              <div className={styles.metricRow}>
                <span className={styles.metricLabel}>Budget</span>
                <span className={styles.metricValue}>{governor.data.budget_used ?? 0} / {governor.data.budget_max ?? 0}</span>
              </div>
              <div className={styles.metricRow}>
                <span className={styles.metricLabel}>Cost</span>
                <span className={styles.metricValue}>
                  ${typeof governor.data.cost_used === 'number' ? governor.data.cost_used.toFixed(4) : '0.0000'}
                </span>
              </div>
            </div>
          )}
        </DebugSection>

        {/* Error Summary */}
        <DebugSection 
          title="Error Summary" 
          loading={errorSummary.isLoading} 
          error={errorSummary.error} 
          data={errorSummary.data}
        >
          {errorSummary.data?.items?.map((item, i) => (
            <div key={i} className={styles.metricRow}>
              <span className={styles.metricLabel}>{item?.type || item?.component || 'unknown'}</span>
              <span className={styles.metricValue}>{item?.count ?? 0}</span>
            </div>
          ))}
        </DebugSection>

        {/* LLM-KB Profiles */}
        <DebugSection 
          title="LLM-KB Profiles" 
          loading={kbProfiles.isLoading} 
          error={kbProfiles.error} 
          data={kbProfiles.data}
          empty={!kbProfiles.data?.items?.length}
        >
          {kbProfiles.data?.items?.map((p, i) => (
            <div key={p?.id || i} className={styles.kbRow}>
              <div className={styles.kbInfo}>
                <span className={styles.kbName}>{p?.name || p?.id || 'Unknown Profile'}</span>
                <div className={styles.kbMeta}>{p?.provider || 'Unknown'} / {p?.model || 'Unknown'}</div>
              </div>
              {p?.status ? (
                <StatusBadge label={p.status} size="sm" />
              ) : (
                <StatusBadge label="profile" variant="muted" size="sm" />
              )}
            </div>
          ))}
        </DebugSection>

        {/* Event Snapshot */}
        <DebugSection 
          title="Debug Events" 
          loading={debugEvents.isLoading} 
          error={debugEvents.error} 
          data={debugEvents.data}
          className={styles.wide}
          empty={!debugEvents.data?.items?.length}
        >
          <div className={styles.list}>
            {debugEvents.data?.items?.slice(0, 10).map((event, i) => (
              <div key={event?.id || i} className={styles.eventRow}>
                <div className={styles.eventMeta}>
                  <span className={styles.eventType}>{event?.type || event?.kind || 'unknown'}</span>
                  <span className={styles.mono}>{event?.id || 'no-id'}</span>
                  <span>{event?.timestamp ? new Date(event.timestamp).toLocaleTimeString() : 'no-time'}</span>
                </div>
                <JsonPanel data={event?.payload} label="Event Payload" />
              </div>
            ))}
          </div>
        </DebugSection>

        {/* Errors Log */}
        <DebugSection 
          title="Recent Errors" 
          loading={errors.isLoading} 
          error={errors.error} 
          data={errors.data}
          empty={!errors.data?.items?.length}
        >
          {errors.data?.items?.slice(0, 10).map((err, i) => (
            <div key={err?.id || i} className={styles.errorRow}>
              <div className={styles.errorTime}>
                {err?.timestamp ? new Date(err.timestamp).toLocaleTimeString() : 'no-time'}
              </div>
              <div className={styles.errorContent}>
                <span className={styles.errorMessage}>{err?.message || 'Unknown error'}</span>
                <span className={styles.errorComponent}>{err?.component || 'unknown'} • {err?.type || 'unknown'}</span>
              </div>
            </div>
          ))}
        </DebugSection>

        {/* Recent Runs */}
        <DebugSection 
          title="Recent Runs" 
          loading={runs.isLoading} 
          error={runs.error} 
          data={runs.data}
          empty={!runs.data?.items?.length}
        >
          {runs.data?.items?.slice(0, 10).map((run, i) => (
            <div key={run?.id || run?.run_id || i} className={styles.runRow}>
              <span className={styles.runId}>{run?.id || run?.run_id || 'unknown'}</span>
              <StatusBadge label={run?.status || 'unknown'} variant="muted" size="sm" />
            </div>
          ))}
        </DebugSection>

        {/* Activity Feed */}
        <DebugSection 
          title="Activity Feed" 
          loading={activity.isLoading} 
          error={activity.error} 
          data={activity.data}
          empty={!activity.data?.items?.length}
        >
          {activity.data?.items?.slice(0, 10).map((item, i) => (
            <div key={i} className={styles.metricRow}>
              <span className={styles.metricLabel}>{item?.at ? new Date(item.at).toLocaleTimeString() : 'no-time'}</span>
              <span className={styles.metricValue}>{item?.summary || 'no summary'}</span>
            </div>
          ))}
        </DebugSection>

        {/* Live Feed */}
        <DebugSection 
          title="Live WebSocket Feed" 
          loading={false} 
          data={live.events}
          empty={live.events.length === 0}
          className={styles.wide}
        >
          <div className={styles.liveFeed}>
            {live.events.slice(0, 15).map((e, i) => (
              <div key={i} className={styles.liveRow}>
                <span className={styles.liveTime}>{e?.receivedAt ? new Date(e.receivedAt).toLocaleTimeString() : 'no-time'}</span>
                <span className={styles.liveMethod}>{(e?.frame as any)?.method || (e?.frame as any)?.type || 'MSG'}</span>
                <div className={styles.livePayload}>
                  {JSON.stringify(e?.frame || {})}
                </div>
                <JsonPanel data={e?.frame} label="Frame" />
              </div>
            ))}
          </div>
        </DebugSection>

        {/* Snapshots */}
        <DebugSection title="/api/status Snapshot" data={systemStatus.data} loading={systemStatus.isLoading} />
        <DebugSection title="/api/operator/overview Snapshot" data={overview.data} loading={overview.isLoading} />

      </div>
    </div>
  );
}
