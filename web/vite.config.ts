import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
	const env = loadEnv(mode, '.', '');
	return {
		plugins: [sveltekit()],
		server: {
			proxy: {
				'/api': {
					target: env.API_TARGET || 'http://localhost:4000',
					changeOrigin: true,
					ws: true
				}
			}
		}
	};
});
