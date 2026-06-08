import { useState } from 'react';
import { JsonPanel } from './JsonPanel';
import { StatusBadge } from './ui/StatusBadge';
import type { TimelineItem } from '@/types/timeline';
import { redactSecrets } from '@/types/timeline';
import styles from './EventRow.module.css';

interface EventRowProps {
  item: TimelineItem;
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleTimeString(undefined, { hour12: false, fractionalSecondDigits: 3 });
  } catch {
    return iso;
  }
}

const sourceLabels: Record<string, string> = {
  live: 'WS',
  activity: 'ACT',
  debug: 'DBG',
  error: 'ERR',
};

export function EventRow({ item }: EventRowProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className={styles.row} data-variant={item.variant}>
      <button
        className={styles.header}
        onClick={() => setExpanded(!expanded)}
        aria-expanded={expanded}
      >
        <span className={styles.time}>{formatTime(item.timestamp)}</span>
        <span className={styles.source}>{sourceLabels[item.source] ?? item.source}</span>
        <StatusBadge label={item.type} variant={item.variant === 'success' ? 'success' : item.variant} />
        <span className={styles.summary}>{item.summary}</span>
      </button>
      {expanded && (
        <div className={styles.detail}>
          <JsonPanel data={redactSecrets(item.raw)} label="Raw payload" defaultExpanded />
        </div>
      )}
    </div>
  );
}
