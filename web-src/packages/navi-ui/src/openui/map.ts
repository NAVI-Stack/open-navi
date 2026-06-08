// Map an OpenUI Lang parse tree (ElementNode) onto NAVI's UI Spec (NaviUINode).
//
// The OpenUI parser has already turned positional call args into named props
// (per `naviOpenUISchema`), so here we only translate component `typeName` +
// `props` into NAVI nodes and recurse through child arrays. The output is then
// validated by `naviUINodeSchema` in the engine — so this mapper stays lenient
// (it may emit a node missing a required field on a partial/streaming tree; the
// engine prunes it to the valid prefix).

import type { ElementNode } from '@openuidev/react-lang';
import type { NaviUINode, NaviUIAction } from '../genui/schema';

function str(v: unknown): string | undefined {
  return typeof v === 'string' ? v : undefined;
}
function bool(v: unknown): boolean | undefined {
  if (typeof v === 'boolean') return v;
  if (v === 'true') return true;
  if (v === 'false') return false;
  return undefined;
}

function isElement(v: unknown): v is ElementNode {
  return !!v && typeof v === 'object' && (v as ElementNode).type === 'element' && typeof (v as ElementNode).typeName === 'string';
}

/** Translate a child collection (array, single element, or bare string) into nodes. */
function mapChildren(value: unknown): NaviUINode[] {
  const items = Array.isArray(value) ? value : value == null ? [] : [value];
  const out: NaviUINode[] = [];
  for (const item of items) {
    if (isElement(item)) {
      const mapped = mapOpenUIElement(item);
      if (mapped) out.push(mapped);
    } else if (typeof item === 'string' && item.trim()) {
      out.push({ t: 'text', value: item });
    }
  }
  return out;
}

function action(kind: NaviUIAction['kind'], intent: unknown): NaviUIAction | undefined {
  const i = str(intent);
  return i ? { kind, intent: i } : undefined;
}

function num(v: unknown): number {
  const n = typeof v === 'number' ? v : Number(v);
  return Number.isFinite(n) ? n : 0;
}

/** Normalize a prop into an array of child ElementNodes (drops non-elements). */
function elementsOf(value: unknown): ElementNode[] {
  const items = Array.isArray(value) ? value : value == null ? [] : [value];
  return items.filter(isElement);
}

/** Bars of a UsageChart: each Bar(label, value) → a {label, value} datum. */
function barsFrom(value: unknown): Array<{ label: string; value: number }> {
  return elementsOf(value)
    .filter((el) => el.typeName.toLowerCase() === 'bar')
    .map((el) => ({ label: str(el.props?.label) ?? '', value: num(el.props?.value) }));
}

/** Cells of a single Row element → their string values. */
function cellStrings(rowValue: unknown): string[] {
  const rowEl = elementsOf(rowValue)[0];
  if (!rowEl) return [];
  return elementsOf(rowEl.props?.cells).map((cell) => str(cell.props?.value) ?? '');
}

/** A list of Row elements → a grid of string cells. */
function tableRows(value: unknown): string[][] {
  return elementsOf(value).map((rowEl) =>
    elementsOf(rowEl.props?.cells).map((cell) => str(cell.props?.value) ?? ''),
  );
}

/**
 * Convert a single OpenUI `ElementNode` into a `NaviUINode`. Returns `null` for
 * an unknown component type. Lenient on props (a partial node may omit required
 * fields); downstream Zod validation enforces correctness.
 */
export function mapOpenUIElement(node: ElementNode): NaviUINode | null {
  const p = node.props ?? {};
  switch (node.typeName.toLowerCase()) {
    case 'text':
      return { t: 'text', value: str(p.value) as string, tone: p.tone as never, size: p.size as never, weight: p.weight as never };
    case 'heading': {
      const level = typeof p.level === 'number' ? p.level : Number(p.level);
      return { t: 'heading', value: str(p.value) as string, level: (level === 1 || level === 2 || level === 3 ? level : undefined) as never };
    }
    case 'markdown':
      return { t: 'markdown', value: str(p.value) as string };
    case 'badge':
      return { t: 'badge', label: str(p.label) as string, status: p.status as never };
    case 'button':
      return { t: 'button', label: str(p.label) as string, variant: p.variant as never, action: action('event', p.intent) };
    case 'field':
      return {
        t: 'field',
        name: str(p.name) as string,
        label: str(p.label),
        placeholder: str(p.placeholder),
        description: str(p.description),
        inputType: p.inputType as never,
        required: bool(p.required),
      };
    case 'divider':
      return { t: 'divider' };
    case 'stack':
      return {
        t: 'stack',
        dir: p.dir as never,
        gap: p.gap as never,
        align: p.align as never,
        justify: p.justify as never,
        wrap: bool(p.wrap),
        children: mapChildren(p.children),
      };
    case 'card':
      return { t: 'card', title: str(p.title), children: mapChildren(p.children) };
    case 'list':
      return { t: 'list', ordered: bool(p.ordered), items: mapChildren(p.items) };
    case 'form': {
      const act = action('submit', p.intent);
      return { t: 'form', action: act as NaviUIAction, submitLabel: str(p.submitLabel), children: mapChildren(p.children) };
    }
    case 'metriccard':
      return { t: 'metricCard', label: str(p.label) as string, value: (str(p.value) ?? '') as string, sub: str(p.sub) };
    case 'usagechart':
      return { t: 'usageChart', title: str(p.title), series: barsFrom(p.bars) };
    case 'datatable':
      return { t: 'dataTable', title: str(p.title), columns: cellStrings(p.head), rows: tableRows(p.rows) };
    // Bar / Row / Cell are consumed by their parent (UsageChart / DataTable) and
    // never render standalone.
    case 'bar':
    case 'row':
    case 'cell':
      return null;
    default:
      return null;
  }
}
