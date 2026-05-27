import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import { resolve } from 'path';

export default defineConfig({
  plugins: [vue()],
  build: {
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      name: 'IntakeVue',
      fileName: 'intake-vue',
      formats: ['es'],
    },
    rollupOptions: {
      // vue is a peer dep — do not bundle it
      external: ['vue'],
      output: {
        globals: {
          vue: 'Vue',
        },
      },
    },
    // Type declarations are emitted via vue-tsc -b in the "build" npm script.
    // Vite itself does not emit .d.ts files.
  },
  test: {
    environment: 'jsdom',
    globals: true,
    exclude: ['dist/**', 'node_modules/**'],
  },
});
