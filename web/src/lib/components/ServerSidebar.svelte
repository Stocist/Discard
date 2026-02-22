<script lang="ts">
	import type { Server } from '$lib/types';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { listServers, createServer } from '$lib/api';

	let servers = $state<Server[]>([]);
	let showCreateModal = $state(false);
	let newServerName = $state('');
	let loading = $state(false);

	const currentServerId = $derived(page.params?.serverId ?? '');

	async function loadServers() {
		try {
			servers = await listServers();
		} catch (e) {
			console.error('Failed to load servers:', e);
		}
	}

	async function handleCreate() {
		if (!newServerName.trim() || loading) return;
		loading = true;
		try {
			const server = await createServer(newServerName.trim());
			servers = [...servers, server];
			showCreateModal = false;
			newServerName = '';
			goto(`/servers/${server.id}`);
		} catch (e) {
			console.error('Failed to create server:', e);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		loadServers();
	});
</script>

<nav class="server-bar">
	<div class="servers">
		{#each servers as server (server.id)}
			<button
				class="server-icon"
				class:active={currentServerId === server.id}
				title={server.name}
				onclick={() => goto(`/servers/${server.id}`)}
			>
				{server.name.charAt(0).toUpperCase()}
			</button>
		{/each}
	</div>

	<div class="separator"></div>

	<button class="server-icon add-btn" title="Create Server" onclick={() => (showCreateModal = true)}>
		+
	</button>
</nav>

{#if showCreateModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="modal-overlay" onclick={() => (showCreateModal = false)} onkeydown={(e) => e.key === 'Escape' && (showCreateModal = false)}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="modal" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
			<h2>Create a Server</h2>
			<form onsubmit={(e) => { e.preventDefault(); handleCreate(); }}>
				<input
					type="text"
					placeholder="Server name"
					bind:value={newServerName}
				/>
				<div class="modal-actions">
					<button type="button" class="cancel-btn" onclick={() => (showCreateModal = false)}>Cancel</button>
					<button type="submit" class="create-btn" disabled={!newServerName.trim() || loading}>
						{loading ? 'Creating...' : 'Create'}
					</button>
				</div>
			</form>
		</div>
	</div>
{/if}

<style>
	.server-bar {
		width: 72px;
		min-width: 72px;
		background: var(--bg-server-bar);
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 12px 0;
		gap: 8px;
		overflow-y: auto;
	}

	.servers {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
	}

	.server-icon {
		width: 48px;
		height: 48px;
		border-radius: 50%;
		background: var(--bg-sidebar);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 18px;
		font-weight: 600;
		color: var(--text-primary);
		transition: border-radius 0.15s;
	}

	.server-icon:hover,
	.server-icon.active {
		border-radius: 16px;
		background: var(--accent);
	}

	.add-btn {
		font-size: 24px;
		color: var(--accent);
	}

	.add-btn:hover {
		background: var(--accent);
		color: white;
		border-radius: 16px;
	}

	.separator {
		width: 32px;
		height: 2px;
		background: var(--bg-sidebar);
		border-radius: 1px;
	}

	.modal-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.7);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 100;
	}

	.modal {
		background: var(--bg-primary);
		border-radius: 8px;
		padding: 24px;
		width: 400px;
		max-width: 90vw;
	}

	.modal h2 {
		margin-bottom: 16px;
		font-size: 20px;
	}

	.modal input {
		width: 100%;
		padding: 10px 12px;
		background: var(--bg-input);
		border-radius: 4px;
		color: var(--text-primary);
		margin-bottom: 16px;
	}

	.modal-actions {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
	}

	.cancel-btn {
		padding: 8px 16px;
		color: var(--text-muted);
	}

	.cancel-btn:hover {
		color: var(--text-primary);
	}

	.create-btn {
		padding: 8px 16px;
		background: var(--accent);
		color: white;
		border-radius: 4px;
		font-weight: 500;
	}

	.create-btn:hover:not(:disabled) {
		background: var(--accent-hover);
	}

	.create-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
