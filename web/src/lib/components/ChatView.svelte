<script lang="ts">
	import type { Message } from '$lib/types';
	import { listMessages, editMessage, deleteMessage } from '$lib/api';
	import { fetchMe } from '$lib/api';
	import { createWSConnection, subscribe, unsubscribe, sendMessage } from '$lib/ws';
	import { renderMarkdown } from '$lib/markdown';
	import MessageInput from './MessageInput.svelte';
	import AttachmentPreview from './AttachmentPreview.svelte';
	import ContextMenu from './ContextMenu.svelte';

	const AVATAR_COLORS = [
		'#b45309', '#a16207', '#4d7c0f', '#15803d',
		'#0e7490', '#1d4ed8', '#7e22ce', '#be123c',
		'#c2410c', '#0f766e',
	];

	function avatarColor(username: string): string {
		let hash = 0;
		for (let i = 0; i < username.length; i++) {
			hash = ((hash << 5) - hash + username.charCodeAt(i)) | 0;
		}
		return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length];
	}

	let { channelId, channelName, onToggleMembers }: {
		channelId: string;
		channelName: string;
		onToggleMembers?: () => void;
	} = $props();

	let messages = $state<Message[]>([]);
	let messagesEl: HTMLDivElement | undefined = $state();
	let loadingMore = $state(false);
	let hasMore = $state(true);
	let isAtBottom = $state(true);
	let activeConn: WebSocket | undefined = $state();

	// Current user ID for ownership checks
	let currentUserId = $state<string | null>(null);

	// Context menu state
	let contextMenu = $state<{ x: number; y: number; message: Message } | null>(null);

	// Inline edit state
	let editingMessageId = $state<string | null>(null);
	let editContent = $state('');

	// Load current user on mount
	$effect(() => {
		fetchMe().then(u => { currentUserId = u.id; }).catch(() => {});
	});

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
		editingMessageId = null;
		contextMenu = null;

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
				} else if (data.type === 'message_edit' && data.message) {
					const edited = data.message as Message;
					messages = messages.map(m => m.id === edited.id ? edited : m);
				} else if (data.type === 'message_delete' && data.message_id) {
					messages = messages.filter(m => m.id !== data.message_id);
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

	// Context menu
	function handleContextMenu(e: MouseEvent, message: Message) {
		e.preventDefault();
		contextMenu = { x: e.clientX, y: e.clientY, message };
	}

	function getContextMenuItems(message: Message) {
		const items: { label: string; action: () => void; danger?: boolean }[] = [];

		items.push({
			label: 'Copy Text',
			action: () => { navigator.clipboard.writeText(message.content); }
		});

		if (currentUserId && message.author_id === currentUserId) {
			items.push({
				label: 'Edit Message',
				action: () => {
					editingMessageId = message.id;
					editContent = message.content;
				}
			});
			items.push({
				label: 'Delete Message',
				danger: true,
				action: async () => {
					try {
						await deleteMessage(message.id);
						// WS broadcast will remove it, but also remove locally for instant feedback
						messages = messages.filter(m => m.id !== message.id);
					} catch (err) {
						console.error('Failed to delete message:', err);
					}
				}
			});
		}

		return items;
	}

	// Inline edit
	async function handleEditSave(messageId: string) {
		const trimmed = editContent.trim();
		if (!trimmed) return;
		try {
			await editMessage(messageId, trimmed);
			// WS broadcast will update the message in the list
		} catch (err) {
			console.error('Failed to edit message:', err);
		}
		editingMessageId = null;
		editContent = '';
	}

	function handleEditCancel() {
		editingMessageId = null;
		editContent = '';
	}

	function handleEditKeydown(e: KeyboardEvent, messageId: string) {
		if (e.key === 'Escape') {
			handleEditCancel();
		} else if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			handleEditSave(messageId);
		}
	}
</script>

