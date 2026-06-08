import { naviUINodeSchema, type NaviUINode } from './schema';
import type { GenUIEngine, ParsedUI } from './engine';
import { naviUIVocabularyPrompt } from './prompt';
import { coerceValidPrefix } from './partial';

/**
 * Adapter contract for plugging thesysdev/openui (OpenUI Lang) — or any other
 * external generative-UI format — into <GenUI>. The host installs the external
 * package (e.g. `@openuidev/react-lang`) and provides `toNaviSpec`, which maps
 * that format's parsed output onto a NaviUINode tree. The library itself stays
 * dependency-free, so the build never depends on an external GenUI engine.
 *
 * See docs/architecture/navi-ui-library.md → "OpenUI integration strategy".
 */
export interface OpenUILangAdapter {
  /** Map an OpenUI Lang payload (string or parsed AST) into a NAVI UI node. */
  toNaviSpec(input: unknown): NaviUINode;
  /**
   * Optional: map a *partial* (still-streaming) OpenUI Lang payload into a node
   * tree. The result may be incomplete — the engine prunes it to the valid
   * prefix. Defaults to `toNaviSpec` when omitted.
   */
  toNaviSpecPartial?(input: unknown): NaviUINode;
  /** Optional: a tailored system prompt for the OpenUI vocabulary. */
  systemPrompt?(): string;
}

/**
 * Build a GenUIEngine backed by OpenUI. Without an adapter the engine is inert
 * and reports a clear, actionable error instead of throwing — the seam exists
 * and is typed; enabling it is an install + wiring step, not a code change here.
 */
export function createOpenUIEngine(adapter?: OpenUILangAdapter): GenUIEngine {
  return {
    name: 'openui',
    parse(input: unknown): ParsedUI {
      if (!adapter) {
        return {
          ok: false,
          error:
            'OpenUI engine is not configured. Install `@openuidev/react-lang` and pass an ' +
            'adapter: createOpenUIEngine({ toNaviSpec }). See docs/architecture/navi-ui-library.md.',
        };
      }
      try {
        const mapped = adapter.toNaviSpec(input);
        const result = naviUINodeSchema.safeParse(mapped);
        if (!result.success) {
          return { ok: false, error: 'OpenUI adapter produced a spec that failed NAVI validation.' };
        }
        return { ok: true, root: result.data };
      } catch (e) {
        return { ok: false, error: `OpenUI parse failed: ${(e as Error).message}` };
      }
    },
    parsePartial(input: unknown): ParsedUI {
      if (!adapter) {
        return {
          ok: false,
          error:
            'OpenUI engine is not configured. Install `@openuidev/react-lang` and pass an ' +
            'adapter: createOpenUIEngine({ toNaviSpec }). See docs/architecture/navi-ui-library.md.',
        };
      }
      try {
        const mapped = (adapter.toNaviSpecPartial ?? adapter.toNaviSpec)(input);
        const root = coerceValidPrefix(mapped);
        if (!root) return { ok: false, error: 'OpenUI stream has no valid prefix yet.' };
        return { ok: true, root, partial: true };
      } catch (e) {
        return { ok: false, error: `OpenUI partial parse failed: ${(e as Error).message}` };
      }
    },
    systemPrompt() {
      return adapter?.systemPrompt?.() ?? naviUIVocabularyPrompt();
    },
  };
}
