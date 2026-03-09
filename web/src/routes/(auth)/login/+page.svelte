<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { fetchTailscaleStatus } from '$lib/api';

	let checking = $state(false);
	let errorMessage = $state('');

	const reason = $derived(new URL($page.url).searchParams.get('reason'));

	async function handleLogin() {
		checking = true;
		errorMessage = '';
		try {
			const status = await fetchTailscaleStatus();
			if (status.authenticated) {
				goto('/', { replaceState: true });
			} else if (status.on_tailscale) {
				errorMessage = 'You are on the Tailscale network but not authorized for this server. Ask the admin to share this node with you.';
			} else {
				errorMessage = 'Could not detect a Tailscale connection. Make sure Tailscale is running and you are connected to the network.';
			}
		} catch {
			errorMessage = 'Unable to reach the server. Check your connection and try again.';
		} finally {
			checking = false;
		}
	}

	onMount(async () => {
		try {
			const status = await fetchTailscaleStatus();
			if (status.authenticated) {
				goto('/', { replaceState: true });
			}
		} catch {
			// Stay on login page
		}
	});
</script>

<div class="login-page">
	<div class="login-card">
		<div class="logo-section">
			<h1 class="logo">Discard</h1>
			<p class="tagline">A private space for your friend group</p>
		</div>

		<div class="features">
			<div class="feature">
				<span class="feature-icon">#</span>
				<span>Text channels with markdown, file sharing, and images</span>
			</div>
			<div class="feature">
				<span class="feature-icon">&#x266a;</span>
				<span>Voice channels for hanging out together</span>
			</div>
			<div class="feature">
				<span class="feature-icon">&#x26BF;</span>
				<span>Private and secure on your own Tailscale network</span>
			</div>
		</div>

		{#if reason === 'unauthenticated'}
			<div class="notice">
				Connect to the Tailscale network to continue.
			</div>
		{:else if reason === 'not_invited'}
			<div class="notice">
				You're on Tailscale, but this node hasn't been shared with you yet. Ask the admin for access.
			</div>
		{/if}

		{#if errorMessage}
			<div class="error-message">{errorMessage}</div>
		{/if}

		<button class="login-btn" onclick={handleLogin} disabled={checking}>
			{#if checking}
				Checking connection...
			{:else}
				Login with Tailscale
			{/if}
		</button>

		<div class="help-section">
			<p class="help-title">Don't have Tailscale?</p>
			<p class="help-text">
				Tailscale creates a private network between your devices. It's free and takes 2 minutes to set up.
			</p>
			<a
				class="help-link"
				href="https://tailscale.com/download"
				target="_blank"
				rel="noopener noreferrer"
			>
				Install Tailscale
			</a>
		</div>
	</div>
</div>

<style>
	.login-page {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 100vh;
		background: var(--bg-primary);
		padding: 24px;
	}

	.login-card {
		width: 100%;
		max-width: 440px;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 28px;
	}

	.logo-section {
		text-align: center;
	}

	.logo {
		font-size: 42px;
		font-weight: 700;
		color: var(--accent);
		letter-spacing: -1px;
	}

	.tagline {
		color: var(--text-muted);
		font-size: 16px;
		margin-top: 6px;
	}

	.features {
		display: flex;
		flex-direction: column;
		gap: 12px;
		width: 100%;
		background: var(--bg-secondary);
		border-radius: 8px;
		padding: 20px;
	}

	.feature {
		display: flex;
		align-items: center;
		gap: 12px;
		font-size: 14px;
		color: var(--text-primary);
	}

	.feature-icon {
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg-tertiary);
		border-radius: 6px;
		font-size: 14px;
		flex-shrink: 0;
	}

	.notice {
		width: 100%;
		padding: 12px 16px;
		background: var(--bg-secondary);
		border-left: 3px solid var(--accent);
		border-radius: 4px;
		color: var(--text-primary);
		font-size: 13px;
	}

	.error-message {
		width: 100%;
		padding: 12px 16px;
		background: rgba(220, 38, 38, 0.1);
		border-left: 3px solid #dc2626;
		border-radius: 4px;
		color: #fca5a5;
		font-size: 13px;
	}

	.login-btn {
		width: 100%;
		padding: 14px 24px;
		background: var(--accent);
		color: #1c1917;
		font-size: 16px;
		font-weight: 600;
		border-radius: 8px;
		cursor: pointer;
		transition: background 0.15s;
	}

	.login-btn:hover:not(:disabled) {
		background: var(--accent-hover);
	}

	.login-btn:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.help-section {
		text-align: center;
		padding: 20px;
		background: var(--bg-secondary);
		border-radius: 8px;
		width: 100%;
	}

	.help-title {
		font-size: 14px;
		font-weight: 600;
		color: var(--text-primary);
		margin-bottom: 6px;
	}

	.help-text {
		font-size: 13px;
		color: var(--text-muted);
		line-height: 1.5;
		margin-bottom: 12px;
	}

	.help-link {
		display: inline-block;
		padding: 8px 20px;
		background: var(--bg-tertiary);
		color: var(--accent);
		font-size: 13px;
		font-weight: 500;
		border-radius: 6px;
		transition: background 0.15s;
	}

	.help-link:hover {
		background: var(--border);
	}
</style>
