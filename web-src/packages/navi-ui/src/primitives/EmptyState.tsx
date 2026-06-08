import type { ReactNode } from 'react';
import { Button } from './Button';
import styles from './EmptyState.module.css';

export interface EmptyStateProps {
  icon?: ReactNode;
  title?: string;
  description?: string;
  message?: string; // Legacy alias for title
  action?: { label: string; onPress: () => void };
}

export function EmptyState({ icon, title, description, message, action }: EmptyStateProps) {
  const displayTitle = title || message || '';

  return (
    <div className={styles.empty}>
      {icon && <div className={styles.icon}>{icon}</div>}
      <h3 className={styles.title}>{displayTitle}</h3>
      {description && <p className={styles.description}>{description}</p>}
      {action && (
        <Button variant="primary" onPress={action.onPress}>
          {action.label}
        </Button>
      )}
    </div>
  );
}
