// System-prompt fragment describing the NAVI UI Spec vocabulary. NAVI (or any
// LLM) is instructed to emit ONLY a JSON spec conforming to this grammar; the
// NaviSpecEngine validates it before rendering, so malformed output degrades to
// a visible error rather than executing anything.

const EXAMPLE = `{
  "t": "card",
  "title": "Create project",
  "children": [
    { "t": "text", "value": "Name your project and choose a mode.", "tone": "secondary" },
    {
      "t": "form",
      "action": { "intent": "create_project" },
      "submitLabel": "Create",
      "children": [
        { "t": "field", "name": "title", "label": "Project name", "placeholder": "e.g. Apollo", "required": true },
        { "t": "field", "name": "goal", "label": "Goal", "inputType": "text" }
      ]
    },
    {
      "t": "stack", "dir": "row", "gap": "sm",
      "children": [
        { "t": "badge", "label": "ACT", "status": "running" },
        { "t": "badge", "label": "draft", "status": "idle" }
      ]
    }
  ]
}`;

export function naviUIVocabularyPrompt(): string {
  return [
    'You can render an interface by returning a NAVI UI Spec: a single JSON object',
    'describing a tree of UI nodes. Return ONLY the JSON object, no prose, no code fences.',
    '',
    'Each node has a short type tag `t`. Available nodes:',
    '- text: { t:"text", value, tone?, size?, weight? } — tone: default|secondary|tertiary|accent|danger|success|warning',
    '- heading: { t:"heading", value, level?:1|2|3 }',
    '- markdown: { t:"markdown", value } — value is a Markdown string',
    '- badge: { t:"badge", label, status? } — status: idle|running|blocked|completed|failed|warning|info|success|danger|muted|accent',
    '- button: { t:"button", label, variant?, action? } — variant: primary|secondary|ghost|danger',
    '- field: { t:"field", name, label?, placeholder?, description?, inputType?, required? }',
    '- divider: { t:"divider" }',
    '- stack: { t:"stack", dir?:"row"|"col", gap?, align?, justify?, wrap?, children:[...] } — gap: none|xs|sm|md|lg|xl|2xl',
    '- card: { t:"card", title?, children:[...] }',
    '- list: { t:"list", ordered?, items:[...] }',
    '- form: { t:"form", action:{ intent, params?, href? }, submitLabel?, children:[...] }',
    '',
    'Interactivity is declarative: buttons and forms carry an `action` with a semantic',
    '`intent` (e.g. "create_project"). The host application decides what each intent does;',
    'never put code, URLs to scripts, or raw HTML in a spec. Form field values are sent',
    'with the submit action under `params`, keyed by each field `name`.',
    '',
    'Example:',
    EXAMPLE,
  ].join('\n');
}
