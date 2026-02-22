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

export function createWSConnection(): WebSocket {
	const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
	const ws = new WebSocket(`${protocol}//${location.host}/api/ws`);

	ws.addEventListener('open', () => {
		ws.send(JSON.stringify({ type: 'presence_request' }));
	});

	ws.addEventListener('message', (event) => {
		try {
			const data = JSON.parse(event.data);
			if (data.type === 'presence_update' || data.type === 'presence_list') {
				handlePresenceMessage(data);
			}
		} catch {
			// Not JSON or not a presence message — ignore here.
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
