import clsx from 'clsx';
import styles from './NaviMetricCard.module.css';

export interface NaviMetricCardProps {
  /** Short metric label, e.g. "Total tool calls". */
  label: string;
  /** The headline value, e.g. "42". */
  value: string | number;
  /** Optional supporting line, e.g. "across 5 tools". */
  sub?: string;
  className?: string;
}

/**
 * NaviMetricCard — a single headline metric tile. A NAVI domain component used by
 * data-driven render surfaces (tool-usage, run summaries) instead of a generic
 * card so the UI carries NAVI meaning.
 */
export function NaviMetricCard({ label, value, sub, className }: NaviMetricCardProps) {
  return (
    <div className={clsx(styles.card, className)} role="group" aria-label={label}>
      <span className={styles.label}>{label}</span>
      <span className={styles.value}>{value}</span>
      {sub ? <span className={styles.sub}>{sub}</span> : null}
    </div>
  );
}
