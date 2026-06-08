import { Inbox } from 'lucide-react';
import { useRecentIntake } from '@/api/intake';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { TimeAgo } from '@/components/ui/TimeAgo';
import styles from './intake.module.css';

// RecentIntakePanel renders recent connector-sourced intake with provenance
// links back to the source record (Console V2 addendum). It rides in the right
// inspector's Context tab for a chat or project. connectorId scopes it when
// known; otherwise it shows recent intake across all connectors.
export function RecentIntakePanel({ connectorId }: { connectorId?: string }) {
  const { data, isLoading, error } = useRecentIntake(connectorId, 15);

  if (isLoading) return <div className={styles.muted}>Loading recent intake…</div>;
  if (error) return <div className={styles.error}>Failed to load recent intake.</div>;

  const records = data ?? [];
  if (records.length === 0) {
    return (
      <div className={styles.muted}>
        <Inbox size={14} /> No connector-sourced context yet.
      </div>
    );
  }

  return (
    <div className={styles.panel}>
      <div className={styles.logList}>
        {records.map((r) => (
          <div key={r.record_id} className={styles.recordRow}>
            <div className={styles.recordHead}>
              <span className={styles.value}>
                {r.connector_id} · {r.source_kind || 'record'}
              </span>
              <div style={{ display: 'flex', gap: 6 }}>
                {r.privacy_class && <StatusBadge size="sm" label={r.privacy_class} variant="muted" />}
                {typeof r.chunk_count === 'number' && r.chunk_count > 0 && (
                  <StatusBadge size="sm" label={`${r.chunk_count} chunks`} variant="muted" />
                )}
              </div>
            </div>
            <div className={styles.recordSub}>
              {r.author ? `${r.author} · ` : ''}
              {r.fetched_at ? <TimeAgo date={r.fetched_at} /> : null}
              {r.trust ? ` · ${r.trust}` : ''}
            </div>
            {r.link_back ? (
              <a className={styles.link} href={r.link_back} target="_blank" rel="noreferrer">
                {r.link_back}
              </a>
            ) : (
              <span className={styles.recordSub}>source: {r.source_id || r.record_id}</span>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
