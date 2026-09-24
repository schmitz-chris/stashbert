import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import { VitePWA } from 'vite-plugin-pwa'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    // Service worker and web app manifest (plan F13b). The app registers the
    // service worker itself with useRegisterSW (components/UpdateBanner.tsx),
    // so index.html gets no inline script (CSP, architecture.md 8). "prompt"
    // keeps a new version from reloading the page in the middle of a scan.
    VitePWA({
      strategies: 'generateSW',
      registerType: 'prompt',
      injectRegister: false,
      filename: 'sw.js',
      manifestFilename: 'manifest.webmanifest',
      manifest: {
        name: 'StashBert',
        short_name: 'StashBert',
        lang: 'de',
        display: 'standalone',
        start_url: '/',
        scope: '/',
        // stone-50, the background of html and body, like theme-color in
        // index.html.
        theme_color: '#fafaf9',
        background_color: '#fafaf9',
        icons: [
          { src: '/icon-192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
          { src: '/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'any' },
        ],
      },
      // globPatterns already contain the icons; this keeps them from
      // appearing twice in the precache list.
      includeManifestIcons: false,
      workbox: {
        // All build files, including zxing_reader.wasm for the scanner
        // (architecture.md 4.3). The default limit of 2 MiB per file applies.
        globPatterns: ['**/*.{js,css,html,wasm,png,svg}'],
        // The startup images are loaded by iOS itself when the app is added
        // to the Home Screen, never by the page. Each iPhone needs only one
        // of them, so the service worker does not download both.
        globIgnores: ['startup-*.png'],
        navigateFallback: 'index.html',
        // The API is never answered from the service worker. There is no
        // runtime caching, so API requests always go to the network.
        navigateFallbackDenylist: [/^\/api\//],
      },
    }),
  ],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
