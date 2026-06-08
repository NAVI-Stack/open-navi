import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { NaviMetricCard } from './NaviMetricCard';
import { NaviUsageChart } from './NaviUsageChart';
import { NaviDataTable } from './NaviDataTable';
import { NaviToolUsagePanel } from '../patterns/NaviToolUsagePanel';
import type { NaviDataView } from '../genui/dataview';

describe('NAVI data components', () => {
  it('NaviMetricCard shows label, value and sub', () => {
    render(<NaviMetricCard label="Total tool calls" value={4} sub="across 2 tools" />);
    expect(screen.getByText('Total tool calls')).toBeInTheDocument();
    expect(screen.getByText('4')).toBeInTheDocument();
    expect(screen.getByText('across 2 tools')).toBeInTheDocument();
  });

  it('NaviUsageChart renders a bar per datum and an empty state', () => {
    const { rerender } = render(
      <NaviUsageChart title="Calls" series={[{ label: 'a', value: 3 }, { label: 'b', value: 1 }]} />,
    );
    expect(screen.getByText('a')).toBeInTheDocument();
    expect(screen.getByText('b')).toBeInTheDocument();
    rerender(<NaviUsageChart title="Calls" series={[]} />);
    expect(screen.getByText('No data to chart.')).toBeInTheDocument();
  });

  it('NaviDataTable renders columns and rows, with an empty state', () => {
    const { rerender } = render(
      <NaviDataTable columns={['Tool', 'Calls']} rows={[['read_file', 3]]} />,
    );
    expect(screen.getByText('Tool')).toBeInTheDocument();
    expect(screen.getByText('read_file')).toBeInTheDocument();
    rerender(<NaviDataTable columns={['Tool', 'Calls']} rows={[]} />);
    expect(screen.getByText('No rows.')).toBeInTheDocument();
  });

  it('NaviToolUsagePanel derives metric/chart/table from a data view', () => {
    const view: NaviDataView = {
      id: 'tool-usage',
      title: 'Tool usage in this chat',
      intent: 'chart',
      dataset: {
        columns: [
          { key: 'tool', label: 'Tool', type: 'string' },
          { key: 'count', label: 'Calls', type: 'number' },
        ],
        rows: [
          { tool: 'read_file', count: 3 },
          { tool: 'list_dir', count: 1 },
        ],
      },
    };
    render(<NaviToolUsagePanel view={view} />);
    expect(screen.getByText('Tool usage in this chat')).toBeInTheDocument();
    expect(screen.getByText('Total tool calls')).toBeInTheDocument();
    // total = 3 + 1
    expect(screen.getByText('4')).toBeInTheDocument();
  });
});
