import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

// Public prefix the app is served under, e.g. "/app/com.ictrek.garm".
// VITE_ so the same value reaches the client (src/lib/base-path.ts).
//
// 生产构建由 Dockerfile 的 GARM_BASE_PATH 构建参数注入。
// 平台内联调（make vos-dev）时由 scripts/vos-dev.sh 注入，dev server 同样带前缀，
// 这样浏览器请求的资源路径与生产一致。
const appBasePath = process.env.VITE_GARM_BASE_PATH || '';
const isDev = process.env.NODE_ENV === 'development';
const basePath = appBasePath ? `${appBasePath}/ui` : isDev ? '' : '/ui';

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
