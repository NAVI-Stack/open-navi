import { FileWarning } from 'lucide-react';
import { useVaultSyncLog } from '@/api/intake';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { TimeAgo } from '@/components/ui/TimeAgo';
import styles from './intake.module.css';

// VaultDriftPanel renders the Vault drift report / per-file sync log in the
// operator/debug surface (Console V2 addendum; Vault §13). P5 ships the UI
// hooks; the Memory Vault deliverable populates the rows. Until then the panel
// shows an explicit "no data yet" state — the surface exists.
export function VaultDriftPanel() {
  const { data, isLoading, error } = useVaultSyncLog(undefined, 50);

  if (isLoading) return <div className={styles.muted}>Loading Vault drift report…</div>;
  if (error) return <div className={styles.error}>Failed to load Vault sync log.</div>;

  const rows = data ?? [];
  if (rows.length === 0) {
    return (
      <div className={styles.muted}>
        <FileWarning size={14} /> No Vault drift yet — the Memory Vault deliverable populates this report once
        it ships. (Surface present; CIP P5 builds the hook, the Vault writes the data.)
      </div>
    );
  }

  return (
    <div className={styles.logList}>
      {rows.map((e) => (
        <div key={e.id} className={styles.logRow}>
          <div className={styles.logMeta}>
            <div>
              <span className={styles.value}>{e.file_path}</span>{' '}
              {e.terminal_status && <StatusBadge size="sm" label={e.terminal_status} variant="muted" />}
            </div>
            <span className={styles.logCounts}>
              {e.diff_summary || 'diff'} · proposed {e.mutations_proposed ?? 0} · approved{' '}
              {e.mutations_approved ?? 0} · proposals {e.proposals_raised ?? 0}
            </span>
          </div>
          {e.created_at && <TimeAgo date={e.created_at} />}
        </div>
      ))}
    </div>
  );
}
