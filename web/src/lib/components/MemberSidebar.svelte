<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import type { ServerMember } from '$lib/types';
	import { isUserOnline, subscribePresence } from '$lib/ws';
	import { openDM, blockUser, avatarSrc } from '$lib/api';
	import { applyBlockState } from '$lib/blocks';

	let { members = [], visible = true, currentUserId = '' }: {
		members?: ServerMember[];
		visible?: boolean;
		currentUserId?: string;
	} = $props();

	let failedAvatars = $state(new Set<string>());
	let presenceTick = $state(0);
	onMount(() => subscribePresence(() => { presenceTick++; }));

	function checkOnline(userId: string): boolean {
		void presenceTick;
		return isUserOnline(userId);
	}

	const onlineMembers = $derived(members.filter(m => checkOnline(m.user_id)));
	const offlineMembers = $derived(members.filter(m => !checkOnline(m.user_id)));

	let contextMenu = $state<{ x: number; y: number; member: ServerMember } | null>(null);
	let confirmBlock = $state<ServerMember | null>(null);

	function handleMemberClick(e: MouseEvent, member: ServerMember) {
		if (member.user_id === currentUserId) return;
		e.preventDefault();
		contextMenu = { x: e.clientX, y: e.clientY, member };
	}

	async function handleMessage(member: ServerMember) {
		contextMenu = null;
		try {
			const dm = await openDM(member.user_id);
			goto(`/dm/${dm.channel.id}`);
		} catch (e) {
			console.error('Failed to open DM:', e);
		}
	}

	async function handleBlock() {
		if (!confirmBlock) return;
		try {
			const state = await blockUser(confirmBlock.user_id);
			applyBlockState(confirmBlock.user_id, state.blocked_either, state.blocked_by_me);
		} catch (e) {
			console.error('Failed to block user:', e);
		}
		confirmBlock = null;
	}

	function handleWindowClick() {
		contextMenu = null;
	}
</script>

<svelte:window onclick={handleWindowClick} />

