import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// El binario de Go incrusta `ui/build` y sirve el panel bajo /_/.
// Todas las rutas de la SPA (assets, rutas internas) deben compartir ese prefijo.
export default defineConfig({
  base: '/_/',
  plugins: [svelte()],
  build: {
    outDir: 'build',
    emptyOutDir: true,
    assetsDir: 'assets',
    sourcemap: false,
    // CSP estricta del servidor: solo scripts y estilos externos del mismo origen.
    cssCodeSplit: false,
    rollupOptions: {
      output: {
        // Nombres con hash para que el HTML generado referencie /_/assets/...
        entryFileNames: 'assets/[name]-[hash].js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]',
      },
    },
  },
})
