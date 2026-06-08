import clsx from 'clsx';
import styles from './StatusBadge.module.css';

export type StatusVariant =
  | 'idle'
  | 'running'
  | 'blocked'
  | 'completed'
  | 'failed'
  | 'warning'
  | 'info'
  | 'default'
  | 'danger'
  | 'success'
  | 'muted'
  | 'accent';

export interface StatusBadgeProps {
  variant?: StatusVariant;
  label: string;
  pulse?: boolean;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

export function StatusBadge({ variant = 'default', label, pulse, size = 'md', className }: StatusBadgeProps) {
  return (
    <div className={clsx(styles.badge, styles[variant], styles[size], className)}>
      <span className={clsx(styles.dot, pulse && styles.pulse)}></span>
      <span className={styles.label}>{label}</span>
    </div>
  );
}
