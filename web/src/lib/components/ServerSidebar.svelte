<script lang="ts">
	import { onMount } from 'svelte';
	import type { Server } from '$lib/types';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { listServers, createServer, fetchMe, deleteServer } from '$lib/api';
	import { subscribeServerEvents } from '$lib/ws';
	import ContextMenu from './ContextMenu.svelte';

	let servers = $state<Server[]>([]);
	let showCreateModal = $state(false);
	let newServerName = $state('');
	let loading = $state(false);
	let currentUserId = $state('');

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

	$effect(() => {
		fetchMe().then(u => { currentUserId = u.id; }).catch(() => {});
	});

	onMount(() => {
		return subscribeServerEvents((event) => {
			if (event.type === 'server_update') {
				servers = servers.map(s => s.id === event.server.id ? event.server : s);
			} else if (event.type === 'server_delete') {
				servers = servers.filter(s => s.id !== event.server_id);
				if (currentServerId === event.server_id) {
					goto('/');
				}
			}
		});
	});

	// Server context menu
	let serverCtx = $state<{ x: number; y: number; serverId: string } | null>(null);
	let confirmDeleteServerId = $state<string | null>(null);
	let deletingServer = $state(false);

	function handleServerContextMenu(e: MouseEvent, srvId: string) {
		e.preventDefault();
		serverCtx = { x: e.clientX, y: e.clientY, serverId: srvId };
	}

	function serverContextItems(srvId: string) {
		const srv = servers.find(s => s.id === srvId);
		const isOwner = !!srv && currentUserId === srv.owner_id;
		const items: { label: string; action: () => void; danger?: boolean }[] = [
			{ label: 'Copy Server ID', action: () => navigator.clipboard.writeText(srvId) },
		];
		if (isOwner) {
			items.push({ label: 'Server Settings', action: () => goto(`/servers/${srvId}?settings=1`) });
			items.push({ label: 'Delete Server', action: () => { confirmDeleteServerId = srvId; }, danger: true });
		}
		return items;
	}

	async function handleDeleteServer() {
		if (!confirmDeleteServerId || deletingServer) return;
		deletingServer = true;
		try {
			await deleteServer(confirmDeleteServerId);
			// WS broadcast will handle the removal from the list
			confirmDeleteServerId = null;
		} catch (e) {
			console.error('Failed to delete server:', e);
		} finally {
			deletingServer = false;
		}
	}

	function handleGlobalKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			if (confirmDeleteServerId) { confirmDeleteServerId = null; return; }
			if (showCreateModal) { showCreateModal = false; newServerName = ''; return; }
		}
	}
</script>

<svelte:window onkeydown={handleGlobalKeydown} />

<nav class="server-bar">
	<div class="servers">
		{#each servers as server (server.id)}
			<button
				class="server-icon"
				class:active={currentServerId === server.id}
				class:has-icon={!!server.icon_path}
				title={server.name}
				onclick={() => goto(`/servers/${server.id}`)}
				oncontextmenu={(e) => handleServerContextMenu(e, server.id)}
			>
				{#if server.icon_path}
					<img src={`/uploads/${server.icon_path}`} alt={server.name} class="server-img" />
				{:else}
					{server.name.charAt(0).toUpperCase()}
				{/if}
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
	<div class="modal-overlay" onclick={() => (showCreateModal = false)}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="modal" onclick={(e) => e.stopPropagation()}>
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

{#if serverCtx}
	<ContextMenu
		x={serverCtx.x}
		y={serverCtx.y}
		items={serverContextItems(serverCtx.serverId)}
		onClose={() => (serverCtx = null)}
	/>
{/if}

{#if confirmDeleteServerId}
	{@const serverToDelete = servers.find(s => s.id === confirmDeleteServerId)}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="modal-overlay" onclick={() => { confirmDeleteServerId = null; }}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="modal" onclick={(e) => e.stopPropagation()}>
			<h2>Delete Server</h2>
			<p class="delete-warning">Are you sure you want to delete <strong>{serverToDelete?.name ?? 'this server'}</strong>? All channels and messages will be permanently removed.</p>
			<div class="modal-actions">
				<button type="button" class="cancel-btn" onclick={() => (confirmDeleteServerId = null)}>Cancel</button>
				<button type="button" class="delete-btn" onclick={handleDeleteServer} disabled={deletingServer}>
					{deletingServer ? 'Deleting...' : 'Delete Server'}
				</button>
			</div>
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

	.server-icon.has-icon {
		padding: 0;
		overflow: hidden;
	}

	.server-img {
		width: 100%;
		height: 100%;
		object-fit: cover;
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

	.delete-warning {
		color: var(--text-muted);
		font-size: 14px;
		line-height: 1.5;
		margin-bottom: 16px;
	}

	.delete-btn {
		padding: 8px 16px;
		background: #ef4444;
		color: white;
		border-radius: 4px;
		font-weight: 500;
	}

	.delete-btn:hover:not(:disabled) {
		background: #dc2626;
	}

	.delete-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
