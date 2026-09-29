import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// Proxies every backend path to the local Go server during `npm run dev`,
// so the frontend gets live reload without rebuilding the Go binary.
const backendTarget = 'http://localhost:8080';
const proxiedPaths = ['/api', '/ui-config', '/healthz', '/docs'];

const targetConfig = { target: backendTarget, changeOrigin: true };

export default defineConfig({
  // The Go server serves the app under /ui/ (see internal/api/ui.go).
  base: '/ui/',
  plugins: [react()],
  build: {
    rolldownOptions: {
      output: {
        // Stable, named vendor chunks: they change only when the dependency
        // does, so a redeploy of the app code leaves them cached in browsers.
        codeSplitting: {
          groups: [
            {
              name: 'react-vendor',
              test: /node_modules[\\/](react|react-dom|react-router|react-router-dom|scheduler)[\\/]/,
            },
            {
              name: 'mui-vendor',
              test: /node_modules[\\/](@mui[\\/](material|system|utils|styled-engine|private-theming|types)|@emotion)[\\/]/,
            },
            { name: 'query-vendor', test: /node_modules[\\/]@tanstack[\\/]/ },
          ],
        },
      },
    },
  },
  server: {
    proxy: Object.fromEntries(proxiedPaths.map((path) => [path, targetConfig])),
  },
});
