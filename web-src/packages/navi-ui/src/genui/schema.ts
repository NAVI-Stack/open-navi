import { z } from 'zod';

// ---------------------------------------------------------------------------
// NAVI UI Spec — the canonical, engine-agnostic description of a generated UI.
//
// A spec is a tree of nodes discriminated by a short `t` (type) tag. Short tags
// keep the wire form compact (a nod to OpenUI's token-efficiency goal) while the
// tree stays human-readable and safe: there is no embedded code or raw HTML, and
// behaviour is expressed declaratively via named `action` intents that the host
// app wires up. This keeps generated UI inside NAVI's safety envelope.
// ---------------------------------------------------------------------------

/** Declarative action. The host interprets `intent`; the library never executes code. */
export const naviUIActionSchema = z.object({
  kind: z.enum(['event', 'submit', 'navigate']).default('event'),
  /** Semantic name the host app knows how to handle (e.g. "create_project"). */
  intent: z.string().min(1),
  /** Static parameters carried with the action (merged with form values for forms). */
  params: z.record(z.unknown()).optional(),
  /** Optional target for `navigate` actions. */
  href: z.string().optional(),
});
export type NaviUIAction = z.infer<typeof naviUIActionSchema>;

export type TextTone = 'default' | 'secondary' | 'tertiary' | 'accent' | 'danger' | 'success' | 'warning';
export type TextSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
export type TextWeight = 'normal' | 'medium' | 'semibold' | 'bold';
export type BadgeStatus =
  | 'idle' | 'running' | 'blocked' | 'completed' | 'failed' | 'warning'
  | 'info' | 'default' | 'danger' | 'success' | 'muted' | 'accent';
export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
export type GapToken = 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl' | '2xl';
export type FieldInputType = 'text' | 'email' | 'password' | 'number' | 'url' | 'tel' | 'search';

export interface TextNode { t: 'text'; value: string; tone?: TextTone; size?: TextSize; weight?: TextWeight }
export interface HeadingNode { t: 'heading'; value: string; level?: 1 | 2 | 3 }
export interface MarkdownNode { t: 'markdown'; value: string }
export interface BadgeNode { t: 'badge'; label: string; status?: BadgeStatus }
export interface ButtonNode { t: 'button'; label: string; variant?: ButtonVariant; action?: NaviUIAction }
export interface FieldNode {
  t: 'field';
  name: string;
  label?: string;
  placeholder?: string;
  description?: string;
  inputType?: FieldInputType;
  required?: boolean;
}
export interface DividerNode { t: 'divider' }
export interface StackNode {
  t: 'stack';
  dir?: 'row' | 'col';
  gap?: GapToken;
  align?: 'start' | 'center' | 'end' | 'stretch' | 'baseline';
  justify?: 'start' | 'center' | 'end' | 'between' | 'around';
  wrap?: boolean;
  children: NaviUINode[];
}
export interface CardNode { t: 'card'; title?: string; children: NaviUINode[] }
export interface ListNode { t: 'list'; ordered?: boolean; items: NaviUINode[] }
export interface FormNode { t: 'form'; action: NaviUIAction; submitLabel?: string; children: NaviUINode[] }

// --- Data-driven render nodes (NAVI-native components) -----------------------
// These map to NAVI domain components (NaviMetricCard / NaviUsageChart /
// NaviDataTable) so data views render as NAVI meaning, not generic primitives.
export interface MetricCardNode { t: 'metricCard'; label: string; value: string | number; sub?: string }
export interface UsageBar { label: string; value: number }
export interface UsageChartNode { t: 'usageChart'; title?: string; series: UsageBar[] }
export interface DataTableNode { t: 'dataTable'; title?: string; columns: string[]; rows: Array<Array<string | number>> }

export type NaviUINode =
  | TextNode | HeadingNode | MarkdownNode | BadgeNode | ButtonNode | FieldNode
  | DividerNode | StackNode | CardNode | ListNode | FormNode
  | MetricCardNode | UsageChartNode | DataTableNode;

