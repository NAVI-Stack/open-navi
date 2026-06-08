import React from 'react';
import { JsonPanel } from './JsonPanel';
import { StatusBadge } from './ui/StatusBadge';
import { AlertCircle, Loader2, Database } from 'lucide-react';
import styles from './DebugSection.module.css';

interface DebugSectionProps {
  title: string;
  loading?: boolean;
  error?: string | Error | null;
  data?: any;
  empty?: boolean;
  emptyMessage?: string;
  children?: React.ReactNode;
  defaultExpanded?: boolean;
  className?: string;
}

export function DebugSection({
  title,
  loading,
  error,
  data,
  empty,
  emptyMessage = 'No data available',
  children,
  defaultExpanded = false,
  className = '',
}: DebugSectionProps) {
  const errorMsg = error instanceof Error ? error.message : error;

  return (
    <div className={`${styles.section} ${className}`}>
      <div className={styles.header}>
        <div className={styles.titleGroup}>
          <Database size={14} className={styles.icon} />
          <h3 className={styles.title}>{title}</h3>
        </div>
        <div className={styles.badges}>
          {loading && <StatusBadge label="loading" variant="muted" />}
          {error && <StatusBadge label="error" variant="danger" />}
        </div>
      </div>

      <div className={styles.content}>
        {loading && !data && (
          <div className={styles.state}>
            <Loader2 size={20} className={styles.spin} />
            <span>Fetching diagnostics...</span>
          </div>
        )}

        {error && !data && (
          <div className={styles.state} data-type="error">
            <AlertCircle size={20} />
            <span>{errorMsg}</span>
          </div>
        )}

        {!loading && !error && empty && (
          <div className={styles.state} data-type="empty">
            <span>{emptyMessage}</span>
          </div>
        )}

        {children && <div className={styles.children}>{children}</div>}

        {data && (
          <div className={styles.jsonWrap}>
            <JsonPanel data={data} label="Raw Payload" defaultExpanded={defaultExpanded} />
          </div>
        )}
      </div>
    </div>
  );
}
