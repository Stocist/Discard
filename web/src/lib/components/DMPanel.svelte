<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import type { DMChannelView, User } from '$lib/types';
	import { listDMs, closeDM, listBlocks, unblockUser, avatarSrc } from '$lib/api';
	import { subscribeDMEvents } from '$lib/ws';

	let { onclose }: { onclose: () => void } = $props();

	let failedAvatars = $state(new Set<string>());
	let dms = $state<DMChannelView[]>([]);
	let blocks = $state<User[]>([]);
	let loading = $state(true);

	function sortDMs(list: DMChannelView[]): DMChannelView[] {
		return [...list].sort((a, b) => {
			const ta = a.last_message_at ?? a.channel.created_at;
			const tb = b.last_message_at ?? b.channel.created_at;
			return tb.localeCompare(ta);
		});
	}

	function relativeTime(dateStr: string | null): string {
		if (!dateStr) return '';
		const diff = Date.now() - new Date(dateStr).getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 1) return 'just now';
		if (mins < 60) return `${mins}m ago`;
		const hrs = Math.floor(mins / 60);
		if (hrs < 24) return `${hrs}h ago`;
		const days = Math.floor(hrs / 24);
		return `${days}d ago`;
	}

	async function load() {
		loading = true;
		try {
			const [dmList, blockList] = await Promise.all([listDMs(), listBlocks()]);
			dms = sortDMs(dmList);
			blocks = blockList;
		} catch (e) {
			console.error('Failed to load DMs/blocks:', e);
		} finally {
			loading = false;
		}
	}

	async function handleClose(channelId: string) {
		try {
			await closeDM(channelId);
			dms = dms.filter(d => d.channel.id !== channelId);
		} catch (e) {
			console.error('Failed to close DM:', e);
		}
	}

	async function handleUnblock(userId: string) {
		try {
			await unblockUser(userId);
			blocks = blocks.filter(b => b.id !== userId);
		} catch (e) {
			console.error('Failed to unblock user:', e);
		}
	}

	function openDMChannel(channelId: string) {
		goto(`/dm/${channelId}`);
		onclose();
	}

	onMount(() => {
		load();
		return subscribeDMEvents((event) => {
			if (event.type === 'dm_opened') {
				const newDM: DMChannelView = {
					channel: event.channel,
					other_user: event.opener,
					last_message_at: null
				};
				if (!dms.some(d => d.channel.id === event.channel.id)) {
					dms = sortDMs([newDM, ...dms]);
				}
			}
		});
	});
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="dm-overlay" onclick={onclose}>
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="dm-panel" onclick={(e) => e.stopPropagation()}>
		<div class="dm-header">
			<h2>Direct Messages</h2>
			<button class="close-btn" onclick={onclose} title="Close">
				<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<line x1="18" y1="6" x2="6" y2="18" />
					<line x1="6" y1="6" x2="18" y2="18" />
				</svg>
			</button>
		</div>

		<div class="dm-content">
			{#if loading}
				<p class="dm-empty">Loading...</p>
			{:else if dms.length === 0}
				<p class="dm-empty">No conversations yet. Message someone from a server's member list.</p>
			{:else}
				<div class="dm-list">
					{#each dms as dm (dm.channel.id)}
						<div class="dm-row">
							<button class="dm-row-main" onclick={() => openDMChannel(dm.channel.id)}>
								<div class="dm-avatar-wrapper">
									{#if dm.other_user.avatar_path && !failedAvatars.has(dm.other_user.avatar_path)}
										<img class="dm-avatar" src={avatarSrc(dm.other_user.avatar_path!)} alt="" onerror={() => { failedAvatars.add(dm.other_user.avatar_path!); failedAvatars = failedAvatars; }} />
									{:else}
										<span class="dm-avatar dm-avatar-fallback">
											{(dm.other_user.display_name ?? dm.other_user.username ?? '?').charAt(0).toUpperCase()}
										</span>
									{/if}
								</div>
								<div class="dm-info">
									<span class="dm-name">{dm.other_user.display_name ?? dm.other_user.username}</span>
									{#if dm.last_message_at}
										<span class="dm-time">{relativeTime(dm.last_message_at)}</span>
									{/if}
								</div>
							</button>
							<button class="dm-close-btn" onclick={() => handleClose(dm.channel.id)} title="Close conversation">
								<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
									<line x1="18" y1="6" x2="6" y2="18" />
									<line x1="6" y1="6" x2="18" y2="18" />
								</svg>
							</button>
						</div>
					{/each}
				</div>
			{/if}

			{#if blocks.length > 0}
				<div class="blocked-section">
					<div class="blocked-header">BLOCKED USERS</div>
					{#each blocks as user (user.id)}
						<div class="blocked-row">
							<div class="dm-avatar-wrapper">
								{#if user.avatar_path && !failedAvatars.has(user.avatar_path)}
									<img class="dm-avatar" src={avatarSrc(user.avatar_path!)} alt="" onerror={() => { failedAvatars.add(user.avatar_path!); failedAvatars = failedAvatars; }} />
								{:else}
									<span class="dm-avatar dm-avatar-fallback">
										{(user.display_name ?? user.username ?? '?').charAt(0).toUpperCase()}
									</span>
								{/if}
							</div>
							<span class="blocked-name">{user.display_name ?? user.username}</span>
							<button class="unblock-btn" onclick={() => handleUnblock(user.id)}>Unblock</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.dm-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.7);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 100;
	}

	.dm-panel {
		background: var(--bg-primary);
		border-radius: 8px;
		width: 480px;
		max-width: 90vw;
		max-height: 80vh;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.dm-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 16px 20px;
		border-bottom: 1px solid var(--border);
	}

	.dm-header h2 {
		font-size: 18px;
		font-weight: 600;
	}

	.close-btn {
		color: var(--text-muted);
		padding: 4px;
		border-radius: 4px;
	}

	.close-btn:hover {
		color: var(--text-primary);
		background: var(--bg-hover);
	}

	.dm-content {
		flex: 1;
		overflow-y: auto;
		padding: 8px 0;
	}

	.dm-empty {
		color: var(--text-muted);
		font-size: 14px;
		text-align: center;
		padding: 32px 20px;
	}

	.dm-list {
		display: flex;
		flex-direction: column;
	}

	.dm-row {
		display: flex;
		align-items: center;
		padding: 0 8px;
	}

	.dm-row:hover {
		background: var(--bg-hover);
	}

	.dm-row-main {
		flex: 1;
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 10px 12px;
		text-align: left;
		min-width: 0;
	}

	.dm-avatar-wrapper {
		flex-shrink: 0;
		width: 36px;
		height: 36px;
	}

	.dm-avatar {
		width: 36px;
		height: 36px;
		border-radius: 50%;
		object-fit: cover;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.dm-avatar-fallback {
		background: var(--bg-tertiary);
		color: var(--text-muted);
		font-size: 14px;
		font-weight: 600;
	}

	.dm-info {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: baseline;
		gap: 8px;
	}

	.dm-name {
		font-size: 14px;
		font-weight: 500;
		color: var(--text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.dm-time {
		font-size: 12px;
		color: var(--text-muted);
		flex-shrink: 0;
	}

	.dm-close-btn {
		color: var(--text-muted);
		padding: 6px;
		border-radius: 4px;
		opacity: 0;
		flex-shrink: 0;
	}

	.dm-row:hover .dm-close-btn {
		opacity: 1;
	}

	.dm-close-btn:hover {
		color: var(--text-primary);
		background: var(--bg-tertiary);
	}

	.blocked-section {
		border-top: 1px solid var(--border);
		margin-top: 8px;
		padding-top: 8px;
	}

	.blocked-header {
		font-size: 11px;
		font-weight: 700;
		color: var(--text-muted);
		letter-spacing: 0.02em;
		text-transform: uppercase;
		padding: 8px 20px 4px;
	}

	.blocked-row {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 8px 20px;
	}

	.blocked-row:hover {
		background: var(--bg-hover);
	}

	.blocked-name {
		flex: 1;
		font-size: 14px;
		color: var(--text-primary);
	}

	.unblock-btn {
		font-size: 12px;
		color: var(--text-muted);
		padding: 4px 10px;
		border: 1px solid var(--border);
		border-radius: 4px;
	}

	.unblock-btn:hover {
		color: var(--text-primary);
		border-color: var(--text-muted);
	}
</style>
