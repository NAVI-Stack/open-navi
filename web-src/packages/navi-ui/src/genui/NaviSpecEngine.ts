import type { ZodError } from 'zod';
import { naviUINodeSchema } from './schema';
import type { GenUIEngine, ParsedUI } from './engine';
import { naviUIVocabularyPrompt } from './prompt';
import { coerceValidPrefix, completeTruncatedJson } from './partial';

function formatZodError(err: ZodError): string {
  const first = err.issues[0];
  if (!first) return 'invalid NAVI UI spec';
  const path = first.path.length ? first.path.join('.') : '(root)';
  return `at ${path}: ${first.message}`;
}

/**
 * Parse a NAVI UI Spec. Accepts a node object, an `{ root: node }` envelope, or
 * a JSON string of either. Always returns a result — never throws — so streaming
 * / partial payloads degrade gracefully.
 */
export function parseNaviSpec(input: unknown): ParsedUI {
  let raw: unknown = input;

  if (typeof input === 'string') {
    const trimmed = input.trim();
    if (!trimmed) return { ok: false, error: 'empty spec' };
    try {
      raw = JSON.parse(trimmed);
    } catch (e) {
      return { ok: false, error: `invalid JSON: ${(e as Error).message}` };
    }
  }

  if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
    const record = raw as Record<string, unknown>;
    if (!('t' in record) && 'root' in record) {
      raw = record.root;
    }
  }

  const result = naviUINodeSchema.safeParse(raw);
  if (!result.success) {
    return { ok: false, error: formatZodError(result.error) };
  }
  return { ok: true, root: result.data };
}

/**
 * Tolerant parse for streaming / truncated NAVI specs. Repairs a partially
 * arrived JSON string into its longest parseable prefix, then renders the
 * valid leading nodes (dropping a half-streamed tail). Falls back to a strict
 * parse when the payload is already complete. Never throws.
 */
export function parseNaviSpecPartial(input: unknown): ParsedUI {
  // A complete payload parses strictly — no need to degrade.
  const strict = parseNaviSpec(input);
  if (strict.ok) return strict;

  let raw: unknown = input;
  if (typeof input === 'string') {
    const completed = completeTruncatedJson(input);
    if (completed === null) return { ok: false, error: 'incomplete spec: no recoverable prefix yet' };
    try {
      raw = JSON.parse(completed);
    } catch {
      return { ok: false, error: 'incomplete spec: unrecoverable' };
    }
  }

  if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
    const record = raw as Record<string, unknown>;
    if (!('t' in record) && 'root' in record) raw = record.root;
  }

  const root = coerceValidPrefix(raw);
  if (!root) return { ok: false, error: 'incomplete spec: no valid prefix yet' };
  return { ok: true, root, partial: true };
}

/** The native NAVI generative-UI engine. This is the default for <GenUI>. */
export const naviSpecEngine: GenUIEngine = {
  name: 'navi-spec',
  parse: parseNaviSpec,
  parsePartial: parseNaviSpecPartial,
  systemPrompt: naviUIVocabularyPrompt,
};
