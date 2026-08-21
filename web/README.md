# Discard Web

The SvelteKit frontend for Discard uses Svelte 5, TypeScript, Vite, and the static adapter. In production its output is embedded in and served by the Go backend.

## Commands

```bash
npm ci
npm run dev
npm run check
npm run build
npm run test:e2e
```

Vite serves HTTPS because browser microphone access requires a secure context. API, upload, and WebSocket requests are proxied to `API_TARGET`, which defaults to `http://localhost:4000`.

The Playwright voice tests require a running development backend and PostgreSQL database. See the root `README.md` for complete setup instructions.
