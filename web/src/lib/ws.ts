import { handleVoiceMessage, cleanupVoiceOnDisconnect } from './voice';

// Module-level presence state.
// Use a callback pattern to notify subscribers since $state() only works in .svelte files.
let onlineSet = new Set<string>();
let listeners = new Set<() => void>();

function notify() {
	for (const fn of listeners) fn();
}

export function subscribePresence(fn: () => void): () => void {
	listeners.add(fn);
	return () => listeners.delete(fn);
}

export function getOnlineUsers(): Set<string> {
	return onlineSet;
}

export function isUserOnline(userId: string): boolean {
	return onlineSet.has(userId);
}

// Server event callbacks (update/delete).
import type { Server, Channel, User } from './types';

type ServerEventHandler = (event: { type: 'server_update'; server: Server } | { type: 'server_delete'; server_id: string }) => void;
let serverListeners = new Set<ServerEventHandler>();

export function subscribeServerEvents(fn: ServerEventHandler): () => void {
	serverListeners.add(fn);
	return () => serverListeners.delete(fn);
}

function handleServerMessage(data: { type: string; server?: Server; server_id?: string }) {
	if (data.type === 'server_update' && data.server) {
		for (const fn of serverListeners) fn({ type: 'server_update', server: data.server });
	} else if (data.type === 'server_delete' && data.server_id) {
		for (const fn of serverListeners) fn({ type: 'server_delete', server_id: data.server_id });
	}
}

function handlePresenceMessage(data: { type: string; user_id?: string; status?: string; user_ids?: string[] }) {
	if (data.type === 'presence_update' && data.user_id && data.status) {
		if (data.status === 'online') {
			onlineSet.add(data.user_id);
		} else {
			onlineSet.delete(data.user_id);
		}
		notify();
	} else if (data.type === 'presence_list' && data.user_ids) {
		onlineSet = new Set(data.user_ids);
		notify();
	}
}

// DM event callbacks.
type DMEvent = { type: 'dm_opened'; channel: Channel; opener: User };
type DMEventHandler = (event: DMEvent) => void;
let dmListeners = new Set<DMEventHandler>();

export function subscribeDMEvents(fn: DMEventHandler): () => void {
	dmListeners.add(fn);
	return () => dmListeners.delete(fn);
}

// Member joined callbacks — notified when a user joins a server.
export interface MemberJoinedEvent {
	type: 'member_joined';
	server_id: string;
	member: {
		user_id: string;
		username: string;
		display_name: string | null;
		avatar_url: string | null;
		role: string;
	};
}
type MemberJoinedHandler = (event: MemberJoinedEvent) => void;
let memberJoinedListeners = new Set<MemberJoinedHandler>();

export function subscribeMemberJoined(fn: MemberJoinedHandler): () => void {
	memberJoinedListeners.add(fn);
	return () => memberJoinedListeners.delete(fn);
}

// Unread message callbacks — notified with channel_id when a new message arrives.
type UnreadHandler = (channelId: string) => void;
let unreadListeners = new Set<UnreadHandler>();

export function subscribeUnread(fn: UnreadHandler): () => void {
	unreadListeners.add(fn);
	return () => unreadListeners.delete(fn);
}

export function createWSConnection(opts?: { handleVoice?: boolean }): WebSocket {
	const handleVoice = opts?.handleVoice ?? true;
	const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
	const ws = new WebSocket(`${protocol}//${location.host}/api/ws`);

	ws.addEventListener('open', () => {
		ws.send(JSON.stringify({ type: 'presence_request' }));
		if (handleVoice) {
			ws.send(JSON.stringify({ type: 'voice_state_request' }));
		}
	});

	ws.addEventListener('close', () => {
		// Don't cleanup voice on WS close — the WebRTC PeerConnection is independent
		// and survives WS reconnects. Voice is cleaned up by leaveVoice() or PC failure.
		// cleanupVoiceOnDisconnect is only called on permanent disconnects (page unload).
	});

	ws.addEventListener('error', () => {
		// Same as close — don't kill voice on transient WS errors.
	});

	ws.addEventListener('message', (event) => {
		try {
			const data = JSON.parse(event.data);
			if (data.type?.startsWith('voice_') || data.type?.startsWith('screen_share_')) {
				if (handleVoice) handleVoiceMessage(ws, data);
			} else if (data.type === 'presence_update' || data.type === 'presence_list') {
				handlePresenceMessage(data);
			} else if (data.type === 'server_update' || data.type === 'server_delete') {
				handleServerMessage(data);
			} else if (data.type === 'dm_opened' && data.channel && data.opener) {
				for (const fn of dmListeners) fn(data as DMEvent);
			} else if (data.type === 'member_joined' && data.server_id && data.member) {
				for (const fn of memberJoinedListeners) fn(data as MemberJoinedEvent);
			} else if (data.type === 'message' && data.message?.channel_id) {
				for (const fn of unreadListeners) fn(data.message.channel_id);
			}
		} catch {
			// Not JSON or not a known event — ignore here.
		}
	});

	return ws;
}

export function subscribe(ws: WebSocket, channelId: string) {
	ws.send(JSON.stringify({ type: 'subscribe', channel_id: channelId }));
}

export function unsubscribe(ws: WebSocket, channelId: string) {
	ws.send(JSON.stringify({ type: 'unsubscribe', channel_id: channelId }));
}

export function sendMessage(ws: WebSocket, channelId: string, content: string) {
	ws.send(JSON.stringify({ type: 'message', channel_id: channelId, content }));
}
