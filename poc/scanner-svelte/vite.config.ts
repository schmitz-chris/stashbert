import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { existsSync, readFileSync } from 'node:fs';
import { defineConfig } from 'vite';

// Optional HTTPS for tests without the reverse proxy: self-signed files in .cert/ (not in Git).
const certKey = '.cert/key.pem';
const certFile = '.cert/cert.pem';
const https =
	existsSync(certKey) && existsSync(certFile)
		? { key: readFileSync(certKey), cert: readFileSync(certFile) }
		: undefined;

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) => filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// SPA mode: every URL is served by the fallback page, nothing is prerendered.
			adapter: adapter({ fallback: 'index.html' }),
			// The spec names public/ for static files (zxing_reader.wasm, beep.wav, icon, manifest).
			files: { assets: 'public' }
		})
	],
	preview: {
		allowedHosts: true,
		https
	}
});
