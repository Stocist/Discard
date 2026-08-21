# Discard

Discard is a self-hosted chat application for private friend groups on Tailscale. A single Go server provides the API, WebSocket text chat, WebRTC voice and screen sharing, and the built Svelte frontend; PostgreSQL stores application data.

## Stack

|Component|Technology|
|-|-|
|Backend|Go 1.27, pgx, Gorilla WebSocket|
|Frontend|SvelteKit 2, Svelte 5, TypeScript, Vite 8|
|Database|PostgreSQL 17|
|Voice and screen sharing|Pion WebRTC with embedded TURN fallback|
|Authentication|Tailscale identity, with a development-only bypass|
|Deployment|Single binary, Docker, or Docker Compose|

## Requirements

- Go 1.27
- Node.js 24.19 or newer and npm 11
- PostgreSQL 17
- `ffmpeg`, `yt-dlp`, and `cwebp` for media processing
- Tailscale for normal authentication and private-network access

## Development

Create a PostgreSQL database and start the backend:

```bash
export DATABASE_URL='postgres://localhost:5432/discard?sslmode=disable'
export DISCARD_DEV=true
go run ./cmd/discard
```

In another terminal, install the frontend dependencies and start Vite:

```bash
cd web
npm ci
npm run dev
```

Open `https://localhost:5173`. Development mode exposes fixed test identities through `?dev_user=0` to `?dev_user=3`; never enable `DISCARD_DEV` in production.

## Testing

```bash
go test -race ./...
go vet ./...
cd web
npm ci
npm run check
npm run build
```

The Playwright suite requires PostgreSQL, the backend on port 4000, and the Vite HTTPS server on port 5173:

```bash
cd web
npx playwright install chromium
npm run test:e2e
```

The browser suite verifies bidirectional audio over both direct ICE and forced TURN relay connections.

## Docker Compose

Set the required secrets in an ignored `.env` file:

```dotenv
TS_AUTHKEY=tskey-auth-...
TURN_SECRET=replace-with-a-random-secret
```

Then start the Tailscale sidecar, application, and PostgreSQL:

```bash
docker compose up -d --build
```

Compose stores PostgreSQL data, uploads, and Tailscale state in named volumes. It does not publish the application on the host network; access is through the Tailscale node and `tailscale serve`.

## Configuration

|Variable|Default|Purpose|
|-|-|-|
|`PORT`|`4000`|Backend HTTP port|
|`DATABASE_URL`|`postgres://localhost:5432/discard?sslmode=disable`|PostgreSQL connection string|
|`UPLOAD_DIR`|`./uploads`|Uploaded-file directory|
|`DISCARD_DEV`|`false`|Disable Tailscale authentication and expose fixed development users|
|`DISCARD_PRODUCTION`|`false`|Prevent accidental use of development authentication in production|
|`TAILSCALE_API_URL`|Local Tailscale API|Override the identity API endpoint|
|`TAILSCALE_API_TOKEN`|Unset|Authenticate to the macOS Tailscale local API|
|`TURN_SECRET`|Random per startup|Shared secret used for ephemeral TURN credentials|
|`TURN_PUBLIC_IP`|Auto-detected tailnet IP|Override the client-reachable TURN relay address|
|`TURN_PORT`|`3478`|Embedded UDP TURN listener port|
|`API_TARGET`|`http://localhost:4000`|Vite development proxy target|

Set a persistent `TURN_SECRET` in deployed environments. Set `TURN_PUBLIC_IP` when automatic tailnet-address discovery is not suitable.

## Blocking Behavior

- Blocking prevents direct-message creation, sending, and editing in both directions.
- Existing direct-message history remains available, and either participant may delete their own messages.
- Presence is hidden in both directions.
- Shared-server messages remain visible. Only the blocker sees the blocked author's name and avatar anonymized.
- Unblocking restores direct-message sending without deleting or recreating the conversation.

## Production Build

```bash
make build
```

The frontend output is copied to `internal/frontend/build` for embedding in the production Go binary. The Dockerfile performs this process in reproducible build stages.
