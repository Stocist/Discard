<script lang="ts">
	import { subscribeScreenShare, getScreenSharerUsername, getRemoteScreenStream, getScreenShareChannelId } from '$lib/voice';

	let { channelId }: { channelId: string } = $props();

	let screenState = $state(0);
	$effect(() => {
		return subscribeScreenShare(() => { screenState++; });
	});

	const sharerUsername = $derived.by(() => { screenState; return getScreenSharerUsername(); });
	const remoteStream = $derived.by(() => { screenState; return getRemoteScreenStream(); });
	const shareChannelId = $derived.by(() => { screenState; return getScreenShareChannelId(); });
	const isActiveHere = $derived(shareChannelId === channelId && remoteStream !== null);

	let videoEl = $state<HTMLVideoElement | null>(null);
	let isFullscreen = $state(false);
	let containerEl = $state<HTMLDivElement | null>(null);

	// Bind remote stream to video element
	$effect(() => {
		if (videoEl && remoteStream) {
			videoEl.srcObject = remoteStream;
			videoEl.play().catch(() => {});
		} else if (videoEl) {
			videoEl.srcObject = null;
		}
	});

	function toggleFullscreen() {
		if (!containerEl) return;
		if (document.fullscreenElement) {
			document.exitFullscreen();
			isFullscreen = false;
		} else {
			containerEl.requestFullscreen().then(() => {
				isFullscreen = true;
			}).catch(() => {});
		}
	}

	function popOut() {
		if (!remoteStream) return;
		const popup = window.open('', '_blank', 'width=1280,height=720,menubar=no,toolbar=no,location=no,status=no');
		if (!popup) return;
		popup.document.title = `${sharerUsername ?? 'User'}'s Screen`;
		popup.document.body.style.cssText = 'margin:0;padding:0;background:#000;overflow:hidden;display:flex;align-items:center;justify-content:center;height:100vh;';
		const video = popup.document.createElement('video');
		video.style.cssText = 'max-width:100%;max-height:100%;object-fit:contain;';
		video.autoplay = true;
		video.srcObject = remoteStream;
		popup.document.body.appendChild(video);
		video.play().catch(() => {});
	}

	// Listen for fullscreen changes
	function onFullscreenChange() {
		isFullscreen = !!document.fullscreenElement;
	}
</script>

<svelte:window onfullscreenchange={onFullscreenChange} />

{#if isActiveHere}
	<div class="screen-share-viewer" bind:this={containerEl}>
		<div class="screen-share-header">
			<span class="screen-share-label">{sharerUsername ?? 'Someone'} is sharing their screen</span>
			<div class="screen-share-controls">
				<button class="screen-ctrl-btn" title="Pop out" onclick={popOut}>
					<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
						<polyline points="15 3 21 3 21 9"></polyline>
						<line x1="10" y1="14" x2="21" y2="3"></line>
						<path d="M21 16v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5"></path>
					</svg>
				</button>
				<button class="screen-ctrl-btn" title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'} onclick={toggleFullscreen}>
					{#if isFullscreen}
						<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<polyline points="4 14 10 14 10 20"></polyline>
							<polyline points="20 10 14 10 14 4"></polyline>
							<line x1="14" y1="10" x2="21" y2="3"></line>
							<line x1="3" y1="21" x2="10" y2="14"></line>
						</svg>
					{:else}
						<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
							<polyline points="15 3 21 3 21 9"></polyline>
							<polyline points="9 21 3 21 3 15"></polyline>
							<line x1="21" y1="3" x2="14" y2="10"></line>
							<line x1="3" y1="21" x2="10" y2="14"></line>
						</svg>
					{/if}
				</button>
			</div>
		</div>
		<div class="screen-share-video-container">
			<video
				bind:this={videoEl}
				class="screen-share-video"
				autoplay
				playsinline
			></video>
		</div>
	</div>
{/if}

<style>
	.screen-share-viewer {
		background: #000;
		border: 1px solid var(--border);
		border-radius: 6px;
		overflow: hidden;
		display: flex;
		flex-direction: column;
		min-height: 200px;
		flex: 1;
	}

	.screen-share-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 6px 10px;
		background: var(--bg-secondary);
		border-bottom: 1px solid var(--border);
	}

	.screen-share-label {
		font-size: 12px;
		color: var(--accent);
		font-weight: 600;
	}

	.screen-share-controls {
		display: flex;
		gap: 4px;
	}

	.screen-ctrl-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 26px;
		height: 26px;
		border-radius: 4px;
		color: var(--text-muted);
		transition: background 0.15s, color 0.15s;
	}

	.screen-ctrl-btn:hover {
		background: var(--bg-hover);
		color: var(--text-primary);
	}

	.screen-share-video-container {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 0;
	}

	.screen-share-video {
		max-width: 100%;
		max-height: 100%;
		object-fit: contain;
	}
</style>
