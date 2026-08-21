<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { getDM } from '$lib/api';
	import { createWSConnection } from '$lib/ws';
	import { captureBlockStateRevisions, isBlockedEither, primeConversationBlockState, subscribeBlockState as subscribeBlocks } from '$lib/blocks';
	import type { DMChannelView } from '$lib/types';
	import ChatView from '$lib/components/ChatView.svelte';

	const channelId = $derived(page.params.channelId ?? '');
	let dm = $state<DMChannelView | null>(null);
	let sharedWs = $state<WebSocket | undefined>(undefined);
	let blockStateTick = $state(0);

	$effect(() => {
		const conn = createWSConnection();
		sharedWs = conn;
		return () => {
			sharedWs = undefined;
			conn.close();
		};
	});

	$effect(() => subscribeBlocks(() => { blockStateTick++; }));

	$effect(() => {
		const id = channelId;
		if (!id) return;
		let cancelled = false;
		dm = null;
		const requestRevisions = captureBlockStateRevisions();
		getDM(id).then((view) => {
			if (cancelled) return;
			dm = view;
			primeConversationBlockState(view.other_user.id, view.can_message, requestRevisions);
		}).catch((error) => {
			console.error('Failed to load DM:', error);
			if (!cancelled) goto('/');
		});
		return () => { cancelled = true; };
	});

	const channelName = $derived(dm?.other_user.display_name ?? dm?.other_user.username ?? 'Direct message');
	const canMessage = $derived.by(() => {
		void blockStateTick;
		return dm ? !isBlockedEither(dm.other_user.id) : false;
	});
</script>

{#if dm}
	<ChatView {channelId} {channelName} ws={sharedWs} isDM canMessage={canMessage} />
{:else}
	<div class="loading">Loading conversation...</div>
{/if}

<style>
	.loading {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-muted);
	}
</style>
