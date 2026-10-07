import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// In dev, Vite serves the UI and proxies data to the Go server on :8080.
// The dashboard is reachable in dev at /__admin (see src/lib/admin-base.ts).
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/media': 'http://localhost:8080',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
  },
});
