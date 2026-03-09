<script lang="ts">
	import { toggleMute, toggleDeafen, leaveVoice, isMuted, isDeafened, isSelfSpeaking, getParticipants, subscribeVoice, getVoiceChannelId, startMicTest, getVoiceError, subscribeVoiceError } from '$lib/voice';
	import type { VoiceParticipant } from '$lib/voice';

	let { ws, channelName }: {
		ws: WebSocket | undefined;
		channelName: string;
	} = $props();

	let voiceState = $state(0);
	let voiceError = $state<string | null>(null);
	$effect(() => {
		return subscribeVoice(() => { voiceState++; });
	});
	$effect(() => {
		voiceError = getVoiceError();
		return subscribeVoiceError((msg) => { voiceError = msg; });
	});

	const muted = $derived.by(() => { voiceState; return isMuted(); });
	const deafened = $derived.by(() => { voiceState; return isDeafened(); });
	const speaking = $derived.by(() => { voiceState; return isSelfSpeaking(); });
	const channelId = $derived.by(() => { voiceState; return getVoiceChannelId(); });
	const participants: VoiceParticipant[] = $derived.by(() => {
		voiceState;
		return channelId ? getParticipants(channelId) : [];
	});

	function handleMute() {
		if (ws) toggleMute(ws);
	}

	function handleDeafen() {
		if (ws) toggleDeafen(ws);
	}

	let micTesting = $state(false);
	let micLevel = $state(0);
	let micTestCleanup: (() => void) | null = null;

	let micTestError = $state<string | null>(null);

	async function handleMicTest() {
		if (micTesting) {
			stopMicTest();
			return;
		}
		micTestError = null;
		try {
			micTesting = true;
			micTestCleanup = await startMicTest((level) => {
				micLevel = level;
			});
		} catch (e) {
			micTesting = false;
			micTestError = e instanceof Error ? e.message : 'Failed to access mic';
		}
	}

	function stopMicTest() {
		if (micTestCleanup) {
			micTestCleanup();
			micTestCleanup = null;
		}
		micTesting = false;
		micLevel = 0;
	}

	function handleDisconnect() {
		stopMicTest();
		if (ws) leaveVoice(ws);
	}
</script>

