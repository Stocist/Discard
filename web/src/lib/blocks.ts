import { listBlocks } from './api';

let blockedByMe = new Set<string>();
let blockedEither = new Set<string>();
let revisions = new Map<string, number>();
let listeners = new Set<() => void>();
let initialization: Promise<void> | null = null;

function notify() {
	for (const listener of listeners) listener();
}

export function subscribeBlockState(listener: () => void): () => void {
	listeners.add(listener);
	return () => listeners.delete(listener);
}

export function initializeBlockState(): Promise<void> {
	if (initialization) return initialization;
	initialization = listBlocks()
		.then((users) => {
			blockedByMe = new Set(users.map((user) => user.id));
			for (const id of blockedByMe) {
				blockedEither.add(id);
			}
			notify();
		})
		.catch((error) => {
			initialization = null;
			throw error;
		});
	return initialization;
}

export function applyBlockState(userId: string, isBlockedEither: boolean, isBlockedByMe: boolean) {
	revisions.set(userId, (revisions.get(userId) ?? 0) + 1);
	if (isBlockedEither) blockedEither.add(userId);
	else blockedEither.delete(userId);
	if (isBlockedByMe) blockedByMe.add(userId);
	else blockedByMe.delete(userId);
	notify();
}

export function captureBlockStateRevisions(): ReadonlyMap<string, number> {
	return new Map(revisions);
}

export function primeConversationBlockState(
	userId: string,
	canMessage: boolean,
	requestRevisions?: ReadonlyMap<string, number>
) {
	if (requestRevisions && (requestRevisions.get(userId) ?? 0) !== (revisions.get(userId) ?? 0)) return;
	if (canMessage) blockedEither.delete(userId);
	else blockedEither.add(userId);
	notify();
}

export function isBlockedByMe(userId: string): boolean {
	return blockedByMe.has(userId);
}

export function isBlockedEither(userId: string): boolean {
	return blockedEither.has(userId);
}
