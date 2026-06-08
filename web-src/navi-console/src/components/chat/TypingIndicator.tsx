import { Bot } from 'lucide-react';
import styles from './TypingIndicator.module.css';

interface Props {
  isNaviActive?: boolean;
}

// TypingIndicator mirrors the assistant message layout (avatar + three bouncing
// dots) so the "NAVI is thinking" state reads as a forming reply.
export function TypingIndicator({ isNaviActive }: Props) {
  return (
    <div className={styles.row} role="status" aria-label="NAVI is typing">
      <div className={styles.avatar} data-active={isNaviActive ? 'true' : undefined} aria-hidden="true">
        <Bot size={16} />
      </div>
      <div className={styles.bubble}>
        <span className={styles.dot} />
        <span className={styles.dot} />
        <span className={styles.dot} />
      </div>
    </div>
  );
}