{#if visible}
	<aside class="member-sidebar">
		<div class="member-header">
			<span class="member-header-label">Members — {members.length}</span>
		</div>
		<div class="member-list">
			{#if onlineMembers.length > 0}
				<div class="member-group-label">ONLINE — {onlineMembers.length}</div>
				{#each onlineMembers as member (member.user_id)}
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div class="member" class:clickable={member.user_id !== currentUserId} onclick={(e) => handleMemberClick(e, member)}>
						<div class="member-avatar-wrapper">
							{#if member.avatar_url && !failedAvatars.has(member.avatar_url)}
								<img class="member-avatar" src={avatarSrc(member.avatar_url!)} alt="" onerror={() => { failedAvatars.add(member.avatar_url!); failedAvatars = failedAvatars; }} />
							{:else}
								<span class="member-avatar member-avatar-fallback">{(member.nickname ?? member.username ?? '?').charAt(0).toUpperCase()}</span>
							{/if}
							<span class="presence-dot online"></span>
						</div>
						<span class="member-name">{member.nickname ?? member.display_name ?? member.username ?? member.user_id.slice(0, 8)}</span>
					</div>
				{/each}
			{/if}
			{#if offlineMembers.length > 0}
				<div class="member-group-label">OFFLINE — {offlineMembers.length}</div>
				{#each offlineMembers as member (member.user_id)}
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div class="member offline" class:clickable={member.user_id !== currentUserId} onclick={(e) => handleMemberClick(e, member)}>
						<div class="member-avatar-wrapper">
							{#if member.avatar_url && !failedAvatars.has(member.avatar_url)}
								<img class="member-avatar" src={avatarSrc(member.avatar_url!)} alt="" onerror={() => { failedAvatars.add(member.avatar_url!); failedAvatars = failedAvatars; }} />
							{:else}
								<span class="member-avatar member-avatar-fallback">{(member.nickname ?? member.username ?? '?').charAt(0).toUpperCase()}</span>
							{/if}
							<span class="presence-dot"></span>
						</div>
						<span class="member-name">{member.nickname ?? member.display_name ?? member.username ?? member.user_id.slice(0, 8)}</span>
					</div>
				{/each}
			{/if}
		</div>
	</aside>
{/if}

{#if contextMenu}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="member-context" style="left: {contextMenu.x}px; top: {contextMenu.y}px" onclick={(e) => e.stopPropagation()}>
		<button class="ctx-item" onclick={() => { if (contextMenu) handleMessage(contextMenu.member); }}>Message</button>
		<button class="ctx-item ctx-danger" onclick={() => { confirmBlock = contextMenu?.member ?? null; contextMenu = null; }}>Block</button>
	</div>
{/if}

{#if confirmBlock}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="block-overlay" onclick={() => (confirmBlock = null)}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="block-modal" onclick={(e) => e.stopPropagation()}>
			<h3>Block {confirmBlock.display_name ?? confirmBlock.username}?</h3>
			<p class="block-warn">They won't be able to message you or see your online status.</p>
			<div class="block-actions">
				<button class="block-cancel" onclick={() => (confirmBlock = null)}>Cancel</button>
				<button class="block-confirm" onclick={handleBlock}>Block</button>
			</div>
		</div>
	</div>
{/if}

<style>
	.member-sidebar {
		width: 240px;
		min-width: 240px;
		background: var(--bg-secondary);
		display: flex;
		flex-direction: column;
		overflow: hidden;
		border-left: 1px solid var(--border);
	}

	.member-header {
		padding: 12px 16px;
		border-bottom: 1px solid var(--border);
		min-height: 48px;
		display: flex;
		align-items: center;
	}

	.member-header-label {
		font-size: 13px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.member-list {
		flex: 1;
		overflow-y: auto;
		padding: 8px 0;
	}

	.member-group-label {
		font-size: 11px;
		font-weight: 700;
		color: var(--text-muted);
		letter-spacing: 0.02em;
		text-transform: uppercase;
		padding: 16px 16px 4px;
	}

	.member {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 6px 16px;
		font-size: 13px;
		color: var(--text-primary);
		border-radius: 4px;
		margin: 1px 8px;
	}

	.member:hover {
		background: var(--bg-hover);
	}

	.member.offline {
		opacity: 0.5;
	}

	.member-avatar-wrapper {
		position: relative;
		flex-shrink: 0;
		width: 32px;
		height: 32px;
	}

	.member-avatar {
		width: 32px;
		height: 32px;
		border-radius: 50%;
		object-fit: cover;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.member-avatar-fallback {
		background: var(--bg-tertiary);
		color: var(--text-muted);
		font-size: 13px;
		font-weight: 600;
	}

	.presence-dot {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		flex-shrink: 0;
		background-color: #57534e;
		position: absolute;
		bottom: -2px;
		right: -2px;
		border: 2px solid var(--bg-secondary);
	}

	.presence-dot.online {
		background-color: var(--accent);
	}

	.member-name {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.member.clickable {
		cursor: pointer;
	}

	.member-context {
		position: fixed;
		background: var(--bg-primary);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 4px;
		z-index: 200;
		min-width: 140px;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
	}

	.ctx-item {
		display: block;
		width: 100%;
		text-align: left;
		padding: 8px 12px;
		font-size: 13px;
		border-radius: 4px;
		color: var(--text-primary);
	}

	.ctx-item:hover {
		background: var(--bg-hover);
	}

	.ctx-danger {
		color: #ef4444;
	}

	.ctx-danger:hover {
		background: rgba(239, 68, 68, 0.15);
	}

	.block-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.7);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 300;
	}

	.block-modal {
		background: var(--bg-primary);
		border-radius: 8px;
		padding: 24px;
		width: 360px;
		max-width: 90vw;
	}

	.block-modal h3 {
		font-size: 18px;
		margin-bottom: 8px;
	}

	.block-warn {
		color: var(--text-muted);
		font-size: 14px;
		margin-bottom: 20px;
	}

	.block-actions {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
	}

	.block-cancel {
		padding: 8px 16px;
		color: var(--text-muted);
	}

	.block-cancel:hover {
		color: var(--text-primary);
	}

	.block-confirm {
		padding: 8px 16px;
		background: #ef4444;
		color: white;
		border-radius: 4px;
		font-weight: 500;
	}

	.block-confirm:hover {
		background: #dc2626;
	}

	@media (max-width: 1024px) {
		.member-sidebar {
			display: none;
		}
	}
</style>
