<script lang="ts">
	import type { Message } from '$lib/types';
	import { listMessages } from '$lib/api';
	import { createWSConnection, subscribe, unsubscribe, sendMessage } from '$lib/ws';
	import MessageInput from './MessageInput.svelte';

	let { channelId, channelName }: {
		channelId: string;
		channelName: string;
	} = $props();

	let messages = $state<Message[]>([]);
	let messagesEl: HTMLDivElement | undefined = $state();
	let loadingMore = $state(false);
	let hasMore = $state(true);
	let isAtBottom = $state(true);
	let activeConn: WebSocket | undefined = $state();

	function formatTime(dateStr: string): string {
		const d = new Date(dateStr);
		const now = new Date();
		const isToday = d.toDateString() === now.toDateString();
		const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
		if (isToday) return `Today at ${time}`;
		return `${d.toLocaleDateString([], { month: '2-digit', day: '2-digit', year: 'numeric' })} ${time}`;
	}

	function shouldGroup(current: Message, prev: Message | undefined): boolean {
		if (!prev) return false;
		if (prev.author_id !== current.author_id) return false;
		const diff = new Date(current.created_at).getTime() - new Date(prev.created_at).getTime();
		return diff < 5 * 60 * 1000;
	}

	function scrollToBottom() {
		if (messagesEl) {
			messagesEl.scrollTop = messagesEl.scrollHeight;
		}
	}

	function handleScroll() {
		if (!messagesEl) return;
		const { scrollTop, scrollHeight, clientHeight } = messagesEl;
		isAtBottom = scrollHeight - scrollTop - clientHeight < 40;
	}

	// Main effect: loads messages + manages WebSocket for the current channelId
	$effect(() => {
		const cid = channelId;
		let cancelled = false;

		messages = [];
		hasMore = true;
		isAtBottom = true;

		// Load history (API returns newest-first, reverse for display)
		(async () => {
			try {
				const msgs = await listMessages(cid, undefined, 50);
				if (cancelled) return;
				messages = msgs.reverse();
				hasMore = msgs.length >= 50;
				requestAnimationFrame(scrollToBottom);
			} catch (e) {
				if (!cancelled) console.error('Failed to load messages:', e);
			}
		})();

		// WebSocket
		const conn = createWSConnection();
		activeConn = conn;

		conn.addEventListener('open', () => {
			if (cancelled) { conn.close(); return; }
			subscribe(conn, cid);
		});

		conn.addEventListener('message', (event) => {
			if (cancelled) return;
			try {
				const data = JSON.parse(event.data);
				if (data.type === 'message' && data.message) {
					messages = [...messages, data.message as Message];
					if (isAtBottom) {
						requestAnimationFrame(scrollToBottom);
					}
				}
			} catch {
				// ignore
			}
		});

		return () => {
			cancelled = true;
			activeConn = undefined;
			if (conn.readyState === WebSocket.OPEN) {
				unsubscribe(conn, cid);
			}
			conn.close();
		};
	});

	async function handleLoadMore() {
		if (loadingMore || !hasMore || messages.length === 0) return;
		loadingMore = true;
		const prevHeight = messagesEl?.scrollHeight ?? 0;
		try {
			const oldest = messages[0];
			const older = await listMessages(channelId, oldest.id, 50);
			if (older.length < 50) hasMore = false;
			messages = [...older.reverse(), ...messages];
			requestAnimationFrame(() => {
				if (messagesEl) {
					messagesEl.scrollTop = messagesEl.scrollHeight - prevHeight;
				}
			});
		} catch (e) {
			console.error('Failed to load older messages:', e);
		} finally {
			loadingMore = false;
		}
	}

	function handleSend(content: string) {
		if (activeConn && activeConn.readyState === WebSocket.OPEN) {
			sendMessage(activeConn, channelId, content);
		}
	}
</script>

<div class="chat-view">
	<div class="chat-header">
		<span class="hash">#</span>
		<span class="channel-name">{channelName}</span>
	</div>

	<div class="messages" bind:this={messagesEl} onscroll={handleScroll}>
		{#if hasMore}
			<div class="load-more">
				<button onclick={handleLoadMore} disabled={loadingMore}>
					{loadingMore ? 'Loading...' : 'Load more messages'}
				</button>
			</div>
		{/if}

		{#each messages as message, i (message.id)}
			{@const grouped = shouldGroup(message, messages[i - 1])}
			<div class="message" class:grouped>
				{#if !grouped}
					<div class="message-header">
						<span class="avatar">{(message.author_username ?? message.author_id).charAt(0).toUpperCase()}</span>
						<span class="author">{message.author_username ?? message.author_id}</span>
						<span class="timestamp">{formatTime(message.created_at)}</span>
					</div>
				{/if}
				<div class="message-content" class:has-header={!grouped}>
					{message.content}
				</div>
			</div>
		{/each}

		{#if messages.length === 0}
			<div class="empty-state">
				<p>No messages yet. Start the conversation!</p>
			</div>
		{/if}
	</div>

	<MessageInput {channelName} onSend={handleSend} />
</div>

<style>
	.chat-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-width: 0;
		background: var(--bg-primary);
	}

	.chat-header {
		padding: 12px 16px;
		border-bottom: 1px solid var(--border);
		display: flex;
		align-items: center;
		gap: 6px;
		min-height: 48px;
	}

	.chat-header .hash {
		color: var(--text-muted);
		font-size: 20px;
	}

	.chat-header .channel-name {
		font-weight: 600;
		font-size: 15px;
	}

	.messages {
		flex: 1;
		overflow-y: auto;
		padding: 16px 0;
		display: flex;
		flex-direction: column;
	}

	.load-more {
		text-align: center;
		padding: 8px;
	}

	.load-more button {
		padding: 4px 12px;
		color: var(--text-muted);
		font-size: 13px;
	}

	.load-more button:hover {
		color: var(--text-primary);
	}

	.message {
		padding: 2px 16px;
	}

	.message:not(.grouped) {
		margin-top: 16px;
	}

	.message:hover {
		background: var(--bg-hover);
	}

	.message-header {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 2px;
	}

	.avatar {
		width: 32px;
		height: 32px;
		border-radius: 50%;
		background: var(--accent);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 14px;
		font-weight: 600;
		color: white;
		flex-shrink: 0;
	}

	.author {
		font-weight: 600;
		font-size: 14px;
	}

	.timestamp {
		font-size: 12px;
		color: var(--text-muted);
	}

	.message-content {
		font-size: 14px;
		line-height: 1.4;
		word-wrap: break-word;
		white-space: pre-wrap;
	}

	.message-content.has-header {
		padding-left: 40px;
	}

	.grouped .message-content {
		padding-left: 40px;
	}

	.empty-state {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-muted);
	}
</style>
