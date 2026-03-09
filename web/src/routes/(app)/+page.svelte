<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { listServers, joinServer } from '$lib/api';

	let loading = $state(true);
	let inviteCode = $state('');
	let joining = $state(false);

	async function init() {
		try {
			const servers = await listServers();
			if (servers.length > 0) {
				goto(`/servers/${servers[0].id}`, { replaceState: true });
				return;
			}
		} catch (e) {
			console.error('Failed to load servers:', e);
		}
		loading = false;
	}

	async function handleJoin() {
		if (!inviteCode.trim() || joining) return;
		joining = true;
		try {
			const server = await joinServer(inviteCode.trim());
			goto(`/servers/${server.id}`);
		} catch (e) {
			console.error('Failed to join server:', e);
		} finally {
			joining = false;
		}
	}

	onMount(() => {
		init();
	});
</script>

{#if loading}
	<div class="loading">
		<p>Loading...</p>
	</div>
{:else}
	<div class="welcome">
		<h1>Welcome to Discard</h1>
		<p class="subtitle">Create a server or join one with an invite code.</p>

		<div class="join-section">
			<h3>Join a Server</h3>
			<form onsubmit={(e) => { e.preventDefault(); handleJoin(); }}>
				<input
					type="text"
					placeholder="Enter invite code"
					bind:value={inviteCode}
				/>
				<button type="submit" class="join-btn" disabled={!inviteCode.trim() || joining}>
					{joining ? 'Joining...' : 'Join'}
				</button>
			</form>
		</div>

		<p class="hint">Or click the <strong>+</strong> button in the sidebar to create a new server.</p>
	</div>
{/if}

<style>
	.loading {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-muted);
	}

	.welcome {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 32px;
		text-align: center;
	}

	h1 {
		font-size: 28px;
		margin-bottom: 8px;
	}

	.subtitle {
		color: var(--text-muted);
		margin-bottom: 32px;
	}

	.join-section {
		background: var(--bg-sidebar);
		border-radius: 8px;
		padding: 24px;
		width: 400px;
		max-width: 90vw;
	}

	.join-section h3 {
		margin-bottom: 12px;
		font-size: 16px;
	}

	.join-section form {
		display: flex;
		gap: 8px;
	}

	.join-section input {
		flex: 1;
		padding: 10px 12px;
		background: var(--bg-input);
		border-radius: 4px;
		color: var(--text-primary);
	}

	.join-btn {
		padding: 10px 20px;
		background: var(--accent);
		color: white;
		border-radius: 4px;
		font-weight: 500;
	}

	.join-btn:hover:not(:disabled) {
		background: var(--accent-hover);
	}

	.join-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.hint {
		margin-top: 24px;
		color: var(--text-muted);
		font-size: 13px;
	}
</style>
