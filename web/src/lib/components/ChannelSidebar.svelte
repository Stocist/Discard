<script lang="ts">
	import { onMount } from 'svelte';
	import type { Channel, ServerMember } from '$lib/types';
	import { createChannel } from '$lib/api';
	import { isUserOnline, subscribePresence } from '$lib/ws';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	let { serverId, channels = $bindable(), serverName, members = [] }: {
		serverId: string;
		channels: Channel[];
		serverName: string;
		members?: ServerMember[];
	} = $props();

	const currentChannelId = $derived(page.params?.channelId ?? '');

	// Force re-render when presence changes
	let presenceTick = $state(0);
	onMount(() => subscribePresence(() => { presenceTick++; }));

	function checkOnline(userId: string): boolean {
		void presenceTick;
		return isUserOnline(userId);
	}

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
</script>

<aside class="channel-sidebar">
	<div class="server-header">
		<h2>{serverName}</h2>
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
			<button
				class="channel"
				class:active={currentChannelId === channel.id}
				onclick={() => goto(`/servers/${serverId}/channels/${channel.id}`)}
			>
				<span class="hash">#</span>
				<span class="channel-name">{channel.name ?? 'unnamed'}</span>
			</button>
		{/each}
	</div>

	<div class="member-list">
		<div class="category-label">MEMBERS — {members.length}</div>
		{#each members as member (member.user_id)}
			<div class="member">
				<span
					class="presence-dot"
					class:online={checkOnline(member.user_id)}
				></span>
				<span class="member-name">{member.nickname ?? member.username ?? member.user_id.slice(0, 8)}</span>
			</div>
		{/each}
	</div>
</aside>

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
	}

	.server-header h2 {
		font-size: 15px;
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
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

	.channel {
		display: flex;
		align-items: center;
		gap: 6px;
		width: 100%;
		padding: 6px 12px;
		border-radius: 4px;
		margin: 1px 8px;
		width: calc(100% - 16px);
		color: var(--text-muted);
	}

	.channel:hover {
		background: var(--bg-hover);
		color: var(--text-primary);
	}

	.channel.active {
		background: var(--bg-hover);
		color: white;
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

	.member-list {
		border-top: 1px solid var(--border);
		overflow-y: auto;
		max-height: 200px;
		padding-bottom: 8px;
	}

	.member {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 4px 16px;
		font-size: 13px;
		color: var(--text-muted);
	}

	.presence-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
		background-color: #57534e;
	}

	.presence-dot.online {
		background-color: var(--accent);
	}

	.member-name {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
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