<div class="voice-panel">
	<div class="voice-info">
		<span class="voice-label" class:speaking>Voice Connected</span>
		<span class="voice-channel">{channelName}</span>
	</div>
	<div class="voice-participants">
		{#each participants as p (p.user_id)}
			<div class="voice-participant" class:speaking={p.speaking}>
				{#if p.avatar_path}
					<img class="voice-avatar" src="/uploads/{p.avatar_path}" alt="" />
				{:else}
					<span class="voice-avatar voice-avatar-fallback">{(p.username ?? '?').charAt(0).toUpperCase()}</span>
				{/if}
				<span class="voice-username">{p.username ?? 'Unknown'}</span>
				{#if p.muted}
					<svg class="voice-status-icon" width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
						<path d="M19 11h-1.7c0 .74-.16 1.43-.43 2.05l1.23 1.23c.56-.98.9-2.09.9-3.28zm-4.02.17c0-.06.02-.11.02-.17V5c0-1.66-1.34-3-3-3S9 3.34 9 5v.18l5.98 5.99zM4.27 3L3 4.27l6.01 6.01V11c0 1.66 1.33 3 2.99 3 .22 0 .44-.03.65-.08l1.66 1.66c-.71.33-1.5.52-2.31.52-2.76 0-5.3-2.1-5.3-5.1H5c0 3.41 2.72 6.23 6 6.72V21h2v-3.28c.91-.13 1.77-.45 2.54-.9L19.73 21 21 19.73 4.27 3z"/>
					</svg>
				{/if}
				{#if p.deafened}
					<svg class="voice-status-icon" width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
						<path d="M12 4C7.58 4 4 7.58 4 12v4c0 1.1.9 2 2 2h1v-7H5.5v-.5C5.5 7.46 8.46 4.5 12 4.5s6.5 2.96 6.5 6.5v.5H17v7h1c1.1 0 2-.9 2-2v-4c0-4.42-3.58-8-8-8zM7 18H6v-5h1v5zm11 0h-1v-5h1v5z"/>
						<line x1="3" y1="3" x2="21" y2="21" stroke="currentColor" stroke-width="2"/>
					</svg>
				{/if}
			</div>
		{/each}
	</div>
	{#if voiceError}
		<div class="voice-error">{voiceError}</div>
	{/if}
	<div class="mic-test-section">
		<button class="mic-test-btn" class:active={micTesting} onclick={handleMicTest}>
			{micTesting ? 'Stop Test' : 'Test Mic'}
		</button>
		{#if micTesting}
			<div class="mic-level-track">
				<div class="mic-level-fill" style="width: {micLevel}%"></div>
			</div>
		{/if}
	</div>
	{#if micTestError}
		<div class="voice-error">{micTestError}</div>
	{/if}
	<div class="voice-controls">
		<button
			class="voice-btn"
			class:active={muted}
			title={muted ? 'Unmute' : 'Mute'}
			onclick={handleMute}
		>
			{#if muted}
				<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
					<path d="M19 11h-1.7c0 .74-.16 1.43-.43 2.05l1.23 1.23c.56-.98.9-2.09.9-3.28zm-4.02.17c0-.06.02-.11.02-.17V5c0-1.66-1.34-3-3-3S9 3.34 9 5v.18l5.98 5.99zM4.27 3L3 4.27l6.01 6.01V11c0 1.66 1.33 3 2.99 3 .22 0 .44-.03.65-.08l1.66 1.66c-.71.33-1.5.52-2.31.52-2.76 0-5.3-2.1-5.3-5.1H5c0 3.41 2.72 6.23 6 6.72V21h2v-3.28c.91-.13 1.77-.45 2.54-.9L19.73 21 21 19.73 4.27 3z"/>
				</svg>
			{:else}
				<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
					<path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3zm5.91-3c-.49 0-.9.36-.98.85C16.52 14.2 14.47 16 12 16s-4.52-1.8-4.93-4.15c-.08-.49-.49-.85-.98-.85-.61 0-1.09.54-1 1.14.49 3 2.89 5.35 5.91 5.78V21h2v-3.08c3.02-.43 5.42-2.78 5.91-5.78.1-.6-.39-1.14-1-1.14z"/>
				</svg>
			{/if}
		</button>
		<button
			class="voice-btn"
			class:active={deafened}
			title={deafened ? 'Undeafen' : 'Deafen'}
			onclick={handleDeafen}
		>
			{#if deafened}
				<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
					<path d="M12 4C7.58 4 4 7.58 4 12v4c0 1.1.9 2 2 2h1v-7H5.5v-.5C5.5 7.46 8.46 4.5 12 4.5s6.5 2.96 6.5 6.5v.5H17v7h1c1.1 0 2-.9 2-2v-4c0-4.42-3.58-8-8-8zM7 18H6v-5h1v5zm11 0h-1v-5h1v5z"/>
					<line x1="3" y1="3" x2="21" y2="21" stroke="currentColor" stroke-width="2"/>
				</svg>
			{:else}
				<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
					<path d="M12 4C7.58 4 4 7.58 4 12v4c0 1.1.9 2 2 2h1v-7H5.5v-.5C5.5 7.46 8.46 4.5 12 4.5s6.5 2.96 6.5 6.5v.5H17v7h1c1.1 0 2-.9 2-2v-4c0-4.42-3.58-8-8-8zM7 18H6v-5h1v5zm11 0h-1v-5h1v5z"/>
				</svg>
			{/if}
		</button>
		<button
			class="voice-btn disconnect"
			title="Disconnect"
			onclick={handleDisconnect}
		>
			<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
				<path d="M12 9c-1.6 0-3.15.25-4.6.72v3.1c0 .39-.23.74-.56.9-.98.49-1.87 1.12-2.66 1.85-.18.18-.43.28-.7.28-.28 0-.53-.11-.71-.29L.29 13.08a.956.956 0 01-.29-.7c0-.28.11-.53.29-.71C3.34 8.78 7.46 7 12 7s8.66 1.78 11.71 4.67c.18.18.29.43.29.71 0 .28-.11.53-.29.71l-2.48 2.48c-.18.18-.43.29-.71.29-.27 0-.52-.11-.7-.28a11.27 11.27 0 00-2.67-1.85.996.996 0 01-.56-.9v-3.1C15.15 9.25 13.6 9 12 9z"/>
			</svg>
		</button>
	</div>
</div>

<style>
	.voice-panel {
		background: var(--bg-primary);
		border-top: 1px solid var(--border);
		padding: 8px 12px;
	}

	.voice-info {
		display: flex;
		flex-direction: column;
		margin-bottom: 6px;
	}

	.voice-label {
		font-size: 11px;
		font-weight: 700;
		color: var(--accent);
		text-transform: uppercase;
		letter-spacing: 0.02em;
		transition: text-shadow 0.15s;
	}

	.voice-label.speaking {
		text-shadow: 0 0 8px var(--accent);
	}

	.voice-channel {
		font-size: 12px;
		color: var(--text-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.voice-participants {
		display: flex;
		flex-direction: column;
		gap: 4px;
		margin-bottom: 8px;
		max-height: 120px;
		overflow-y: auto;
	}

	.voice-participant {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 2px 4px;
		border-radius: 4px;
	}

	.voice-participant.speaking {
		background: rgba(132, 204, 22, 0.1);
	}

	.voice-avatar {
		width: 22px;
		height: 22px;
		border-radius: 50%;
		object-fit: cover;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		transition: box-shadow 0.15s;
	}

	.voice-participant.speaking .voice-avatar {
		box-shadow: 0 0 0 2px var(--accent);
	}

	.voice-avatar-fallback {
		background: var(--bg-tertiary);
		color: var(--text-muted);
		font-size: 10px;
		font-weight: 600;
	}

	.voice-username {
		font-size: 12px;
		color: var(--text-primary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.voice-status-icon {
		color: var(--text-muted);
		flex-shrink: 0;
	}

	.voice-controls {
		display: flex;
		gap: 4px;
		justify-content: center;
	}

	.voice-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 32px;
		height: 32px;
		border-radius: 50%;
		background: var(--bg-tertiary);
		color: var(--text-muted);
		transition: background 0.15s, color 0.15s;
	}

	.voice-btn:hover {
		background: var(--bg-hover);
		color: var(--text-primary);
	}

	.voice-btn.active {
		background: #dc2626;
		color: white;
	}

	.voice-btn.disconnect {
		background: #dc2626;
		color: white;
	}

	.voice-btn.disconnect:hover {
		background: #b91c1c;
	}

	.voice-error {
		font-size: 11px;
		color: #ef4444;
		background: rgba(239, 68, 68, 0.1);
		padding: 4px 8px;
		border-radius: 4px;
		margin-bottom: 6px;
		line-height: 1.3;
		word-break: break-word;
	}

	.mic-test-section {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 8px;
	}

	.mic-test-btn {
		font-size: 11px;
		padding: 3px 10px;
		border-radius: 4px;
		background: var(--bg-tertiary);
		color: var(--text-muted);
		white-space: nowrap;
		transition: background 0.15s, color 0.15s;
	}

	.mic-test-btn:hover {
		background: var(--bg-hover);
		color: var(--text-primary);
	}

	.mic-test-btn.active {
		background: var(--accent);
		color: var(--bg-primary);
	}

	.mic-level-track {
		flex: 1;
		height: 6px;
		background: var(--bg-tertiary);
		border-radius: 3px;
		overflow: hidden;
	}

	.mic-level-fill {
		height: 100%;
		background: var(--accent);
		border-radius: 3px;
		transition: width 0.05s linear;
	}
</style>
