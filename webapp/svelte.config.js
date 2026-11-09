import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

// Use different base paths for development vs production
const isDev = process.env.NODE_ENV === 'development';
// Public prefix the app is served under, e.g. "/app/com.ictrek.garm".
// VITE_ so the same value reaches the client (src/lib/base-path.ts).
const appBasePath = process.env.VITE_GARM_BASE_PATH || '';
const basePath = isDev ? '' : `${appBasePath}/ui`;

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: 'index.html',
			precompress: false,
			strict: true
		}),
		paths: {
			base: basePath
		}
	}
};

export default config;
