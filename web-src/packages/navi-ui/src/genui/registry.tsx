import type { CSSProperties, ReactNode } from 'react';
import { Stack } from '../primitives/Stack';
import { Card } from '../primitives/Card';
import { Text } from '../primitives/Text';
import { Button } from '../primitives/Button';
import { StatusBadge } from '../primitives/StatusBadge';
import { Field } from '../primitives/Field';
import { NaviMetricCard } from '../primitives/NaviMetricCard';
import { NaviUsageChart } from '../primitives/NaviUsageChart';
import { NaviDataTable } from '../primitives/NaviDataTable';
import type { NaviUINode, NaviUIAction } from './schema';

/** Invoked when a generated button/form dispatches its declarative action. */
export type GenUIActionHandler = (action: NaviUIAction, meta: { node: NaviUINode }) => void;

export interface NodeRenderContext {
  node: NaviUINode;
  render: (child: NaviUINode, key: number | string) => ReactNode;
  onAction?: GenUIActionHandler;
}

export type NodeRenderer = (ctx: NodeRenderContext) => ReactNode;
export type ComponentRegistry = Record<string, NodeRenderer>;

function pick<T extends NaviUINode['t']>(node: NaviUINode): Extract<NaviUINode, { t: T }> {
  return node as Extract<NaviUINode, { t: T }>;
}

const dividerStyle: CSSProperties = {
  border: 'none',
  borderTop: '1px solid var(--navi-border)',
  margin: '4px 0',
  width: '100%',
};

/**
 * Default mapping from spec node types to `@navi/ui` primitives. Apps extend or
 * override this with `extendRegistry` — e.g. the Console plugs its own Markdown
 * renderer in for the `markdown` node without forking the library.
 */
export const defaultRegistry: ComponentRegistry = {
  text: ({ node }) => {
    const n = pick<'text'>(node);
    return <Text size={n.size} tone={n.tone} weight={n.weight}>{n.value}</Text>;
  },
  heading: ({ node }) => {
    const n = pick<'heading'>(node);
    const size = n.level === 1 ? '2xl' : n.level === 3 ? 'lg' : 'xl';
    return <Text block size={size} weight="semibold">{n.value}</Text>;
  },
  markdown: ({ node }) => {
    const n = pick<'markdown'>(node);
    // Default: render as preformatted text. Apps override `markdown` to plug in
    // a full Markdown renderer (see the Console GenUI demo).
    return (
      <div style={{ whiteSpace: 'pre-wrap' }}>
        <Text>{n.value}</Text>
      </div>
    );
  },
  divider: () => <hr style={dividerStyle} />,
  badge: ({ node }) => {
    const n = pick<'badge'>(node);
    return <StatusBadge variant={n.status ?? 'default'} label={n.label} />;
  },
  button: ({ node, onAction }) => {
    const n = pick<'button'>(node);
    return (
      <Button
        variant={n.variant ?? 'secondary'}
        onPress={() => {
          if (n.action) onAction?.(n.action, { node: n });
        }}
      >
        {n.label}
      </Button>
    );
  },
  field: ({ node }) => {
    const n = pick<'field'>(node);
    return (
      <Field
        name={n.name}
        label={n.label}
        placeholder={n.placeholder}
        description={n.description}
        type={n.inputType}
        isRequired={n.required}
      />
    );
  },
  stack: (ctx) => {
    const n = pick<'stack'>(ctx.node);
    return (
      <Stack dir={n.dir} gap={n.gap} align={n.align} justify={n.justify} wrap={n.wrap}>
        {n.children.map((child, i) => ctx.render(child, i))}
      </Stack>
    );
  },
  card: (ctx) => {
    const n = pick<'card'>(ctx.node);
    return (
      <Card>
        <Stack gap="md">
          {n.title && <Text block size="lg" weight="semibold">{n.title}</Text>}
          {n.children.map((child, i) => ctx.render(child, i))}
        </Stack>
      </Card>
    );
  },
  list: (ctx) => {
    const n = pick<'list'>(ctx.node);
    const Tag = n.ordered ? 'ol' : 'ul';
    return (
      <Tag style={{ margin: 0, paddingLeft: '1.25rem' }}>
        {n.items.map((child, i) => (
          <li key={i}>{ctx.render(child, i)}</li>
        ))}
      </Tag>
    );
  },
  form: (ctx) => {
    const n = pick<'form'>(ctx.node);
    return (
      <form
        onSubmit={(e) => {
          e.preventDefault();
          const data = new FormData(e.currentTarget);
          const values = Object.fromEntries(data.entries());
          ctx.onAction?.({ ...n.action, params: { ...(n.action.params ?? {}), ...values } }, { node: n });
        }}
      >
        <Stack gap="md">
          {n.children.map((child, i) => ctx.render(child, i))}
          <div>
            <Button type="submit" variant="primary">{n.submitLabel ?? 'Submit'}</Button>
          </div>
        </Stack>
      </form>
    );
  },
  metricCard: ({ node }) => {
    const n = pick<'metricCard'>(node);
    return <NaviMetricCard label={n.label} value={n.value} sub={n.sub} />;
  },
  usageChart: ({ node }) => {
    const n = pick<'usageChart'>(node);
    return <NaviUsageChart title={n.title} series={n.series} />;
  },
  dataTable: ({ node }) => {
    const n = pick<'dataTable'>(node);
    return <NaviDataTable title={n.title} columns={n.columns} rows={n.rows} />;
  },
};

/** Compose a registry: `extendRegistry(defaultRegistry, { markdown: myRenderer })`. */
export function extendRegistry(base: ComponentRegistry, overrides: ComponentRegistry): ComponentRegistry {
  return { ...base, ...overrides };
}
