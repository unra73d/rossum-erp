import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// The build lands in web/dist, which the Go binary embeds. In dev, Vite serves
// the UI and proxies the API to the Go process on :8080.
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../web/dist',
    // emptyOutDir wipes web/dist, including the .gitkeep that go:embed needs
    // to compile before the UI has ever been built. ui/public/.gitkeep is
    // copied back in on every build, so a fresh clone always compiles.
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
