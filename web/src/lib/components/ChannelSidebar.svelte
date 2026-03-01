<script lang="ts">
	import type { Channel, Server, User } from '$lib/types';
	import { createChannel, updateChannel, deleteChannel } from '$lib/api';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import ServerSettings from './ServerSettings.svelte';
	import ContextMenu from './ContextMenu.svelte';
	import UserProfile from './UserProfile.svelte';

	let { serverId, channels = $bindable(), serverName = $bindable(), server = $bindable(), isOwner = false, onserverdelete, unreadCounts = {}, currentUser = $bindable() }: {
		serverId: string;
		channels: Channel[];
		serverName: string;
		server?: Server;
		isOwner?: boolean;
		onserverdelete?: () => void;
		unreadCounts?: Record<string, number>;
		currentUser?: User | null;
	} = $props();

	let showSettings = $state(false);
	let showProfile = $state(false);

	const currentChannelId = $derived(page.params?.channelId ?? '');

	// Open settings modal when ?settings=1 is in the URL (from ServerSidebar context menu)
	$effect(() => {
		const url = page.url;
		if (url.searchParams.get('settings') === '1' && server && isOwner) {
			showSettings = true;
			// Remove the query param without triggering navigation
			const clean = new URL(url);
			clean.searchParams.delete('settings');
			history.replaceState({}, '', clean.pathname + clean.search);
		}
	});

	// Channel creation
	let showNewChannel = $state(false);
	let newChannelName = $state('');
	let creatingChannel = $state(false);

	async function handleCreateChannel() {
		const name = newChannelName.trim();
		if (!name || creatingChannel) return;
		creatingChannel = true;
		try {
			const ch = await createChannel(serverId, name);
			channels = [...channels, ch];
			newChannelName = '';
			showNewChannel = false;
			goto(`/servers/${serverId}/channels/${ch.id}`);
		} catch (e) {
			console.error('Failed to create channel:', e);
		} finally {
			creatingChannel = false;
		}
	}

	function handleNewChannelKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') handleCreateChannel();
		if (e.key === 'Escape') {
			showNewChannel = false;
			newChannelName = '';
		}
	}

	// Channel deletion
	let confirmDeleteId = $state<string | null>(null);
	let deletingChannel = $state(false);

	async function handleDeleteChannel(channelId: string) {
		if (deletingChannel) return;
		deletingChannel = true;
		try {
			await deleteChannel(serverId, channelId);
			channels = channels.filter(c => c.id !== channelId);
			confirmDeleteId = null;
			// Navigate away if the deleted channel was active.
			if (currentChannelId === channelId) {
				const first = channels[0];
				if (first) {
					goto(`/servers/${serverId}/channels/${first.id}`);
				} else {
					goto(`/servers/${serverId}`);
				}
			}
		} catch (e) {
			console.error('Failed to delete channel:', e);
		} finally {
			deletingChannel = false;
		}
	}

	// Channel rename
	let editingChannelId = $state<string | null>(null);
	let editChannelName = $state('');
	let savingChannelName = $state(false);

	function startRenameChannel(chId: string) {
		const ch = channels.find(c => c.id === chId);
		editingChannelId = chId;
		editChannelName = ch?.name ?? '';
	}

	async function handleRenameChannel() {
		if (!editingChannelId || savingChannelName) return;
		const trimmed = editChannelName.trim();
		if (!trimmed) { editingChannelId = null; return; }
		const ch = channels.find(c => c.id === editingChannelId);
		if (ch && trimmed === ch.name) { editingChannelId = null; return; }
		savingChannelName = true;
		try {
			const updated = await updateChannel(serverId, editingChannelId, trimmed);
			channels = channels.map(c => c.id === updated.id ? updated : c);
			editingChannelId = null;
		} catch (e) {
			console.error('Failed to rename channel:', e);
		} finally {
			savingChannelName = false;
		}
	}

	function handleRenameKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') handleRenameChannel();
		if (e.key === 'Escape') { editingChannelId = null; }
	}

	// Channel context menu
	let channelCtx = $state<{ x: number; y: number; channelId: string } | null>(null);

	function handleGlobalKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			if (confirmDeleteId) { confirmDeleteId = null; return; }
			if (editingChannelId) { editingChannelId = null; return; }
			if (showNewChannel) { showNewChannel = false; newChannelName = ''; return; }
		}
	}

	function handleChannelContextMenu(e: MouseEvent, channelId: string) {
		e.preventDefault();
		channelCtx = { x: e.clientX, y: e.clientY, channelId };
	}

	function channelContextItems(chId: string) {
		const items: { label: string; action: () => void; danger?: boolean }[] = [
			{ label: 'Copy Channel ID', action: () => navigator.clipboard.writeText(chId) },
		];
		if (isOwner) {
			items.push({ label: 'Edit Channel', action: () => startRenameChannel(chId) });
			items.push({ label: 'Delete Channel', action: () => { confirmDeleteId = chId; }, danger: true });
		}
		return items;
	}
