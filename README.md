# Discard

A lightweight, self-hosted Discord alternative built for private friend groups running on Tailscale mesh networks.

## What is this?

Discard is a personalised chat platform — text, voice, video, streaming, and music — designed to run on your own server within a Tailscale network. No cloud dependencies, no data harvesting, no corporate oversight. Just you and your mates.

## Why not just use Discord?

- Discord's privacy track record is getting worse (data leaks, invasive age verification, ID collection)
- You don't control your data
- You're dependent on a company that can change terms, ban servers, or shut down
- Self-hosted alternatives that exist (Revolt/Stoat, Spacebar, Matrix) are either half-broken, too complex to set up, or missing key features

## Why not use existing self-hosted alternatives?


| Alternative             | Issue                                                                              |
| ----------------------- | ---------------------------------------------------------------------------------- |
| **Revolt/Stoat**        | Voice is broken, mid-migration, unstable                                           |
| **Spacebar**            | Voice/video experimental, no UDP support                                           |
| **Matrix/Element**      | Complex setup (Synapse + Element + Coturn + LiveKit), feels like Slack not Discord |
| **Stryve / Wrongthink** | Abandoned or minimal features                                                      |


None of these are built for a **trusted private network**. Discard assumes Tailscale handles networking, encryption, and identity — which eliminates the hardest parts of building a chat platform.

## Key differentiators

- **Zero-config networking** — Tailscale handles encryption (WireGuard), NAT traversal, and DNS. No TURN servers, no SSL certs, no port forwarding.
- **Easy setup** — Single Go binary + Postgres. One-command deploy.
- **Built for small groups** — Not trying to be Discord at scale. Optimised for private friend groups.
- **Music bot built-in** — Not a separate service to install. ffmpeg + yt-dlp integrated directly.
- **Tailscale-native** — Leverages Tailscale node sharing so friends don't need to be on the same Tailnet. Each friend has their own free Tailscale account, server is shared to them.

## Tech stack


| Component             | Technology                              |
| --------------------- | --------------------------------------- |
| Backend               | Go                                      |
| Database              | PostgreSQL                              |
| Real-time             | WebSockets                              |
| Voice/Video/Streaming | Pion WebRTC                             |
| Music                 | ffmpeg + yt-dlp + Opus codec            |
| Frontend              | SvelteKit (Svelte 5, TypeScript)        |
| File storage          | Disk (WebP conversion for images)       |
| Networking            | Tailscale (with optional fallback auth) |


## Target deployment

- Ubuntu 22.04 LTS server on a Tailscale meshnet
- Friends connect via browser at `http://discard.your-tailnet:4000`
- Node sharing used to give friends access without joining your Tailnet

## Quick start (planned)

```bash
# Install
curl -sL https://github.com/your-username/discard/releases/latest/install.sh | bash

# Run
discard --port 4000

# Open http://your-machine:4000 from any device on your Tailnet
```

## Documentation

Deployment guide coming soon.