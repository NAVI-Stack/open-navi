import { NaviMetricCard } from '../primitives/NaviMetricCard';
import { NaviUsageChart, type NaviUsageBar } from '../primitives/NaviUsageChart';
import { NaviDataTable } from '../primitives/NaviDataTable';
import { firstColumnOfType, type NaviDataView } from '../genui/dataview';
import styles from './NaviToolUsagePanel.module.css';

export interface NaviToolUsagePanelProps {
  view: NaviDataView;
  className?: string;
}

/**
 * NaviToolUsagePanel renders a tool-usage NaviDataView (metric + bar chart +
 * table) directly from the canonical view. It is the NAVI-native composition used
 * when rendering a data view without OpenUI Lang (mode "navi-ui"), and the basis
 * for the OpenUI lane's component vocabulary. It derives axes generically: the
 * first string column is the label, the first number column the value.
 */
export function NaviToolUsagePanel({ view, className }: NaviToolUsagePanelProps) {
  const labelCol = firstColumnOfType(view, 'string');
  const valueCol = firstColumnOfType(view, 'number');

  const series: NaviUsageBar[] = labelCol && valueCol
    ? view.dataset.rows.map((row) => ({
        label: String(row[labelCol.key] ?? ''),
        value: Number(row[valueCol.key] ?? 0),
      }))
    : [];
  const total = series.reduce((sum, b) => sum + (Number.isFinite(b.value) ? b.value : 0), 0);

  const columns = view.dataset.columns.map((c) => c.label);
  const rows = view.dataset.rows.map((row) =>
    view.dataset.columns.map((c) => formatCell(row[c.key])),
  );

  const showChart = view.intent !== 'table' && series.length > 0;

  return (
    <section className={`${styles.panel}${className ? ` ${className}` : ''}`} aria-label={view.title}>
      <div className={styles.head}>{view.title}</div>
      <div className={styles.metrics}>
        <NaviMetricCard
          label="Total tool calls"
          value={total}
          sub={`across ${series.length} tool${series.length === 1 ? '' : 's'}`}
        />
      </div>
      {showChart ? <NaviUsageChart title="Calls per tool" series={series} /> : null}
      <NaviDataTable title="Details" columns={columns} rows={rows} />
    </section>
  );
}

function formatCell(value: unknown): string | number {
  if (value === null || value === undefined) return '—';
  if (typeof value === 'number' || typeof value === 'string') return value;
  return String(value);
}
