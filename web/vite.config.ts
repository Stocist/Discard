import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, loadEnv } from 'vite';
import basicSsl from '@vitejs/plugin-basic-ssl';

export default defineConfig(({ mode }) => {
	const env = loadEnv(mode, '.', '');
	return {
		plugins: [sveltekit(), basicSsl()],
		server: {
			proxy: {
				'/api': {
					target: env.API_TARGET || 'http://localhost:4000',
					changeOrigin: true,
					ws: true
				},
				'/uploads': {
					target: env.API_TARGET || 'http://localhost:4000',
					changeOrigin: true
				}
			}
		}
	};
});
