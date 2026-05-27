import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    // The relay's cors_origins must include http://localhost:5173
    // See config.yaml at the monorepo root
  },
});
