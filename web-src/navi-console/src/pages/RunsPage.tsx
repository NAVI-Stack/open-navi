import { useRuns } from '@/api/runs';
import { useNavigate } from '@/app/router';
import { StatusBadge } from '@/components/ui/StatusBadge';
import { EmptyState } from '@/components/ui/EmptyState';
import { TimeAgo } from '@/components/ui/TimeAgo';
import { PlayCircle, Activity } from 'lucide-react';
import styles from './RunsPage.module.css';

export function RunsPage() {
  const navigate = useNavigate();
  const { data, isLoading, error } = useRuns(50);

  if (isLoading) {
    return <div className={styles.loading}>Loading runs...</div>;
  }

  if (error) {
    return (
      <EmptyState
        icon={<Activity size={24} />}
        title="Error Loading Runs"
        description="Could not connect to the gateway to fetch runs."
      />
    );
  }

  const runs = data?.items || [];

  if (runs.length === 0) {
    return (
      <EmptyState
        icon={<PlayCircle size={24} />}
        title="No Runs Yet"
        description="Active and past runs will appear here."
      />
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1 className={styles.title}>Execution Runs</h1>
        <p className={styles.description}>
          View lifecycle events, status, and task progression across all runtime sessions.
        </p>
      </div>

      <div className={styles.tableWrapper}>
        <table className={styles.table}>
          <thead>
            <tr>
              <th>Run ID</th>
              <th>Status</th>
              <th>Chat / Project</th>
              <th>Created</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {runs.map((run) => (
              <tr 
                key={run.run_id} 
                className={styles.row}
                onClick={() => navigate(`/runs/${run.run_id}`)}
              >
                <td className={styles.cellMain}>
                  <span className={styles.mono}>{run.run_id?.split('-')[0]}</span>
                </td>
                <td>
                  <StatusBadge 
                    label={run.status || 'unknown'} 
                    variant={run.status === 'running' ? 'running' : run.status === 'completed' ? 'completed' : run.status === 'failed' ? 'failed' : 'default'}
                  />
                </td>
                <td className={styles.cellMuted}>
                  {run.chat_id ? (
                    <span className={styles.monoLink} onClick={(e) => {
                      e.stopPropagation();
                      navigate(`/chats/${run.chat_id}`);
                    }}>
                      chat:{run.chat_id.split('-')[0]}
                    </span>
                  ) : 'System'}
                </td>
                <td className={styles.cellMuted}>
                  {run.created_at ? <TimeAgo date={run.created_at} /> : '-'}
                </td>
                <td className={styles.cellMuted}>
                  {run.updated_at ? <TimeAgo date={run.updated_at} /> : '-'}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