</script>

<svelte:window onkeydown={handleGlobalKeydown} />

<aside class="channel-sidebar">
	<div class="server-header">
		<h2>{serverName}</h2>
		{#if isOwner}
			<button
				class="settings-btn"
				title="Server Settings"
				onclick={() => (showSettings = true)}
			>
				<svg width="16" height="16" viewBox="0 0 20 20" fill="currentColor">
					<path fill-rule="evenodd" d="M11.49 3.17c-.38-1.56-2.6-1.56-2.98 0a1.532 1.532 0 01-2.286.948c-1.372-.836-2.942.734-2.106 2.106.54.886.061 2.042-.947 2.287-1.561.379-1.561 2.6 0 2.978a1.532 1.532 0 01.947 2.287c-.836 1.372.734 2.942 2.106 2.106a1.532 1.532 0 012.287.947c.379 1.561 2.6 1.561 2.978 0a1.533 1.533 0 012.287-.947c1.372.836 2.942-.734 2.106-2.106a1.533 1.533 0 01.947-2.287c1.561-.379 1.561-2.6 0-2.978a1.532 1.532 0 01-.947-2.287c.836-1.372-.734-2.942-2.106-2.106a1.532 1.532 0 01-2.287-.947zM10 13a3 3 0 100-6 3 3 0 000 6z" clip-rule="evenodd"/>
				</svg>
			</button>
		{/if}
	</div>

	<div class="channels">
		<div class="category-header">
			<span class="category-label">TEXT CHANNELS</span>
			<button
				class="add-channel-btn"
				title="Create Channel"
				onclick={() => { showNewChannel = !showNewChannel; }}
			>+</button>
		</div>
		{#if showNewChannel}
			<div class="new-channel-input">
				<span class="hash">#</span>
				<input
					type="text"
					placeholder="new-channel"
					maxlength="100"
					bind:value={newChannelName}
					onkeydown={handleNewChannelKeydown}
					disabled={creatingChannel}
				/>
			</div>
		{/if}
		{#each channels as channel (channel.id)}
			{@const unread = unreadCounts[channel.id] ?? 0}
			<div class="channel-row">
				<button
					class="channel"
					class:active={currentChannelId === channel.id}
					class:unread={unread > 0 && currentChannelId !== channel.id}
					onclick={() => goto(`/servers/${serverId}/channels/${channel.id}`)}
					oncontextmenu={(e) => handleChannelContextMenu(e, channel.id)}
				>
					<span class="hash">#</span>
					<span class="channel-name">{channel.name ?? 'unnamed'}</span>
					{#if unread > 0 && currentChannelId !== channel.id}
						<span class="unread-badge">{unread > 99 ? '99+' : unread}</span>
					{/if}
				</button>
				<button
					class="delete-channel-btn"
					title="Delete Channel"
					onclick={(e) => { e.stopPropagation(); confirmDeleteId = channel.id; }}
				>&times;</button>
			</div>
		{/each}
	</div>

	{#if currentUser}
		<button class="user-panel" onclick={() => (showProfile = true)}>
			<div class="user-panel-avatar-wrapper">
				{#if currentUser.avatar_path}
					<img class="user-panel-avatar" src="/uploads/{currentUser.avatar_path}" alt="" />
				{:else}
					<span class="user-panel-avatar user-panel-avatar-fallback">{(currentUser.username ?? '?').charAt(0).toUpperCase()}</span>
				{/if}
				<span class="user-panel-presence"></span>
			</div>
			<span class="user-panel-name">{currentUser.display_name ?? currentUser.username}</span>
		</button>
	{/if}
</aside>

{#if editingChannelId}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="modal-overlay" onclick={() => { editingChannelId = null; }}>
		<div class="modal-dialog" onclick={(e) => e.stopPropagation()}>
			<h3>Edit Channel</h3>
			<input
				type="text"
				class="rename-input"
				maxlength="100"
				bind:value={editChannelName}
				onkeydown={handleRenameKeydown}
				disabled={savingChannelName}
			/>
			<div class="modal-actions">
				<button class="btn-cancel" onclick={() => { editingChannelId = null; }}>Cancel</button>
				<button
					class="btn-save"
					disabled={savingChannelName || !editChannelName.trim()}
					onclick={handleRenameChannel}
				>{savingChannelName ? 'Saving...' : 'Save'}</button>
			</div>
		</div>
	</div>
{/if}

{#if confirmDeleteId}
	{@const channelToDelete = channels.find(c => c.id === confirmDeleteId)}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="modal-overlay" onclick={() => { confirmDeleteId = null; }}>
		<div class="modal-dialog" onclick={(e) => e.stopPropagation()}>
			<h3>Delete Channel</h3>
			<p>Are you sure you want to delete <strong>#{channelToDelete?.name ?? 'this channel'}</strong>? All messages will be permanently removed.</p>
			<div class="modal-actions">
				<button class="btn-cancel" onclick={() => { confirmDeleteId = null; }}>Cancel</button>
				<button
					class="btn-danger"
					disabled={deletingChannel}
					onclick={() => handleDeleteChannel(confirmDeleteId!)}
				>{deletingChannel ? 'Deleting...' : 'Delete'}</button>
			</div>
		</div>
	</div>
{/if}

{#if showSettings && server}
	<ServerSettings
		{server}
		onclose={() => (showSettings = false)}
		ondelete={() => { showSettings = false; if (onserverdelete) onserverdelete(); }}
		onsave={(updated) => { server = updated; serverName = updated.name; }}
	/>
{/if}

{#if channelCtx}
	<ContextMenu
		x={channelCtx.x}
		y={channelCtx.y}
		items={channelContextItems(channelCtx.channelId)}
		onClose={() => (channelCtx = null)}
	/>
{/if}

{#if showProfile && currentUser}
	<UserProfile
		user={currentUser}
		onclose={() => (showProfile = false)}
		onsave={(updated) => { currentUser = updated; }}
	/>
{/if}

<style>
	.channel-sidebar {
		width: 240px;
		min-width: 240px;
		background: var(--bg-sidebar);
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.server-header {
		padding: 12px 16px;
		border-bottom: 1px solid var(--border);
		min-height: 48px;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
	}

	.server-header h2 {
		font-size: 15px;
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.settings-btn {
		flex-shrink: 0;
		color: var(--text-muted);
		padding: 4px;
		border-radius: 4px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.settings-btn:hover {
		color: var(--text-primary);
		background: var(--bg-hover);
	}

	.channels {
		flex: 1;
		overflow-y: auto;
		padding: 8px 0;
	}

	.category-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 16px 16px 4px;
	}

	.category-label {
		font-size: 11px;
		font-weight: 700;
		color: var(--text-muted);
		letter-spacing: 0.02em;
		text-transform: uppercase;
	}

	.add-channel-btn {
		font-size: 16px;
		line-height: 1;
		color: var(--text-muted);
		cursor: pointer;
		padding: 0 2px;
		border-radius: 3px;
	}

	.add-channel-btn:hover {
		color: var(--text-primary);
	}

	.new-channel-input {
		display: flex;
		align-items: center;
		gap: 6px;
		margin: 2px 8px;
		padding: 4px 8px;
		background: var(--bg-primary);
		border-radius: 4px;
	}

	.new-channel-input input {
		flex: 1;
		background: none;
		border: none;
		outline: none;
		color: var(--text-primary);
		font-size: 14px;
		font-family: inherit;
		padding: 2px 0;
	}

	.new-channel-input input::placeholder {
		color: var(--text-muted);
	}

	.channel-row {
		display: flex;
		align-items: center;
		margin: 1px 8px;
		border-radius: 4px;
		position: relative;
	}

	.channel-row:hover .delete-channel-btn {
		opacity: 1;
	}

	.channel {
		display: flex;
		align-items: center;
		gap: 6px;
		width: 100%;
		padding: 6px 12px;
		border-radius: 4px;
		color: var(--text-muted);
	}

	.channel.unread {
		color: var(--text-primary);
		font-weight: 600;
	}

	.unread-badge {
		margin-left: auto;
		background: var(--accent);
		color: #1c1917;
		font-size: 11px;
		font-weight: 700;
		padding: 1px 5px;
		border-radius: 8px;
		min-width: 18px;
		text-align: center;
		line-height: 16px;
	}

	.channel:hover {
		background: var(--bg-hover);
		color: var(--text-primary);
	}

	.channel.active {
		background: var(--bg-hover);
		color: white;
	}

	.delete-channel-btn {
		position: absolute;
		right: 4px;
		opacity: 0;
		font-size: 14px;
		color: var(--text-muted);
		padding: 2px 6px;
		border-radius: 3px;
		cursor: pointer;
		transition: opacity 0.1s;
	}

	.delete-channel-btn:hover {
		color: #ef4444;
		background: var(--bg-primary);
	}

	.hash {
		font-size: 18px;
		font-weight: 400;
		opacity: 0.7;
	}

	.channel-name {
		font-size: 14px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.user-panel {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 12px;
		background: var(--bg-server-bar);
		border-top: 1px solid var(--border);
		cursor: pointer;
		width: 100%;
		text-align: left;
	}

	.user-panel:hover {
		background: var(--bg-hover);
	}

	.user-panel-avatar-wrapper {
		position: relative;
		flex-shrink: 0;
		width: 32px;
		height: 32px;
	}

	.user-panel-avatar {
		width: 32px;
		height: 32px;
		border-radius: 50%;
		object-fit: cover;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.user-panel-avatar-fallback {
		background: var(--bg-tertiary);
		color: var(--text-muted);
		font-size: 14px;
		font-weight: 600;
	}

	.user-panel-presence {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		background-color: var(--accent);
		position: absolute;
		bottom: -2px;
		right: -2px;
		border: 2px solid var(--bg-server-bar);
	}

	.user-panel-name {
		font-size: 13px;
		font-weight: 600;
		color: var(--text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.modal-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.6);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 100;
	}

	.modal-dialog {
		background: var(--bg-sidebar);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 20px 24px;
		max-width: 400px;
		width: 90%;
	}

	.modal-dialog h3 {
		margin: 0 0 8px;
		font-size: 16px;
	}

	.modal-dialog p {
		color: var(--text-muted);
		font-size: 14px;
		margin: 0 0 16px;
		line-height: 1.4;
	}

	.modal-actions {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
	}

	.btn-cancel, .btn-danger {
		padding: 8px 16px;
		border-radius: 4px;
		font-size: 14px;
		cursor: pointer;
	}

	.btn-cancel {
		background: var(--bg-primary);
		color: var(--text-primary);
	}

	.btn-cancel:hover {
		background: var(--bg-hover);
	}

	.btn-danger {
		background: #dc2626;
		color: white;
	}

	.btn-danger:hover {
		background: #b91c1c;
	}

	.btn-danger:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.btn-save {
		padding: 8px 16px;
		border-radius: 4px;
		font-size: 14px;
		cursor: pointer;
		background: var(--accent);
		color: white;
	}

	.btn-save:hover:not(:disabled) {
		background: var(--accent-hover);
	}

	.btn-save:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.rename-input {
		width: 100%;
		padding: 8px 10px;
		background: var(--bg-primary);
		border: 1px solid var(--border);
		border-radius: 4px;
		color: var(--text-primary);
		font-size: 14px;
		font-family: inherit;
		margin-bottom: 16px;
	}

	@media (max-width: 768px) {
		.channel-sidebar {
			position: fixed;
			top: 0;
			left: 0;
			bottom: 0;
			z-index: 45;
			transform: translateX(-100%);
			transition: transform 0.2s ease;
		}
	}
</style>
