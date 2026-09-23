import { existsSync, readFileSync } from 'node:fs'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// Optional local HTTPS for testing on a phone without a reverse proxy:
// if .cert/key.pem and .cert/cert.pem exist, the preview serves HTTPS.
const certDir = new URL('./.cert/', import.meta.url)
const keyFile = new URL('key.pem', certDir)
const certFile = new URL('cert.pem', certDir)
const https =
  existsSync(keyFile) && existsSync(certFile)
    ? { key: readFileSync(keyFile), cert: readFileSync(certFile) }
    : undefined

// The StashBert Go server (make run). Proxying /api keeps the requests
// same-origin, so the server needs no CORS headers.
const proxy = { '/api': 'http://localhost:8080' }

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: { proxy },
  preview: {
    // The reverse proxy reaches the preview under its own hostname.
    allowedHosts: true,
    https,
    proxy,
  },
})
