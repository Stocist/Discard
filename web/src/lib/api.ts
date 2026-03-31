import type { Server, Channel, Message, ServerMember, User, DMChannelView } from './types';

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

// Tailscale status (public, no auth required)
export interface TailscaleStatus {
	on_tailscale: boolean;
	authenticated: boolean;
	dev_mode?: boolean;
	error?: string;
	user?: {
		id: string;
		username: string;
		display_name: string | null;
		avatar_path: string | null;
	};
}

export async function fetchTailscaleStatus(): Promise<TailscaleStatus> {
	const res = await fetch('/api/tailscale/status');
	if (!res.ok) {
		throw new Error(`Tailscale status check failed: ${res.status}`);
	}
	return res.json();
}

/** Resolve an avatar path: external URLs pass through, relative paths get /uploads/ prefix. */
export function avatarSrc(path: string): string {
	if (path.startsWith('http://') || path.startsWith('https://')) return path;
	return `/uploads/${path}`;
}

// Auth / user
export function fetchMe(): Promise<User> {
	return apiFetch('/me');
}

export async function updateMe(displayName?: string, avatar?: File): Promise<User> {
	const form = new FormData();
	if (displayName !== undefined) form.append('display_name', displayName);
	if (avatar) form.append('avatar', avatar);
	const res = await fetch('/api/me', {
		method: 'PUT',
		body: form
	});
	if (!res.ok) {
		const text = await res.text().catch(() => res.statusText);
		throw new ApiError(res.status, text);
	}
	return res.json();
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

export async function updateServer(serverId: string, name: string, icon?: File): Promise<Server> {
	if (icon) {
		const form = new FormData();
		form.append('name', name);
		form.append('icon', icon);
		const res = await fetch(`/api/servers/${serverId}`, {
			method: 'PUT',
			body: form
		});
		if (!res.ok) {
			const text = await res.text().catch(() => res.statusText);
			throw new ApiError(res.status, text);
		}
		return res.json();
	}
	return apiFetch(`/servers/${serverId}`, {
		method: 'PUT',
		body: JSON.stringify({ name })
	});
}

export function deleteServer(serverId: string): Promise<void> {
	return apiFetch(`/servers/${serverId}`, { method: 'DELETE' });
}

export function regenerateInviteCode(serverId: string): Promise<Server> {
	return apiFetch(`/servers/${serverId}/invite-code/regenerate`, { method: 'POST' });
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

export function updateChannel(serverId: string, channelId: string, name: string): Promise<Channel> {
	return apiFetch(`/servers/${serverId}/channels/${channelId}`, {
		method: 'PUT',
		body: JSON.stringify({ name })
	});
}

export function deleteChannel(serverId: string, channelId: string): Promise<void> {
	return apiFetch(`/servers/${serverId}/channels/${channelId}`, { method: 'DELETE' });
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

// DMs
export function openDM(userId: string): Promise<DMChannelView> {
	return apiFetch('/dm/open', { method: 'POST', body: JSON.stringify({ user_id: userId }) });
}

export function listDMs(): Promise<DMChannelView[]> {
	return apiFetch('/dm');
}

export function closeDM(channelId: string): Promise<void> {
	return apiFetch(`/dm/${channelId}/close`, { method: 'PUT' });
}

// Blocks
export function blockUser(userId: string): Promise<void> {
	return apiFetch('/blocks', { method: 'POST', body: JSON.stringify({ user_id: userId }) });
}

export function unblockUser(userId: string): Promise<void> {
	return apiFetch(`/blocks/${userId}`, { method: 'DELETE' });
}

export function listBlocks(): Promise<User[]> {
	return apiFetch('/blocks');
}

// Read state / unread
export function markChannelRead(channelId: string): Promise<void> {
	return apiFetch(`/channels/${channelId}/read`, { method: 'PUT' });
}

export function getUnreadCounts(serverId: string): Promise<Record<string, number>> {
	return apiFetch(`/servers/${serverId}/unread`);
}
