import type { Server, Channel, Message, ServerMember, Friendship, User } from './types';

class ApiError extends Error {
	constructor(
		public status: number,
		message: string
	) {
		super(message);
	}
}

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
	const res = await fetch(`/api${path}`, {
		...options,
		headers: {
			'Content-Type': 'application/json',
			...options?.headers
		}
	});
	if (!res.ok) {
		const text = await res.text().catch(() => res.statusText);
		throw new ApiError(res.status, text);
	}
	if (res.status === 204) return undefined as T;
	return res.json();
}

// Auth / user
export function fetchMe(): Promise<User> {
	return apiFetch('/me');
}

// Servers
export function createServer(name: string): Promise<Server> {
	return apiFetch('/servers', {
		method: 'POST',
		body: JSON.stringify({ name })
	});
}

export function listServers(): Promise<Server[]> {
	return apiFetch('/servers');
}

export function getServer(id: string): Promise<Server> {
	return apiFetch(`/servers/${id}`);
}

export function joinServer(inviteCode: string): Promise<Server> {
	return apiFetch('/servers/join', {
		method: 'POST',
		body: JSON.stringify({ invite_code: inviteCode })
	});
}

export function leaveServer(serverId: string): Promise<void> {
	return apiFetch(`/servers/${serverId}/members/me`, { method: 'DELETE' });
}

// Channels
export function createChannel(serverId: string, name: string, type?: string): Promise<Channel> {
	return apiFetch(`/servers/${serverId}/channels`, {
		method: 'POST',
		body: JSON.stringify({ name, type })
	});
}

export function listChannels(serverId: string): Promise<Channel[]> {
	return apiFetch(`/servers/${serverId}/channels`);
}

// Members
export function listMembers(serverId: string): Promise<ServerMember[]> {
	return apiFetch(`/servers/${serverId}/members`);
}

// Messages
export function listMessages(
	channelId: string,
	before?: string,
	limit?: number
): Promise<Message[]> {
	const params = new URLSearchParams();
	if (before) params.set('before', before);
	if (limit) params.set('limit', String(limit));
	const qs = params.toString();
	return apiFetch(`/channels/${channelId}/messages${qs ? `?${qs}` : ''}`);
}

// Create message (multipart — supports file attachments)
export async function createMessage(
	channelId: string,
	content: string,
	files?: File[]
): Promise<Message> {
	const form = new FormData();
	form.append('content', content);
	if (files) {
		for (const file of files) {
			form.append('files', file);
		}
	}
	// Do NOT use apiFetch here — it sets Content-Type to application/json.
	// For multipart/form-data the browser must set the boundary automatically.
	const res = await fetch(`/api/channels/${channelId}/messages`, {
		method: 'POST',
		body: form
	});
	if (!res.ok) {
		const text = await res.text().catch(() => res.statusText);
		throw new ApiError(res.status, text);
	}
	return res.json();
}

// Edit / Delete messages
export function editMessage(messageId: string, content: string): Promise<Message> {
	return apiFetch(`/messages/${messageId}`, {
		method: 'PUT',
		body: JSON.stringify({ content })
	});
}

export function deleteMessage(messageId: string): Promise<void> {
	return apiFetch(`/messages/${messageId}`, { method: 'DELETE' });
}

// Friends
export function sendFriendRequest(username: string): Promise<Friendship> {
	return apiFetch('/friends/requests', {
		method: 'POST',
		body: JSON.stringify({ username })
	});
}

export function acceptFriend(friendshipId: string): Promise<Friendship> {
	return apiFetch(`/friends/requests/${friendshipId}/accept`, { method: 'POST' });
}

export function listFriends(): Promise<Friendship[]> {
	return apiFetch('/friends');
}
