<script lang="ts">
	import { subscribeCameraState, getCameraParticipants, isCameraOn, getLocalCameraStream } from '$lib/voice';
	import type { VoiceParticipant } from '$lib/voice';

	let { participants, localUserId }: {
		participants: VoiceParticipant[];
		localUserId: string;
	} = $props();

	let cameraState = $state(0);
	$effect(() => {
		return subscribeCameraState(() => { cameraState++; });
	});

	const remoteCameras: Map<string, MediaStream> = $derived.by(() => {
		cameraState;
		return getCameraParticipants();
	});

	const localCameraOn = $derived.by(() => { cameraState; return isCameraOn(); });
	const localCameraStream = $derived.by(() => { cameraState; return getLocalCameraStream(); });

	// Build list of video entries: local + remote
	interface VideoEntry {
		userId: string;
		username: string;
		stream: MediaStream;
		isLocal: boolean;
	}

	const videoEntries: VideoEntry[] = $derived.by(() => {
		cameraState; // re-derive on camera state changes
		const entries: VideoEntry[] = [];

		// Local camera
		if (localCameraOn && localCameraStream) {
			const localP = participants.find(p => p.user_id === localUserId);
			entries.push({
				userId: localUserId,
				username: localP?.username ?? 'You',
				stream: localCameraStream,
				isLocal: true
			});
		}

		// Remote cameras
		for (const [userId, stream] of remoteCameras) {
			const p = participants.find(pp => pp.user_id === userId);
			entries.push({
				userId,
				username: p?.username ?? 'Unknown',
				stream,
				isLocal: false
			});
		}

		return entries;
	});

	const gridClass = $derived.by(() => {
		const count = videoEntries.length;
		if (count <= 1) return 'grid-1';
		if (count === 2) return 'grid-2';
		if (count <= 4) return 'grid-4';
		return 'grid-many';
	});

	// Bind streams to video elements
	function bindVideo(node: HTMLVideoElement, stream: MediaStream) {
		node.srcObject = stream;
		node.play().catch(() => {});
		return {
			update(newStream: MediaStream) {
				if (node.srcObject !== newStream) {
					node.srcObject = newStream;
					node.play().catch(() => {});
				}
			},
			destroy() {
				node.srcObject = null;
			}
		};
	}
</script>

{#if videoEntries.length > 0}
	<div class="video-grid {gridClass}">
		{#each videoEntries as entry (entry.userId)}
			<div class="video-cell">
				<video
					class="video-element"
					class:mirrored={entry.isLocal}
					autoplay
					playsinline
					muted={entry.isLocal}
					use:bindVideo={entry.stream}
				></video>
				<span class="video-username">{entry.isLocal ? 'You' : entry.username}</span>
			</div>
		{/each}
	</div>
{/if}

<style>
	.video-grid {
		display: grid;
		gap: 4px;
		padding: 8px;
		background: var(--bg-primary);
		border-bottom: 1px solid var(--border);
		max-height: 300px;
	}

	.grid-1 {
		grid-template-columns: 1fr;
	}

	.grid-2 {
		grid-template-columns: 1fr 1fr;
	}

	.grid-4 {
		grid-template-columns: 1fr 1fr;
		grid-template-rows: 1fr 1fr;
	}

	.grid-many {
		grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
	}

	.video-cell {
		position: relative;
		border-radius: 6px;
		overflow: hidden;
		background: var(--bg-secondary);
		aspect-ratio: 4 / 3;
	}

	.video-element {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
	}

	.video-element.mirrored {
		transform: scaleX(-1);
	}

	.video-username {
		position: absolute;
		bottom: 4px;
		left: 6px;
		font-size: 11px;
		color: #fff;
		background: rgba(0, 0, 0, 0.6);
		padding: 1px 6px;
		border-radius: 3px;
		pointer-events: none;
	}
</style>
