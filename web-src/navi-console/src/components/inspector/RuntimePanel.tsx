import { useChatRuntimeSummary } from '@/api/chats';
import { StatusBadge } from '../ui/StatusBadge';
import { TimeAgo } from '../ui/TimeAgo';
import styles from './RuntimePanel.module.css';

interface RuntimePanelProps {
  chatId?: string;
}

export function RuntimePanel({ chatId }: RuntimePanelProps) {
  const { data: summary, isLoading } = useChatRuntimeSummary(chatId || '');

  if (!chatId) {
    return <div className={styles.empty}>No chat selected</div>;
  }

  if (isLoading) {
    return <div className={styles.empty}>Loading runtime info...</div>;
  }

  if (!summary) {
    return <div className={styles.empty}>No runtime active</div>;
  }

  return (
    <div className={styles.panel}>
      <div className={styles.section}>
        <div className={styles.sectionTitle}>Status</div>
        <StatusBadge 
          label={summary.status || 'unknown'} 
          variant={
            summary.status === 'running' ? 'running' :
            summary.status === 'blocked' ? 'blocked' :
            summary.status === 'completed' ? 'completed' :
            summary.status === 'failed' ? 'failed' : 'idle'
          }
          pulse={summary.status === 'running'}
        />
      </div>

      <div className={styles.section}>
        <div className={styles.sectionTitle}>Runtime Session</div>
        <div className={styles.codeValue}>{summary.runtime_session_id || 'None'}</div>
      </div>

      {summary.active_run_id && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>Active Run</div>
          <div className={styles.codeValue}>{summary.active_run_id}</div>
        </div>
      )}

      {summary.recent_run_ids && summary.recent_run_ids.length > 0 && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>Recent Runs</div>
          <div className={styles.runList}>
            {summary.recent_run_ids.map(id => (
              <div key={id} className={styles.runListItem}>{id}</div>
            ))}
          </div>
        </div>
      )}

      <div className={styles.section}>
        <div className={styles.sectionTitle}>Timestamps</div>
        <div className={styles.metaList}>
          <div className={styles.metaItem}>
            <span className={styles.metaLabel}>Created:</span>
            <span className={styles.metaValue}><TimeAgo date={summary.created_at} /></span>
          </div>
          <div className={styles.metaItem}>
            <span className={styles.metaLabel}>Updated:</span>
            <span className={styles.metaValue}><TimeAgo date={summary.updated_at} /></span>
          </div>
        </div>
      </div>
    </div>
  );
}
