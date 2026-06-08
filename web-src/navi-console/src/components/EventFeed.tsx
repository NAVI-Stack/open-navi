import { EventRow } from './EventRow';
import { EmptyState } from './ui/EmptyState';
import { ScrollText } from 'lucide-react';
import type { TimelineItem } from '@/types/timeline';
import styles from './EventFeed.module.css';

interface EventFeedProps {
  items: TimelineItem[];
  emptyMessage?: string;
}

export function EventFeed({ items, emptyMessage = 'No events yet' }: EventFeedProps) {
  if (items.length === 0) {
    return (
      <div className={styles.emptyWrap}>
        <EmptyState icon={<ScrollText size={24} />} title={emptyMessage} />
      </div>
    );
  }

  return (
    <div className={styles.feedWrap}>
      <div className={styles.feed}>
        {items.map((item) => (
          <EventRow key={item.id} item={item} />
        ))}
      </div>
    </div>
  );
}
