import { naviUINodeSchema, type NaviUINode } from './schema';

// ---------------------------------------------------------------------------
// Streaming / progressive rendering support.
//
// Generative UI arrives token-by-token: a spec is rarely complete on the first
// chunk. Borrowing OpenUI's streaming-first philosophy, these helpers let an
// engine render the *valid prefix* of an incomplete payload instead of waiting
// for the whole tree (or flashing an error between chunks).
//
// Two independent concerns:
//   1. completeTruncatedJson — repair a truncated JSON *string* into the
//      longest parseable prefix (close an open string, balance brackets).
//   2. coerceValidPrefix — given a parsed-but-possibly-incomplete node tree,
//      keep only the leading children that validate, dropping a half-streamed
//      tail. Both are pure and never throw.
// ---------------------------------------------------------------------------

/**
 * Repair a possibly-truncated JSON string into the longest parseable prefix.
 *
 * Scans once to record, at every offset, the open-bracket stack and whether the
 * offset sits inside a string literal. Then walks cut points from the end
 * backwards, closing an open string and appending the matching brackets, and
 * returns the first candidate that `JSON.parse` accepts. Returns `null` when no
 * prefix is recoverable. Pure; never throws.
 */
export function completeTruncatedJson(src: string): string | null {
  const s = src.trim();
  if (!s) return null;

  const n = s.length;
  const stackAt: string[] = new Array(n + 1);
  const inStringAt: boolean[] = new Array(n + 1);
  const stack: string[] = [];
  let inString = false;
  let escaped = false;
  stackAt[0] = '';
  inStringAt[0] = false;
  for (let i = 0; i < n; i++) {
    const ch = s[i];
    if (inString) {
      if (escaped) escaped = false;
      else if (ch === '\\') escaped = true;
      else if (ch === '"') inString = false;
    } else if (ch === '"') {
      inString = true;
    } else if (ch === '{' || ch === '[') {
      stack.push(ch);
    } else if (ch === '}' || ch === ']') {
      stack.pop();
    }
    inStringAt[i + 1] = inString;
    stackAt[i + 1] = stack.join('');
  }

  for (let end = n; end >= 1; end--) {
    let candidate = s.slice(0, end);
    if (inStringAt[end]) candidate += '"'; // close a string we cut through
    candidate = candidate.replace(/[\s,:]+$/, ''); // drop a dangling separator
    const open = stackAt[end];
    for (let k = open.length - 1; k >= 0; k--) candidate += open[k] === '{' ? '}' : ']';
    try {
      JSON.parse(candidate);
      return candidate;
    } catch {
      /* try an earlier cut */
    }
  }
  return null;
}

/**
 * Coerce a parsed (possibly incomplete) value into the largest valid NaviUINode
 * prefix. Container children/items are pruned to their leading run of valid
 * nodes — a half-streamed final child is dropped rather than failing the whole
 * tree. Returns `null` if nothing valid can be salvaged yet. Pure.
 */
export function coerceValidPrefix(raw: unknown): NaviUINode | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const obj: Record<string, unknown> = { ...(raw as Record<string, unknown>) };
  if (typeof obj.t !== 'string') return null;
  if (Array.isArray(obj.children)) obj.children = leadingValid(obj.children);
  if (Array.isArray(obj.items)) obj.items = leadingValid(obj.items);
  const res = naviUINodeSchema.safeParse(obj);
  return res.success ? res.data : null;
}

/** Keep the leading run of children that each validate; stop at the first that
 *  doesn't (the truncated tail). */
function leadingValid(arr: unknown[]): NaviUINode[] {
  const out: NaviUINode[] = [];
  for (const item of arr) {
    const node = coerceValidPrefix(item);
    if (!node) break;
    out.push(node);
  }
  return out;
}
