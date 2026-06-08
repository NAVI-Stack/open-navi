import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import dts from 'vite-plugin-dts';
import { copyFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const configDir = dirname(fileURLToPath(import.meta.url));
const r = (p: string) => resolve(configDir, p);

// Standalone library build for external NAVI consumers. In-repo apps consume
// @navi/ui as SOURCE (via their own alias), so this config is build-only and is
// intentionally separate from vite.config.ts (which serves vitest + Ladle).
// See docs/architecture/navi-ui-library.md → "Standalone library build".

// Peers + optional integrations are never bundled — consumers bring their own
// React/react-aria/icons, and the OpenUI integration is opt-in. clsx + zod ARE
// bundled (small, internal-only) so consumers don't have to install them.
const external = [
  'react',
  'react-dom',
  'react/jsx-runtime',
  'react-aria-components',
  'lucide-react',
  // OpenUI is an opt-in subpath; its parser is an optionalDependency and must
  // never be pulled into the core bundle.
  '@openuidev/react-lang',
];

/** Copy the design-token stylesheet into dist/ verbatim (it is not imported by
 *  the JS entry, so the bundler never sees it). */
function copyTokens() {
  return {
    name: 'navi-ui-copy-tokens',
    closeBundle() {
      copyFileSync(r('src/tokens/tokens.css'), r('dist/tokens.css'));
    },
  };
}

export default defineConfig({
  plugins: [
    react(),
    dts({
      include: ['src'],
      exclude: ['src/**/*.test.{ts,tsx}', 'src/**/*.stories.{ts,tsx}', 'src/test/**'],
      entryRoot: 'src',
      insertTypesEntry: true,
    }),
    copyTokens(),
  ],
  build: {
    lib: {
      entry: {
        index: r('src/index.ts'),
        // Opt-in OpenUI Lang integration — a separate entry so the core bundle
        // never pulls in @openuidev/react-lang.
        openui: r('src/openui/index.ts'),
      },
      formats: ['es'],
    },
    // One stylesheet for all component CSS Modules (tokens.css is copied
    // separately). External consumers import '@navi/ui/navi-ui.css'.
    cssCodeSplit: false,
    sourcemap: true,
    rollupOptions: {
      external,
      output: {
        assetFileNames: (asset) =>
          asset.names?.some((n) => n.endsWith('.css')) ? 'navi-ui.css' : '[name][extname]',
      },
    },
  },
});
