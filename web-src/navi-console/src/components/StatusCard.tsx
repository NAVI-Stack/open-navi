import type { ReactNode } from 'react';
import clsx from 'clsx';
import styles from './StatusCard.module.css';

interface StatusCardProps {
  title: string;
  children: ReactNode;
  className?: string;
  loading?: boolean;
  error?: string | null;
  headerAction?: ReactNode;
  icon?: ReactNode;
}

export function StatusCard({ title, children, className, loading, error, headerAction, icon }: StatusCardProps) {
  return (
    <div className={clsx(styles.card, className)}>
      <div className={styles.header}>
        <div className={styles.titleGroup}>
          {icon && <span className={styles.icon}>{icon}</span>}
          <h3 className={styles.title}>{title}</h3>
        </div>
        {headerAction && <div className={styles.headerAction}>{headerAction}</div>}
      </div>
      <div className={styles.body}>
        {loading ? (
          <div className={styles.loading}>Loading...</div>
        ) : error ? (
          <div className={styles.error}>{error}</div>
        ) : (
          children
        )}
      </div>
    </div>
  );
}
