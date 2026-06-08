// Runtime smoke test for the built standalone artifact.
//
// Imports the *built* dist/index.js (the same module an external NAVI app
// consumes via the package `exports` map) and asserts the public API survived
// bundling + peer externalization. Run after `pnpm --filter @navi/ui run build`.
//
//   node scripts/smoke-dist.mjs
//
// Exits non-zero on any missing export so CI / the M2 verification gate fails
// loudly instead of shipping a broken bundle.
import { existsSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const distDir = resolve(here, '../dist');

for (const file of ['index.js', 'index.d.ts', 'navi-ui.css', 'tokens.css']) {
  if (!existsSync(resolve(distDir, file))) {
    console.error(`[smoke:dist] missing build output: dist/${file}`);
    process.exit(1);
  }
}

const ui = await import('../dist/index.js');

const required = [
  // primitives
  'Button', 'Card', 'StatusBadge', 'EmptyState', 'Stack', 'Text', 'Spinner', 'Field',
  // theme
  'applyTheme', 'ThemeProvider', 'createDefaultTheme',
  // genui
  'GenUI', 'naviSpecEngine', 'createOpenUIEngine', 'defaultRegistry', 'extendRegistry',
  'naviUINodeSchema', 'naviUIVocabularyPrompt',
];

const missing = required.filter((name) => ui[name] === undefined);
if (missing.length) {
  console.error('[smoke:dist] built artifact is missing public exports:', missing.join(', '));
  process.exit(1);
}

// Exercise a real code path through the bundle: the native engine should parse
// and validate a spec without the source tree present.
const parsed = ui.naviSpecEngine.parse({ t: 'text', value: 'hello from dist' });
if (!parsed.ok) {
  console.error('[smoke:dist] naviSpecEngine.parse failed on a valid spec:', parsed.error);
  process.exit(1);
}

console.log(`[smoke:dist] OK — ${required.length} public exports present; engine parse works.`);