/** Every node type the default registry understands. */
export const NAVI_UI_NODE_TYPES = [
  'text', 'heading', 'markdown', 'badge', 'button', 'field',
  'divider', 'stack', 'card', 'list', 'form',
  'metricCard', 'usageChart', 'dataTable',
] as const;

const toneEnum = z.enum(['default', 'secondary', 'tertiary', 'accent', 'danger', 'success', 'warning']);
const sizeEnum = z.enum(['xs', 'sm', 'md', 'lg', 'xl', '2xl']);
const weightEnum = z.enum(['normal', 'medium', 'semibold', 'bold']);
const statusEnum = z.enum([
  'idle', 'running', 'blocked', 'completed', 'failed', 'warning',
  'info', 'default', 'danger', 'success', 'muted', 'accent',
]);
const buttonVariantEnum = z.enum(['primary', 'secondary', 'ghost', 'danger']);
const gapEnum = z.enum(['none', 'xs', 'sm', 'md', 'lg', 'xl', '2xl']);
const alignEnum = z.enum(['start', 'center', 'end', 'stretch', 'baseline']);
const justifyEnum = z.enum(['start', 'center', 'end', 'between', 'around']);
const inputTypeEnum = z.enum(['text', 'email', 'password', 'number', 'url', 'tel', 'search']);

/**
 * Runtime validator for a NAVI UI node tree (recursive). The input generic is
 * relaxed to `unknown` because `action.kind` carries a `.default()`, which makes
 * the schema's input type diverge from its (fully-populated) output type.
 */
export const naviUINodeSchema: z.ZodType<NaviUINode, z.ZodTypeDef, unknown> = z.lazy(() =>
  z.discriminatedUnion('t', [
    z.object({ t: z.literal('text'), value: z.string(), tone: toneEnum.optional(), size: sizeEnum.optional(), weight: weightEnum.optional() }),
    z.object({ t: z.literal('heading'), value: z.string(), level: z.union([z.literal(1), z.literal(2), z.literal(3)]).optional() }),
    z.object({ t: z.literal('markdown'), value: z.string() }),
    z.object({ t: z.literal('badge'), label: z.string(), status: statusEnum.optional() }),
    z.object({ t: z.literal('button'), label: z.string(), variant: buttonVariantEnum.optional(), action: naviUIActionSchema.optional() }),
    z.object({
      t: z.literal('field'),
      name: z.string(),
      label: z.string().optional(),
      placeholder: z.string().optional(),
      description: z.string().optional(),
      inputType: inputTypeEnum.optional(),
      required: z.boolean().optional(),
    }),
    z.object({ t: z.literal('divider') }),
    z.object({
      t: z.literal('stack'),
      dir: z.enum(['row', 'col']).optional(),
      gap: gapEnum.optional(),
      align: alignEnum.optional(),
      justify: justifyEnum.optional(),
      wrap: z.boolean().optional(),
      children: z.array(naviUINodeSchema),
    }),
    z.object({ t: z.literal('card'), title: z.string().optional(), children: z.array(naviUINodeSchema) }),
    z.object({ t: z.literal('list'), ordered: z.boolean().optional(), items: z.array(naviUINodeSchema) }),
    z.object({ t: z.literal('form'), action: naviUIActionSchema, submitLabel: z.string().optional(), children: z.array(naviUINodeSchema) }),
    z.object({ t: z.literal('metricCard'), label: z.string(), value: z.union([z.string(), z.number()]), sub: z.string().optional() }),
    z.object({
      t: z.literal('usageChart'),
      title: z.string().optional(),
      series: z.array(z.object({ label: z.string(), value: z.number() })),
    }),
    z.object({
      t: z.literal('dataTable'),
      title: z.string().optional(),
      columns: z.array(z.string()),
      rows: z.array(z.array(z.union([z.string(), z.number()]))),
    }),
  ]),
);

/** Optional envelope form: `{ root: <node> }`. Bare nodes are also accepted. */
export const naviUISpecSchema = z.union([
  naviUINodeSchema,
  z.object({ root: naviUINodeSchema }),
]);
export type NaviUISpec = z.infer<typeof naviUISpecSchema>;
