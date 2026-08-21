import { defineConfig } from '@playwright/test';

export default defineConfig({
	testDir: './tests/e2e',
	timeout: 60_000,
	fullyParallel: false,
	use: {
		baseURL: process.env.PLAYWRIGHT_BASE_URL ?? 'https://127.0.0.1:5173',
		ignoreHTTPSErrors: true,
		trace: 'retain-on-failure'
	},
	projects: [
		{
			name: 'chromium',
			use: {
				browserName: 'chromium',
				launchOptions: {
					args: ['--use-fake-device-for-media-stream', '--use-fake-ui-for-media-stream']
				}
			}
		}
	]
});
