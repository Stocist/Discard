<script lang="ts">
	import { onMount } from 'svelte';
	import type { ServerMember } from '$lib/types';
	import { isUserOnline, subscribePresence } from '$lib/ws';

	let { members = [], visible = true }: {
		members?: ServerMember[];
		visible?: boolean;
	} = $props();

	let presenceTick = $state(0);
	onMount(() => subscribePresence(() => { presenceTick++; }));

	function checkOnline(userId: string): boolean {
		void presenceTick;
		return isUserOnline(userId);
	}

	const onlineMembers = $derived(members.filter(m => checkOnline(m.user_id)));
	const offlineMembers = $derived(members.filter(m => !checkOnline(m.user_id)));
</script>

{#if visible}
	<aside class="member-sidebar">
		<div class="member-header">
			<span class="member-header-label">Members — {members.length}</span>
		</div>
		<div class="member-list">
			{#if onlineMembers.length > 0}
				<div class="member-group-label">ONLINE — {onlineMembers.length}</div>
				{#each onlineMembers as member (member.user_id)}
					<div class="member">
						<div class="member-avatar-wrapper">
							{#if member.avatar_url}
								<img class="member-avatar" src="/uploads/{member.avatar_url}" alt="" />
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
					<div class="member offline">
						<div class="member-avatar-wrapper">
							{#if member.avatar_url}
								<img class="member-avatar" src="/uploads/{member.avatar_url}" alt="" />
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

	@media (max-width: 1024px) {
		.member-sidebar {
			display: none;
		}
	}
</style>
