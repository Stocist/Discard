<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { getServer, listChannels, listMembers, fetchMe, getUnreadCounts, markChannelRead } from '$lib/api';
	import { subscribeUnread } from '$lib/ws';
	import type { Channel, Server, ServerMember, User } from '$lib/types';
	import ChannelSidebar from '$lib/components/ChannelSidebar.svelte';
	import ChatView from '$lib/components/ChatView.svelte';
	import MemberSidebar from '$lib/components/MemberSidebar.svelte';

	const serverId = $derived(page.params.serverId ?? '');
	const channelId = $derived(page.params.channelId ?? '');

	let channels = $state<Channel[]>([]);
	let members = $state<ServerMember[]>([]);
	let serverName = $state('');
	let server = $state<Server | undefined>(undefined);
	let currentUser = $state<User | null>(null);
	let unreadCounts = $state<Record<string, number>>({});
	let showMembers = $state(true);

	const currentUserId = $derived(currentUser?.id ?? '');
	const isOwner = $derived(!!server && currentUserId === server.owner_id);
	const currentChannel = $derived(channels.find(c => c.id === channelId));
	const channelName = $derived(currentChannel?.name ?? 'unknown');

	$effect(() => {
		fetchMe().then(u => { currentUser = u; }).catch(() => {});
	});

	$effect(() => {
		const sid = serverId;
		if (!sid) return;

		(async () => {
			try {
				const [srv, chans, mems, counts] = await Promise.all([
					getServer(sid),
					listChannels(sid),
					listMembers(sid),
					getUnreadCounts(sid)
				]);
				server = srv;
				serverName = srv.name;
				channels = chans;
				members = mems;
				unreadCounts = counts;
			} catch (e) {
				console.error('Failed to load server data:', e);
			}
		})();
	});

	// Mark channel as read when switching channels
	$effect(() => {
		const cid = channelId;
		if (!cid) return;
		markChannelRead(cid).catch(() => {});
		// Clear this channel's unread count locally for instant feedback
		if (unreadCounts[cid]) {
			const updated = { ...unreadCounts };
			delete updated[cid];
			unreadCounts = updated;
		}
	});

	// Increment unread counts when WS messages arrive for other channels
	$effect(() => {
		const cid = channelId;
		return subscribeUnread((msgChannelId: string) => {
			if (msgChannelId === cid) return;
			unreadCounts = {
				...unreadCounts,
				[msgChannelId]: (unreadCounts[msgChannelId] ?? 0) + 1
			};
		});
	});

	function handleServerDelete() {
		goto('/');
	}

	function toggleMembers() {
		showMembers = !showMembers;
	}
</script>

<ChannelSidebar
	{serverId}
	bind:channels
	bind:serverName
	bind:server
	{isOwner}
	onserverdelete={handleServerDelete}
	{unreadCounts}
	bind:currentUser
/>

{#if channelId}
	<ChatView {channelId} {channelName} onToggleMembers={toggleMembers} />
{/if}

<MemberSidebar {members} visible={showMembers} />
