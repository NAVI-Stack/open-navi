import { useEffect, useState } from 'react';
import { Database, Save, History } from 'lucide-react';
import { useConnectorSync, useUpdateSyncPolicy, useRequestBackfill } from '@/api/intake';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { TimeAgo } from '@/components/ui/TimeAgo';
import type { IntakeSyncLogEntry, SyncPolicy } from '@/types/api';
import styles from './intake.module.css';

const PRIVACY_CLASSES = ['public', 'personal', 'sensitive', 'secret'];

function statusVariant(status?: string): 'success' | 'warning' | 'danger' | 'muted' | 'accent' {
  switch (status) {
    case 'completed':
      return 'success';
    case 'budget_exceeded':
    case 'consent_pending':
      return 'warning';
    case 'cost_ceiling':
    case 'consent_rejected':
    case 'error':
      return 'danger';
    case 'running':
      return 'accent';
    default:
      return 'muted';
  }
}

function PassRow({ e }: { e: IntakeSyncLogEntry }) {
  return (
    <div className={styles.logRow}>
      <div className={styles.logMeta}>
        <div>
          <StatusBadge size="sm" label={e.terminal_status ?? 'unknown'} variant={statusVariant(e.terminal_status)} />{' '}
          <span className={styles.muted}>{e.job_mode ?? 'delta'}</span>
        </div>
        <span className={styles.logCounts}>
          admitted {e.records_admitted ?? 0} · deduped {e.records_deduped ?? 0} · distilled{' '}
          {e.records_distilled ?? 0} · synthesized {e.records_synthesized ?? 0} · errors {e.errors ?? 0}
        </span>
      </div>
      {e.ended_at && <TimeAgo date={e.ended_at} />}
    </div>
  );
}

