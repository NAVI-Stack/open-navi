import clsx from 'clsx';
import styles from './Spinner.module.css';

export interface SpinnerProps {
  size?: number;
  label?: string;
  className?: string;
}

/** Lightweight, dependency-free loading indicator. */
export function Spinner({ size = 16, label = 'Loading', className }: SpinnerProps) {
  return (
    <span
      className={clsx(styles.spinner, className)}
      role="status"
      aria-label={label}
      style={{ width: size, height: size }}
    />
  );
}
