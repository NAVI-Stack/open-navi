import clsx from 'clsx';
import styles from './NaviUsageChart.module.css';

export interface NaviUsageBar {
  label: string;
  value: number;
}

export interface NaviUsageChartProps {
  title?: string;
  series: NaviUsageBar[];
  className?: string;
}

/**
 * NaviUsageChart — a dependency-free horizontal bar chart for usage/count data
 * (e.g. tool calls per tool). CSS-driven bars keep the bundle lean; the widest
 * bar fills the track and the rest scale proportionally. Renders an empty state
 * when there is no data rather than a blank chart.
 */
export function NaviUsageChart({ title, series, className }: NaviUsageChartProps) {
  const max = series.reduce((m, b) => (b.value > m ? b.value : m), 0);
  return (
    <figure className={clsx(styles.chart, className)} aria-label={title ?? 'Usage chart'}>
      {title ? <figcaption className={styles.title}>{title}</figcaption> : null}
      {series.length === 0 ? (
        <div className={styles.empty}>No data to chart.</div>
      ) : (
        <div className={styles.bars} role="list">
          {series.map((bar, i) => {
            const pct = max > 0 ? Math.max(2, Math.round((bar.value / max) * 100)) : 0;
            return (
              <div className={styles.row} role="listitem" key={`${bar.label}-${i}`}>
                <span className={styles.barLabel} title={bar.label}>{bar.label}</span>
                <span className={styles.track}>
                  <span
                    className={styles.bar}
                    style={{ width: `${pct}%` }}
                    aria-hidden="true"
                  />
                </span>
                <span className={styles.barValue}>{bar.value}</span>
              </div>
            );
          })}
        </div>
      )}
    </figure>
  );
}