// ConnectorSyncPanel renders the CIP P5 per-connector sync state inside the
// connector detail view (Console V2 addendum): current policy, last-pass
// metrics, next scheduled pass, recent errors, and an editable policy form.
export function ConnectorSyncPanel({ connectorId }: { connectorId: string }) {
  const { data, isLoading, error } = useConnectorSync(connectorId);
  const update = useUpdateSyncPolicy();
  const backfill = useRequestBackfill();

  const [cadence, setCadence] = useState('');
  const [privacy, setPrivacy] = useState('');
  const [maxRecords, setMaxRecords] = useState('');
  const [maxBytes, setMaxBytes] = useState('');
  const [cost, setCost] = useState('');
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    if (!data) return;
    setCadence(data.policy.delta?.cadence ?? '');
    setPrivacy(data.policy.privacy_class ?? '');
    setMaxRecords(String(data.policy.delta?.budget?.max_records_per_pass ?? 0));
    setMaxBytes(String(data.policy.delta?.budget?.max_bytes_per_pass ?? 0));
    setCost(String(data.policy.delta?.budget?.cost_ceiling_usd ?? 0));
  }, [data]);

  if (isLoading) return <div className={styles.muted}>Loading sync state…</div>;
  if (error) return <div className={styles.error}>Failed to load sync state.</div>;
  if (!data) return <div className={styles.muted}>No sync state.</div>;

  const p = data.policy;

  const onSave = async () => {
    setSaveError(null);
    const patch: SyncPolicy = {
      connector_id: connectorId,
      privacy_class: privacy,
      visibility: p.visibility ?? '',
      delta: {
        cadence,
        cursor: '',
        dedupe: '',
        freshness: '',
        budget: {
          max_records_per_pass: Number(maxRecords) || 0,
          max_bytes_per_pass: Number(maxBytes) || 0,
          cost_ceiling_usd: Number(cost) || 0,
        },
      },
      backfill: p.backfill,
    };
    try {
      await update.mutateAsync(patch);
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div className={styles.panel}>
      {/* Last pass metrics */}
      <div className={styles.card}>
        <div className={styles.cardHead}>
          <span className={styles.cardTitle}>
            <Database size={13} /> Last pass
          </span>
          {data.last_pass?.terminal_status && (
            <StatusBadge
              size="sm"
              label={data.last_pass.terminal_status}
              variant={statusVariant(data.last_pass.terminal_status)}
            />
          )}
        </div>
        {data.last_pass ? (
          <div className={styles.metricsRow}>
            <div className={styles.metric}>
              <span className={styles.metricNum}>{data.last_pass.records_admitted ?? 0}</span>
              <span className={styles.metricLabel}>admitted</span>
            </div>
            <div className={styles.metric}>
              <span className={styles.metricNum}>{data.last_pass.records_deduped ?? 0}</span>
              <span className={styles.metricLabel}>deduped</span>
            </div>
            <div className={styles.metric}>
              <span className={styles.metricNum}>{data.last_pass.records_distilled ?? 0}</span>
              <span className={styles.metricLabel}>distilled</span>
            </div>
            <div className={styles.metric}>
              <span className={styles.metricNum}>{data.last_pass.records_synthesized ?? 0}</span>
              <span className={styles.metricLabel}>synthesized</span>
            </div>
            <div className={styles.metric}>
              <span className={styles.metricNum}>{data.last_pass.errors ?? 0}</span>
              <span className={styles.metricLabel}>errors</span>
            </div>
          </div>
        ) : (
          <div className={styles.muted}>No passes recorded yet.</div>
        )}
        <div className={styles.grid} style={{ marginTop: 10 }}>
          <span className={styles.label}>Next scheduled</span>
          <span className={styles.value}>
            {data.next_pass ? <TimeAgo date={data.next_pass} /> : 'webhook / manual (no fixed interval)'}
          </span>
          {data.consent_state && (
            <>
              <span className={styles.label}>Backfill consent</span>
              <span className={styles.value}>{data.consent_state}</span>
            </>
          )}
        </div>
      </div>

      {/* Editable policy */}
      <div className={styles.card}>
        <div className={styles.cardHead}>
          <span className={styles.cardTitle}>Sync policy (delta)</span>
          <span className={styles.muted}>{connectorId}</span>
        </div>
        <div className={styles.editGrid}>
          <span className={styles.label}>Cadence</span>
          <input className={styles.input} value={cadence} onChange={(e) => setCadence(e.target.value)} placeholder="20m / manual / webhook-triggered" />
          <span className={styles.label}>Privacy class</span>
          <select className={styles.select} value={privacy} onChange={(e) => setPrivacy(e.target.value)}>
            {PRIVACY_CLASSES.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
          <span className={styles.label}>Max records / pass</span>
          <input className={styles.input} type="number" value={maxRecords} onChange={(e) => setMaxRecords(e.target.value)} />
          <span className={styles.label}>Max bytes / pass</span>
          <input className={styles.input} type="number" value={maxBytes} onChange={(e) => setMaxBytes(e.target.value)} />
          <span className={styles.label}>Cost ceiling (USD)</span>
          <input className={styles.input} type="number" step="0.01" value={cost} onChange={(e) => setCost(e.target.value)} />
        </div>
        {saveError && <div className={styles.error} style={{ marginTop: 8 }}>{saveError}</div>}
        <div className={styles.actions}>
          <button className={`${styles.btn} ${styles.btnPrimary}`} onClick={onSave} disabled={update.isPending}>
            <Save size={13} /> {update.isPending ? 'Saving…' : 'Save policy'}
          </button>
          <button
            className={styles.btn}
            onClick={() => backfill.mutate(connectorId)}
            disabled={backfill.isPending}
            title="Raises a consent Proposal in the Proposal queue; the backfill starts only after you approve it."
          >
            <History size={13} /> Request backfill
          </button>
        </div>
      </div>

      {/* Recent passes */}
      <div className={styles.card}>
        <div className={styles.cardHead}>
          <span className={styles.cardTitle}>Recent passes</span>
        </div>
        {data.recent_passes.length === 0 ? (
          <div className={styles.muted}>No sync passes recorded.</div>
        ) : (
          <div className={styles.logList}>
            {data.recent_passes.map((e) => (
              <PassRow key={e.id} e={e} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
