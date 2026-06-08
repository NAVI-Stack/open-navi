/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const configDir = dirname(fileURLToPath(import.meta.url));

// Lightweight config shared by vitest AND Ladle. The standalone library build
// lives in vite.lib.config.ts (run via `pnpm build`) so its lib/dts settings
// never leak into the test runner or the Ladle dev server.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@navi/ui': resolve(configDir, 'src'),
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: [resolve(configDir, 'src/test/setup.ts')],
    css: false,
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
  },
});
