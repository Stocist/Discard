<script lang="ts">
	import type { Channel } from '$lib/types';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	let { serverId, channels, serverName, memberCount = 0 }: {
		serverId: string;
		channels: Channel[];
		serverName: string;
		memberCount?: number;
	} = $props();

	const currentChannelId = $derived(page.params?.channelId ?? '');
</script>

<aside class="channel-sidebar">
	<div class="server-header">
		<h2>{serverName}</h2>
	</div>

	<div class="channels">
		<div class="category-label">TEXT CHANNELS</div>
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

	<div class="member-info">
		<span class="member-count">{memberCount} member{memberCount !== 1 ? 's' : ''}</span>
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

	.category-label {
		padding: 16px 16px 4px;
		font-size: 11px;
		font-weight: 700;
		color: var(--text-muted);
		letter-spacing: 0.02em;
		text-transform: uppercase;
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

	.member-info {
		padding: 10px 16px;
		border-top: 1px solid var(--border);
		font-size: 12px;
		color: var(--text-muted);
	}
</style>
