// Concrete OpenUI Lang → NAVI adapter.
//
// Bridges thesysdev/openui (`@openuidev/react-lang`) into NAVI's GenUI pipeline:
// OpenUI parses the lang text into an ElementNode tree, `mapOpenUIElement`
// translates it to a NaviUINode, and NAVI's own registry + primitives render it
// (theming, a11y, and the safety envelope stay NAVI's). This module is the
// ONLY place that imports `@openuidev/react-lang`, and it lives behind the
// opt-in `@navi/ui/openui` subpath — the core library never depends on it.

import { createParser, createStreamingParser, type ElementNode, type ParseResult } from '@openuidev/react-lang';
import type { GenUIEngine } from '../genui/engine';
import { createOpenUIEngine, type OpenUILangAdapter } from '../genui/OpenUIEngine';
import type { NaviUINode } from '../genui/schema';
import { mapOpenUIElement } from './map';
import { naviOpenUISchema, naviOpenUISystemPrompt, NAVI_OPENUI_ROOT } from './library';

function rootOf(input: unknown, parse: (text: string) => ParseResult): ElementNode | null {
  if (typeof input === 'string') return parse(input).root;
  if (input && typeof input === 'object') {
    const obj = input as Record<string, unknown>;
    // An already-parsed ElementNode…
    if (obj.type === 'element' && typeof obj.typeName === 'string') return input as ElementNode;
    // …or a full ParseResult.
    if ('root' in obj) return (obj.root as ElementNode | null) ?? null;
  }
  return null;
}

/**
 * Build an {@link OpenUILangAdapter} that maps NAVI's OpenUI Lang vocabulary
 * onto NaviUINodes. Supports one-shot parsing (`toNaviSpec`) and streaming
 * (`toNaviSpecPartial`, via OpenUI's incremental parser) so partial specs
 * render progressively.
 */
export function createNaviOpenUIAdapter(): OpenUILangAdapter {
  const parser = createParser(naviOpenUISchema, NAVI_OPENUI_ROOT);
  const streaming = createStreamingParser(naviOpenUISchema, NAVI_OPENUI_ROOT);

  return {
    toNaviSpec(input: unknown): NaviUINode {
      const root = rootOf(input, (t) => parser.parse(t));
      if (!root) {
        throw new Error('OpenUI Lang produced no root — ensure the program defines `root = Card(...)`.');
      }
      const mapped = mapOpenUIElement(root);
      if (!mapped) throw new Error(`OpenUI root component "${root.typeName}" is not in NAVI's vocabulary.`);
      return mapped;
    },
    toNaviSpecPartial(input: unknown): NaviUINode {
      // `set` diffs against the buffer (append → incremental, replace → reset),
      // so feeding the growing full text each render is correct and cheap.
      const root = typeof input === 'string' ? streaming.set(input).root : rootOf(input, (t) => parser.parse(t));
      const mapped = root ? mapOpenUIElement(root) : null;
      // Empty shell while the root hasn't streamed in yet — renders nothing,
      // not an error; the engine prunes/validates from here.
      return mapped ?? { t: 'stack', children: [] };
    },
    systemPrompt: naviOpenUISystemPrompt,
  };
}

/**
 * Convenience: a ready-to-use OpenUI {@link GenUIEngine} for NAVI's vocabulary.
 *
 *   import { createNaviOpenUIEngine } from '@navi/ui/openui';
 *   <GenUI engine={createNaviOpenUIEngine()} source={openUILangText} streaming />
 */
export function createNaviOpenUIEngine(): GenUIEngine {
  return createOpenUIEngine(createNaviOpenUIAdapter());
}
