<script lang="ts">
	import { page } from '$app/state';
	import { getServer, listChannels, listMembers } from '$lib/api';
	import type { Channel, ServerMember } from '$lib/types';
	import ChannelSidebar from '$lib/components/ChannelSidebar.svelte';
	import ChatView from '$lib/components/ChatView.svelte';

	const serverId = $derived(page.params.serverId ?? '');
	const channelId = $derived(page.params.channelId ?? '');

	let channels = $state<Channel[]>([]);
	let members = $state<ServerMember[]>([]);
	let serverName = $state('');

	const currentChannel = $derived(channels.find(c => c.id === channelId));
	const channelName = $derived(currentChannel?.name ?? 'unknown');

	$effect(() => {
		const sid = serverId;
		if (!sid) return;

		(async () => {
			try {
				const [srv, chans, mems] = await Promise.all([
					getServer(sid),
					listChannels(sid),
					listMembers(sid)
				]);
				serverName = srv.name;
				channels = chans;
				members = mems;
			} catch (e) {
				console.error('Failed to load server data:', e);
			}
		})();
	});
</script>

<ChannelSidebar
	{serverId}
	{channels}
	{serverName}
	memberCount={members.length}
/>

{#if channelId}
	<ChatView {channelId} {channelName} />
{/if}
