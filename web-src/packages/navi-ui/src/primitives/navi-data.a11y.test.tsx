import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { axe } from 'vitest-axe';
import { NaviMetricCard } from './NaviMetricCard';
import { NaviUsageChart } from './NaviUsageChart';
import { NaviDataTable } from './NaviDataTable';
import { NaviToolUsagePanel } from '../patterns/NaviToolUsagePanel';
import type { NaviDataView } from '../genui/dataview';

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

describe('NAVI data components — accessibility', () => {
  it('NaviMetricCard has no axe violations', async () => {
    const { container } = render(<NaviMetricCard label="Total tool calls" value={4} sub="across 2 tools" />);
    expect((await axe(container)).violations).toEqual([]);
  });

  it('NaviUsageChart has no axe violations', async () => {
    const { container } = render(
      <NaviUsageChart title="Calls per tool" series={[{ label: 'read_file', value: 3 }, { label: 'list_dir', value: 1 }]} />,
    );
    expect((await axe(container)).violations).toEqual([]);
  });

  it('NaviDataTable has no axe violations', async () => {
    const { container } = render(
      <NaviDataTable title="Details" columns={['Tool', 'Calls']} rows={[['read_file', 3], ['list_dir', 1]]} />,
    );
    expect((await axe(container)).violations).toEqual([]);
  });

  it('NaviToolUsagePanel has no axe violations', async () => {
    const { container } = render(<NaviToolUsagePanel view={view} />);
    expect((await axe(container)).violations).toEqual([]);
  });
});
