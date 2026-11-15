/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// In development and preview, /v1 is proxied to fazd so the app and the API
// share an origin. In production, set VITE_API_URL to the API's origin.
const api = process.env.FAZ_API ?? 'http://127.0.0.1:8080';

export default defineConfig({
  plugins: [react()],
  build: {
    // Fonts stay separate files: unicode-range means a browser only fetches
    // the subsets a page uses, which inlining would defeat.
    assetsInlineLimit: (file) => (/\.woff2?$/.test(file) ? false : undefined),
  },
  server: { proxy: { '/v1': api, '/healthz': api } },
  preview: { proxy: { '/v1': api, '/healthz': api } },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: { modules: { classNameStrategy: 'non-scoped' } },
  },
});
