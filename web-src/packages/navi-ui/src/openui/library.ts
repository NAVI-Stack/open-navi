// NAVI's OpenUI Lang vocabulary.
//
// OpenUI Lang's parser maps POSITIONAL call arguments onto named props using a
// component's declared property order, so the `properties` key order below is
// load-bearing — it must match the positional signature documented in the
// system prompt and consumed by `mapOpenUIElement`. `required` drives the
// parser's missing-arg validation. This is the minimal `LibraryJSONSchema`
// shape `createParser`/`createStreamingParser` consume (no React renderers or
// Zod needed — we render through NAVI primitives, not OpenUI's <Renderer>).

// The minimal `LibraryJSONSchema` shape `createParser` consumes. Declared
// locally rather than imported: `@openuidev/react-lang` doesn't re-export the
// type, and importing it from `@openuidev/lang-core` would add a hard
// dependency on a transitive package. Structurally assignable to the real type.
export type OpenUILibrarySchema = {
  $defs: Record<string, { properties?: Record<string, unknown>; required?: string[] }>;
};

/** Root component name OpenUI Lang programs must assign (`root = Card(...)`). */
export const NAVI_OPENUI_ROOT = 'Card';

/**
 * The OpenUI component library mirroring NAVI's UI Spec vocabulary. Property
 * order = positional argument order.
 */
export const naviOpenUISchema: OpenUILibrarySchema = {
  $defs: {
    Text: { properties: { value: {}, tone: {}, size: {}, weight: {} }, required: ['value'] },
    Heading: { properties: { value: {}, level: {} }, required: ['value'] },
    Markdown: { properties: { value: {} }, required: ['value'] },
    Badge: { properties: { label: {}, status: {} }, required: ['label'] },
    Button: { properties: { label: {}, variant: {}, intent: {} }, required: ['label'] },
    Field: {
      properties: { name: {}, label: {}, placeholder: {}, description: {}, inputType: {}, required: {} },
      required: ['name'],
    },
    Divider: { properties: {}, required: [] },
    Stack: {
      properties: { children: {}, dir: {}, gap: {}, align: {}, justify: {}, wrap: {} },
      required: [],
    },
    Card: { properties: { title: {}, children: {} }, required: [] },
    List: { properties: { items: {}, ordered: {} }, required: [] },
    Form: { properties: { intent: {}, children: {}, submitLabel: {} }, required: ['intent'] },
    // Data-driven render vocabulary (maps to NAVI domain components).
    MetricCard: { properties: { label: {}, value: {}, sub: {} }, required: ['label', 'value'] },
    UsageChart: { properties: { title: {}, bars: {} }, required: [] },
    Bar: { properties: { label: {}, value: {} }, required: ['label', 'value'] },
    DataTable: { properties: { title: {}, head: {}, rows: {} }, required: [] },
    Row: { properties: { cells: {} }, required: [] },
    Cell: { properties: { value: {} }, required: ['value'] },
  },
};

/** Component type names this library understands (used by the mapper). */
export const NAVI_OPENUI_COMPONENTS = Object.keys(naviOpenUISchema.$defs ?? {});

const SIGNATURES = [
  'Text(value: string, tone?, size?, weight?) — a line of text. tone: default|secondary|tertiary|accent|danger|success|warning',
  'Heading(value: string, level?: 1|2|3) — a section heading',
  'Markdown(value: string) — a Markdown block',
  'Badge(label: string, status?) — status: idle|running|blocked|completed|failed|warning|info|success|danger|muted|accent',
  'Button(label: string, variant?, intent?) — variant: primary|secondary|ghost|danger. intent is a semantic action name.',
  'Field(name: string, label?, placeholder?, description?, inputType?, required?) — a form input',
  'Divider() — a horizontal rule',
  'Stack(children?: Node[], dir?: "row"|"col", gap?, align?, justify?, wrap?) — layout container',
  'Card(title?: string, children?: Node[]) — a titled surface',
  'List(items?: Node[], ordered?: boolean) — a bulleted/numbered list',
  'Form(intent: string, children?: Node[], submitLabel?) — submits field values under the given intent',
  'MetricCard(label: string, value: string, sub?: string) — a headline metric tile',
  'UsageChart(title?: string, bars?: Bar[]) — a horizontal bar chart',
  'Bar(label: string, value: string) — one bar in a UsageChart',
  'DataTable(title?: string, head?: Row, rows?: Row[]) — a table; head is the header Row',
  'Row(cells?: Cell[]) — a table row',
  'Cell(value: string) — one table cell',
].join('\n');

/**
 * A grammar-accurate OpenUI Lang system prompt for NAVI's vocabulary. Mirrors
 * the conventions OpenUI's own prompt generator enforces (statement-per-line,
 * positional args, `root = Card(...)` first for streaming) so a model emits
 * OpenUI Lang that NAVI can render through its primitives.
 */
export function naviOpenUISystemPrompt(): string {
  return [
    'You render interfaces by emitting openui-lang, a declarative UI language.',
    'Your ENTIRE response must be valid openui-lang — no markdown, no prose, no code fences.',
    '',
    '## Syntax',
    '1. One statement per line: `identifier = Expression`.',
    '2. `root` is the entry point — every program must define `root = Card(...)`.',
    '3. Arguments are POSITIONAL (order matters); named/colon syntax is NOT supported.',
    '4. Reference other statements by identifier; every identifier except `root` must be',
    '   referenced by another statement or it is dropped.',
    '5. Write `root = Card(...)` FIRST so the shell streams in before its children.',
    '',
    '## Components (positional signatures)',
    SIGNATURES,
    '',
    'Interactivity is declarative: Button/Form carry a semantic `intent` (e.g. "create_project").',
    'NAVI surfaces the intent to the host; never embed code, URLs to scripts, or raw HTML.',
    '',
    '## Example',
    'root = Card("Create project", [intro, form, tags])',
    'intro = Text("Name your project.", "secondary")',
    'form = Form("create_project", [titleField], "Create")',
    'titleField = Field("title", "Project name", "e.g. Apollo")',
    'tags = Stack([act, draft], "row", "sm")',
    'act = Badge("ACT", "running")',
    'draft = Badge("draft", "idle")',
  ].join('\n');
}
