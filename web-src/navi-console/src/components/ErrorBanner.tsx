import { useState } from 'react';
import { AlertTriangle, ChevronDown, ChevronRight, X } from 'lucide-react';
import { NaviApiError } from '@/api/errors';
import styles from './ErrorBanner.module.css';

interface ErrorBannerProps {
  error: Error;
  onDismiss?: () => void;
}

export function ErrorBanner({ error, onDismiss }: ErrorBannerProps) {
  const [expanded, setExpanded] = useState(false);

  const isApi = error instanceof NaviApiError;
  const status = isApi ? error.status : undefined;
  const raw = isApi ? error.raw : undefined;

  return (
    <div className={styles.banner} role="alert">
      <div className={styles.header}>
        <AlertTriangle size={14} />
        <span className={styles.message}>
          {status ? `[${status}] ` : ''}{error.message}
        </span>
        {raw !== undefined && (
          <button
            className={styles.toggle}
            onClick={() => setExpanded(!expanded)}
            aria-label={expanded ? 'Collapse details' : 'Expand details'}
          >
            {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </button>
        )}
        {onDismiss && (
          <button className={styles.dismiss} onClick={onDismiss} aria-label="Dismiss">
            <X size={14} />
          </button>
        )}
      </div>
      {expanded && raw !== undefined && (
        <pre className={styles.details}>{JSON.stringify(raw, null, 2)}</pre>
      )}
    </div>
  );
}
