import clsx from 'clsx';
import styles from './NaviDataTable.module.css';

export interface NaviDataTableProps {
  title?: string;
  columns: string[];
  rows: Array<Array<string | number>>;
  className?: string;
}

/**
 * NaviDataTable — a compact, token-styled table for structured data views. A NAVI
 * domain component (not a generic <table> primitive) so data-driven surfaces share
 * one presentation. Renders an empty state when there are no rows.
 */
export function NaviDataTable({ title, columns, rows, className }: NaviDataTableProps) {
  return (
    <div className={clsx(styles.wrap, className)}>
      {title ? <div className={styles.title}>{title}</div> : null}
      {rows.length === 0 ? (
        <div className={styles.empty}>No rows.</div>
      ) : (
        <table className={styles.table}>
          <thead>
            <tr>
              {columns.map((col, i) => (
                <th key={`${col}-${i}`} scope="col">{col}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, r) => (
              <tr key={r}>
                {row.map((cell, c) => (
                  <td key={c}>{cell}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
