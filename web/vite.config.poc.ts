import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { viteSingleFile } from 'vite-plugin-singlefile'

// Offline demo: `npm run build:poc` -> one self-contained HTML file (JS, CSS, fonts, SVGs, fixtures inlined).
// vite-plugin-singlefile sets inlineDynamicImports, so here mermaid / echarts are inlined into the page too
// (still evaluated lazily). The regular app build (vite.config.ts) keeps them as separate lazy chunks.
export default defineConfig({
  plugins: [vue(), tailwindcss(), viteSingleFile()],
  resolve: { alias: { '@': '/src' } },
  build: {
    outDir: '/tmp/aumbry-poc/build',
    emptyOutDir: true,
    target: 'es2022',
    chunkSizeWarningLimit: 100_000,
    assetsInlineLimit: 100_000_000, // KaTeX fonts etc. as data URIs
    rollupOptions: { input: 'poc.html' },
  },
})
