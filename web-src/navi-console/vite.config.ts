/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const configDir = dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: [resolve(configDir, 'src/test/setup.ts')],
    css: false,
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
  },
  resolve: {
    // Consume the shared @navi/ui library from source. Order matters: the most
    // specific alias (the tokens stylesheet) must precede the package root.
    alias: [
      { find: '@navi/ui/tokens.css', replacement: resolve(configDir, '../packages/navi-ui/src/tokens/tokens.css') },
      // Opt-in OpenUI subpath — kept on source so the Console never depends on a
      // built @navi/ui (no build-order coupling). Must precede the package root.
      { find: '@navi/ui/openui', replacement: resolve(configDir, '../packages/navi-ui/src/openui/index.ts') },
      { find: '@navi/ui', replacement: resolve(configDir, '../packages/navi-ui/src/index.ts') },
      { find: '@', replacement: resolve(configDir, 'src') },
    ],
  },
  build: {
    outDir: resolve(configDir, '../../web'),
    emptyOutDir: false,
    sourcemap: true,
    chunkSizeWarningLimit: 1600,
    rollupOptions: {
      output: {
        entryFileNames: 'assets/[name]-[hash].js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]',
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:6284',
        changeOrigin: false,
      },
      '/ws': {
        target: 'ws://localhost:6284',
        ws: true,
      },
      '/health': {
        target: 'http://localhost:6284',
      },
      '/auth': {
        target: 'http://localhost:6284',
      },
    },
  },
});
