import type { NaviUINode } from './schema';

/** Result of an engine parsing a raw payload into a render tree. */
export type ParsedUI =
  | { ok: true; root: NaviUINode; warnings?: string[]; partial?: boolean }
  | { ok: false; error: string };

/**
 * A GenUIEngine turns an opaque payload (a NAVI spec, an OpenUI Lang string, or
 * any other generative-UI format) into a validated NaviUINode tree, and can
 * describe its vocabulary to an LLM. Engines are interchangeable behind <GenUI>,
 * so NAVI is never locked into a single generative-UI format.
 */
export interface GenUIEngine {
  /** Stable identifier, e.g. "navi-spec" or "openui". */
  readonly name: string;
  /** Parse + validate a payload into a render tree. Never throws. */
  parse(input: unknown): ParsedUI;
  /**
   * Optional tolerant parse for streaming / incomplete payloads: render the
   * valid prefix of a partially-arrived spec. Engines that support progressive
   * rendering set `partial: true` on the result while the stream is still
   * incomplete. <GenUI streaming> prefers this over `parse`. Never throws.
   */
  parsePartial?(input: unknown): ParsedUI;
  /** A system-prompt fragment describing what this engine can render. */
  systemPrompt(): string;
}
