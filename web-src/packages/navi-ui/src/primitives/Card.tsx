import type { ReactNode } from 'react';
import clsx from 'clsx';
import styles from './Card.module.css';

export interface CardProps {
  children: ReactNode;
  className?: string;
  variant?: 'default' | 'elevated' | 'outlined';
  hoverable?: boolean;
  onClick?: () => void;
}

export function Card({ children, className, variant = 'default', hoverable, onClick }: CardProps) {
  return (
    <div
      className={clsx(styles.card, styles[variant], hoverable && styles.hoverable, className)}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
    >
      {children}
    </div>
  );
}