<div class="chat-view">
	<div class="chat-header">
		<span class="hash">#</span>
		<span class="channel-name">{channelName}</span>
		{#if onToggleMembers}
			<button class="header-btn" title="Toggle member list" onclick={onToggleMembers}>
				<svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
					<path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/>
				</svg>
			</button>
		{/if}
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
			<div
				class="message"
				class:grouped
				oncontextmenu={(e) => handleContextMenu(e, message)}
			>
				{#if !grouped}
					<div class="message-header">
						{#if message.author_avatar_url}
							<img class="avatar" src="/uploads/{message.author_avatar_url}" alt="" />
						{:else}
							<span class="avatar" style="background: {avatarColor(message.author_username ?? message.author_id)}">{(message.author_username ?? message.author_id).charAt(0).toUpperCase()}</span>
						{/if}
						<span class="author">{message.author_display_name ?? message.author_username ?? message.author_id}</span>
						<span class="timestamp">{formatTime(message.created_at)}</span>
					</div>
				{/if}
				{#if editingMessageId === message.id}
					<div class="edit-container" class:has-header={!grouped}>
						<textarea
							class="edit-textarea"
							bind:value={editContent}
							onkeydown={(e) => handleEditKeydown(e, message.id)}
						></textarea>
						<div class="edit-actions">
							<span class="edit-hint">Escape to cancel, Enter to save</span>
							<button class="edit-btn cancel" onclick={handleEditCancel}>Cancel</button>
							<button class="edit-btn save" onclick={() => handleEditSave(message.id)}>Save</button>
						</div>
					</div>
				{:else}
					<div class="message-content" class:has-header={!grouped}>
						{@html renderMarkdown(message.content)}
						{#if message.edited}
							<span class="edited-tag">(edited)</span>
						{/if}
					</div>
				{/if}
				{#if message.attachments && message.attachments.length > 0}
					<div class="message-attachments" class:has-header={!grouped}>
						<AttachmentPreview attachments={message.attachments} />
					</div>
				{/if}
			</div>
		{/each}

		{#if messages.length === 0}
			<div class="empty-state">
				<p>No messages yet. Start the conversation!</p>
			</div>
		{/if}
	</div>

	<MessageInput {channelId} {channelName} onSend={handleSend} />
</div>

{#if contextMenu}
	<ContextMenu
		x={contextMenu.x}
		y={contextMenu.y}
		items={getContextMenuItems(contextMenu.message)}
		onClose={() => { contextMenu = null; }}
	/>
{/if}

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

	.header-btn {
		margin-left: auto;
		color: var(--text-muted);
		padding: 4px;
		border-radius: 4px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.header-btn:hover {
		color: var(--text-primary);
		background: var(--bg-hover);
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
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 14px;
		font-weight: 600;
		color: white;
		flex-shrink: 0;
		object-fit: cover;
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
	}

	.message-content.has-header {
		padding-left: 40px;
	}

	.grouped .message-content {
		padding-left: 40px;
	}

	.edited-tag {
		font-size: 11px;
		color: var(--text-muted);
		margin-left: 4px;
	}

	.edit-container {
		padding: 4px 0;
	}

	.edit-container.has-header {
		padding-left: 40px;
	}

	.grouped .edit-container {
		padding-left: 40px;
	}

	.edit-textarea {
		width: 100%;
		min-height: 60px;
		padding: 8px 12px;
		background: var(--bg-input);
		color: var(--text-primary);
		border: 1px solid var(--accent);
		border-radius: 6px;
		font-family: inherit;
		font-size: 14px;
		line-height: 1.4;
		resize: vertical;
		outline: none;
	}

	.edit-actions {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-top: 4px;
	}

	.edit-hint {
		font-size: 11px;
		color: var(--text-muted);
		flex: 1;
	}

	.edit-btn {
		padding: 4px 12px;
		border-radius: 4px;
		font-size: 13px;
		cursor: pointer;
	}

	.edit-btn.cancel {
		color: var(--text-muted);
	}

	.edit-btn.cancel:hover {
		color: var(--text-primary);
	}

	.edit-btn.save {
		background: var(--accent);
		color: #1c1917;
		font-weight: 600;
	}

	.edit-btn.save:hover {
		background: var(--accent-hover);
	}

	.message-attachments.has-header {
		padding-left: 40px;
	}

	.grouped .message-attachments {
		padding-left: 40px;
	}

	.empty-state {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-muted);
	}

	@media (max-width: 768px) {
		.message {
			padding: 2px 8px;
		}

		.message-content.has-header,
		.grouped .message-content {
			padding-left: 40px;
		}

		.chat-header {
			padding: 12px 8px;
			padding-left: 48px;
		}
	}
</style>
