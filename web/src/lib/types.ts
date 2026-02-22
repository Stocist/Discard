export interface User {
	id: string;
	username: string;
	display_name: string | null;
	avatar_path: string | null;
	tailscale_id: string | null;
	status: string;
	created_at: string;
	updated_at: string;
}

export interface Server {
	id: string;
	name: string;
	icon_path: string | null;
	owner_id: string;
	invite_code: string | null;
	created_at: string;
}

export interface Channel {
	id: string;
	server_id: string | null;
	name: string | null;
	topic: string | null;
	type: string;
	position: number;
	created_at: string;
}

export interface Attachment {
	id: string;
	message_id: string;
	file_path: string;
	original_name: string;
	mime_type: string | null;
	file_size: number | null;
	width: number | null;
	height: number | null;
	created_at: string;
}

export interface Message {
	id: string;
	channel_id: string;
	author_id: string;
	content: string;
	edited: boolean;
	created_at: string;
	updated_at: string;
	author_username?: string;
	attachments?: Attachment[];
}

export interface ServerMember {
	user_id: string;
	server_id: string;
	nickname: string | null;
	joined_at: string;
	username?: string;
}

export interface Friendship {
	id: string;
	user_a: string;
	user_b: string;
	status: string;
	initiated_by: string;
	dm_channel_id: string | null;
	created_at: string;
	updated_at: string;
}

export interface WSMessage {
	type: 'message';
	message: Message;
}

export interface WSMessageEdit {
	type: 'message_edit';
	message: Message;
}

export interface WSMessageDelete {
	type: 'message_delete';
	channel_id: string;
	message_id: string;
}

export interface WSPresenceUpdate {
	type: 'presence_update';
	user_id: string;
	status: 'online' | 'offline';
}

export interface WSPresenceList {
	type: 'presence_list';
	user_ids: string[];
}
