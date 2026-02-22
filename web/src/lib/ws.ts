export function createWSConnection(): WebSocket {
	const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
	return new WebSocket(`${protocol}//${location.host}/api/ws`);
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
