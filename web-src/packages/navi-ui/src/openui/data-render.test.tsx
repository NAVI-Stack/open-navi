import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { GenUI } from '../genui';
import { createNaviOpenUIEngine, mapOpenUIElement } from './index';

// Mirrors the OpenUI Lang the Go emitter (internal/navi/render/openui.go)
// produces for a tool-usage data view.
const TOOL_USAGE_PROGRAM = [
  'root = Card("Tool usage in this chat", [metric, chart, table])',
  'metric = MetricCard("Total tool calls", "4", "across 2 tools")',
  'chart = UsageChart("Calls per tool", [b0, b1])',
  'b0 = Bar("read_file", "3")',
  'b1 = Bar("list_dir", "1")',
  'c0 = Cell("Tool")',
  'c1 = Cell("Calls")',
  'head = Row([c0, c1])',
  'c2 = Cell("read_file")',
  'c3 = Cell("3")',
  'r0 = Row([c2, c3])',
  'table = DataTable("Details", head, [r0])',
].join('\n');

describe('OpenUI data-render vocabulary', () => {
  it('parses MetricCard/UsageChart/DataTable into a NAVI node tree', () => {
    const engine = createNaviOpenUIEngine();
    const res = engine.parse(TOOL_USAGE_PROGRAM);
    expect(res.ok).toBe(true);
    if (!res.ok) return;
    expect(res.root.t).toBe('card');
    if (res.root.t !== 'card') return;
    const types = res.root.children.map((c) => c.t);
    expect(types).toEqual(['metricCard', 'usageChart', 'dataTable']);
  });

  it('renders the data view through NAVI domain components', () => {
    const engine = createNaviOpenUIEngine();
    render(<GenUI engine={engine} source={TOOL_USAGE_PROGRAM} />);
    expect(screen.getByText('Total tool calls')).toBeInTheDocument();
    expect(screen.getByText('Calls per tool')).toBeInTheDocument();
    expect(screen.getByText('Details')).toBeInTheDocument();
    // The chart bar label and the table cell both say read_file.
    expect(screen.getAllByText('read_file').length).toBeGreaterThan(0);
  });

  it('maps a synthetic UsageChart element into a series', () => {
    const node = mapOpenUIElement({
      type: 'element',
      typeName: 'UsageChart',
      props: {
        title: 'Calls per tool',
        bars: [
          { type: 'element', typeName: 'Bar', props: { label: 'read_file', value: '3' }, partial: false },
          { type: 'element', typeName: 'Bar', props: { label: 'list_dir', value: '1' }, partial: false },
        ],
      },
      partial: false,
    });
    expect(node).toEqual({
      t: 'usageChart',
      title: 'Calls per tool',
      series: [
        { label: 'read_file', value: 3 },
        { label: 'list_dir', value: 1 },
      ],
    });
  });
});
