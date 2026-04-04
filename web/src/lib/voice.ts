// Voice channel state management.
// Uses callback pattern (same as presence in ws.ts) since $state() doesn't work in .ts files.

export interface VoiceParticipant {
	user_id: string;
	username?: string;
	avatar_path?: string | null;
	muted: boolean;
	deafened: boolean;
	speaking: boolean;
	screen_sharing: boolean;
	camera_on: boolean;
}

// Module state (plain variables, NOT $state)
let currentChannelId: string | null = null;
let muted = false;
let deafened = false;
let participants: Map<string, VoiceParticipant[]> = new Map();
let pc: RTCPeerConnection | null = null;
let localStream: MediaStream | null = null;
let listeners = new Set<() => void>();
let speakingInterval: ReturnType<typeof setInterval> | null = null;
let audioContext: AudioContext | null = null;
let analyser: AnalyserNode | null = null;

// Screen share state
let screenStream: MediaStream | null = null;
let screenSharing = false;
let screenSharerUserId: string | null = null;
let screenSharerUsername: string | null = null;
let screenShareChannelId: string | null = null;
let screenShareListeners = new Set<() => void>();
let remoteScreenStream: MediaStream | null = null;

// Camera state
let cameraStream: MediaStream | null = null;
let cameraOn = false;
let cameraParticipants: Map<string, MediaStream> = new Map(); // remote camera streams keyed by user ID
let cameraListeners = new Set<() => void>();

// Error state — surfaced to UI
let lastError: string | null = null;
let errorListeners = new Set<(msg: string | null) => void>();

// Local speaking state (direct, no server round-trip)
let selfSpeaking = false;

function notify() {
	for (const fn of listeners) fn();
}

function setError(msg: string | null) {
	lastError = msg;
	for (const fn of errorListeners) fn(msg);
	if (msg) console.error('[voice]', msg);
}

export function getVoiceError(): string | null {
	return lastError;
}

export function subscribeVoiceError(fn: (msg: string | null) => void): () => void {
	errorListeners.add(fn);
	return () => errorListeners.delete(fn);
}

function wsSend(ws: WebSocket, data: Record<string, unknown>) {
	if (ws.readyState === WebSocket.OPEN) {
		try {
			ws.send(JSON.stringify(data));
		} catch (e) {
			console.warn('[voice] wsSend failed:', e);
		}
	}
}

export function joinVoice(ws: WebSocket, channelId: string): void {
	setError(null);

	// Check secure context (getUserMedia requires localhost or HTTPS)
	if (typeof window !== 'undefined' && !window.isSecureContext) {
		setError('Mic access requires HTTPS or localhost. Current page is not a secure context.');
		return;
	}
	if (typeof navigator === 'undefined' || !navigator.mediaDevices) {
		setError('navigator.mediaDevices not available — are you on HTTPS or localhost?');
		return;
	}

	if (currentChannelId) {
		leaveVoice(ws);
	}
	currentChannelId = channelId;
	notify();
	console.log('[voice] joining channel', channelId, 'ws state:', ws.readyState);
	wsSend(ws, { type: 'voice_join', channel_id: channelId });
}

export function leaveVoice(ws: WebSocket): void {
	if (!currentChannelId) return;
	// Stop screen share if active
	if (screenSharing) {
		stopScreenShare(ws);
	}
	// Stop camera if active
	if (cameraOn) {
		stopCamera(ws);
	}
	wsSend(ws, { type: 'voice_leave', channel_id: currentChannelId });
	stopSpeakingDetection();
	if (pc) {
		pc.close();
		pc = null;
	}
	if (localStream) {
		localStream.getTracks().forEach(t => t.stop());
		localStream = null;
	}
	// Clean up remote audio/video elements
	document.querySelectorAll('audio[data-voice-remote]').forEach(el => el.remove());
	document.querySelectorAll('audio[data-screen-audio]').forEach(el => el.remove());
	remoteScreenStream = null;
	screenSharerUserId = null;
	screenSharerUsername = null;
	screenShareChannelId = null;
	cameraParticipants.clear();
	currentChannelId = null;
	muted = false;
	deafened = false;
	notify();
	notifyScreenShare();
	notifyCamera();
}

export function toggleMute(ws: WebSocket): void {
	muted = !muted;
	if (localStream) {
		localStream.getAudioTracks().forEach(t => {
			t.enabled = !muted;
		});
	}
	if (currentChannelId) {
		wsSend(ws, { type: 'voice_mute', channel_id: currentChannelId, muted });
	}
	notify();
}

export function toggleDeafen(ws: WebSocket): void {
	deafened = !deafened;
	// Disable all remote audio elements
	document.querySelectorAll<HTMLAudioElement>('audio[data-voice-remote]').forEach(el => {
		el.muted = deafened;
	});
	if (currentChannelId) {
		wsSend(ws, { type: 'voice_deafen', channel_id: currentChannelId, deafened });
	}
	notify();
}

export function getVoiceChannelId(): string | null {
	return currentChannelId;
}

export function isMuted(): boolean {
	return muted;
}

export function isDeafened(): boolean {
	return deafened;
}

export function isSelfSpeaking(): boolean {
	return selfSpeaking;
}

export function getParticipants(channelId: string): VoiceParticipant[] {
	return participants.get(channelId) ?? [];
}

export function getAllVoiceParticipants(): Map<string, VoiceParticipant[]> {
	return participants;
}

export function subscribeVoice(fn: () => void): () => void {
	listeners.add(fn);
	return () => listeners.delete(fn);
}

// Screen share state accessors
function notifyScreenShare() {
	for (const fn of screenShareListeners) fn();
}

export function subscribeScreenShare(fn: () => void): () => void {
	screenShareListeners.add(fn);
	return () => screenShareListeners.delete(fn);
}

export function isScreenSharing(): boolean {
	return screenSharing;
}

export function getScreenSharerUserId(): string | null {
	return screenSharerUserId;
}

export function getScreenSharerUsername(): string | null {
	return screenSharerUsername;
}

export function getScreenShareChannelId(): string | null {
	return screenShareChannelId;
}

export function getRemoteScreenStream(): MediaStream | null {
	return remoteScreenStream;
}

// Camera state accessors
function notifyCamera() {
	for (const fn of cameraListeners) fn();
}

export function subscribeCameraState(fn: () => void): () => void {
	cameraListeners.add(fn);
	return () => cameraListeners.delete(fn);
}

export function isCameraOn(): boolean {
	return cameraOn;
}

export function getCameraParticipants(): Map<string, MediaStream> {
	return cameraParticipants;
}

export function getLocalCameraStream(): MediaStream | null {
	return cameraStream;
}

export async function startCamera(ws: WebSocket): Promise<void> {
	if (!currentChannelId || !pc) {
		setError('Must be in a voice channel to use camera');
		return;
	}
	if (cameraOn) return;

	try {
		cameraStream = await navigator.mediaDevices.getUserMedia({
			video: { width: { ideal: 640 }, height: { ideal: 480 } },
			audio: false
		});

		// Add camera video track to the existing PeerConnection with a "camera-" stream ID
		for (const track of cameraStream.getTracks()) {
			const cameraMediaStream = new MediaStream([track]);
			// Override stream ID by creating a custom one — use addTrack with custom stream
			pc.addTrack(track, cameraMediaStream);
		}

		cameraOn = true;
		wsSend(ws, { type: 'voice_camera_start', channel_id: currentChannelId });
		notify();
		notifyCamera();
	} catch (e) {
		if (e instanceof DOMException && e.name === 'NotAllowedError') {
			return;
		}
		const msg = e instanceof Error ? e.message : String(e);
		setError('Camera failed: ' + msg);
	}
}

export function stopCamera(ws: WebSocket): void {
	if (!cameraOn) return;

	if (cameraStream) {
		if (pc) {
			for (const track of cameraStream.getTracks()) {
				const sender = pc.getSenders().find(s => s.track === track);
				if (sender) {
					pc.removeTrack(sender);
				}
			}
		}
		cameraStream.getTracks().forEach(t => t.stop());
		cameraStream = null;
	}

	cameraOn = false;
	if (currentChannelId) {
		wsSend(ws, { type: 'voice_camera_stop', channel_id: currentChannelId });
	}
	notify();
	notifyCamera();
}

export async function startScreenShare(ws: WebSocket, resolution?: string): Promise<void> {
	if (!currentChannelId || !pc) {
		setError('Must be in a voice channel to share screen');
		return;
	}
	if (screenSharing) return;
	if (!navigator.mediaDevices?.getDisplayMedia) {
		setError('Screen sharing is not supported on this device');
		return;
	}

	try {
		let videoConstraints: boolean | MediaTrackConstraints = true;
		if (resolution === '720p') {
			videoConstraints = { height: { ideal: 720 } };
		} else if (resolution === '1080p') {
			videoConstraints = { height: { ideal: 1080 } };
		} else if (resolution === '4k') {
			videoConstraints = { height: { ideal: 2160 } };
		}

		screenStream = await navigator.mediaDevices.getDisplayMedia({
			video: videoConstraints,
			audio: true
		});

		// Add screen share tracks to the existing PeerConnection
		for (const track of screenStream.getTracks()) {
			pc.addTrack(track, screenStream);
		}

		// Listen for user stopping share via browser UI
		screenStream.getVideoTracks()[0]?.addEventListener('ended', () => {
			stopScreenShare(ws);
		});

		// Some browsers interrupt getUserMedia when getDisplayMedia is called.
		// Re-check mic track is still live; re-acquire if needed.
		if (localStream && pc) {
			const micTrack = localStream.getAudioTracks()[0];
			if (!micTrack || micTrack.readyState === 'ended') {
				try {
					const newStream = await navigator.mediaDevices.getUserMedia({
						audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }
					});
					localStream = newStream;
					const newTrack = newStream.getAudioTracks()[0];
					if (newTrack) {
						newTrack.enabled = !muted;
						// Find the sender that was carrying our voice audio and replace its track
						const voiceSender = pc.getSenders().find(s =>
							s.track === null || s.track === micTrack
						);
						if (voiceSender) {
							await voiceSender.replaceTrack(newTrack);
						}
					}
				} catch (_) { /* mic re-acquire failed, voice will be muted */ }
			}
		}

		screenSharing = true;
		wsSend(ws, { type: 'screen_share_start', channel_id: currentChannelId });
		notify();
		notifyScreenShare();
	} catch (e) {
		// User cancelled the picker — not an error
		if (e instanceof DOMException && e.name === 'NotAllowedError') {
			return;
		}
		const msg = e instanceof Error ? e.message : String(e);
		setError('Screen share failed: ' + msg);
	}
}

export function stopScreenShare(ws: WebSocket): void {
	if (!screenSharing) return;

	if (screenStream) {
		// Remove screen share tracks from PeerConnection
		if (pc) {
			for (const track of screenStream.getTracks()) {
				const sender = pc.getSenders().find(s => s.track === track);
				if (sender) {
					pc.removeTrack(sender);
				}
			}
		}
		screenStream.getTracks().forEach(t => t.stop());
		screenStream = null;
	}

	screenSharing = false;
	if (currentChannelId) {
		wsSend(ws, { type: 'screen_share_stop', channel_id: currentChannelId });
	}
	notify();
	notifyScreenShare();
}

// cleanupVoiceOnDisconnect handles ungraceful WS disconnect: cleans up audio elements,
// PeerConnection, and local stream without trying to send WS messages.
export function cleanupVoiceOnDisconnect(): void {
	stopSpeakingDetection();
	if (screenStream) {
		screenStream.getTracks().forEach(t => t.stop());
		screenStream = null;
	}
	screenSharing = false;
	if (cameraStream) {
		cameraStream.getTracks().forEach(t => t.stop());
		cameraStream = null;
	}
	cameraOn = false;
	cameraParticipants.clear();
	if (pc) {
		pc.close();
		pc = null;
	}
	if (localStream) {
		localStream.getTracks().forEach(t => t.stop());
		localStream = null;
	}
	// Remove all remote audio/video elements
	document.querySelectorAll('audio[data-voice-remote]').forEach(el => el.remove());
	document.querySelectorAll('audio[data-screen-audio]').forEach(el => el.remove());
	remoteScreenStream = null;
	screenSharerUserId = null;
	screenSharerUsername = null;
	screenShareChannelId = null;
	if (currentChannelId) {
		currentChannelId = null;
		muted = false;
		deafened = false;
		notify();
		notifyScreenShare();
		notifyCamera();
	}
}

// Called from ws.ts for all voice_* and screen_share_* messages
export function handleVoiceMessage(ws: WebSocket, data: Record<string, unknown>): void {
	switch (data.type) {
		case 'voice_offer':
			handleOffer(ws, data);
			break;
		case 'voice_ice_candidate':
			handleIceCandidate(data);
			break;
		case 'voice_state':
			handleState(data);
			break;
		case 'voice_state_all':
			handleStateAll(data);
			break;
		case 'voice_speaking':
			handleSpeaking(data);
			break;
		case 'screen_share_started':
			handleScreenShareStarted(data);
			break;
		case 'screen_share_stopped':
			handleScreenShareStopped(data);
			break;
	}
}

// Client-side speaking detection using AudioContext analyser on the local mic.
function startSpeakingDetection(ws: WebSocket, stream: MediaStream) {
	stopSpeakingDetection();
	audioContext = new AudioContext();
	const source = audioContext.createMediaStreamSource(stream);
	analyser = audioContext.createAnalyser();
	analyser.fftSize = 512;
	analyser.smoothingTimeConstant = 0.4;
	source.connect(analyser);

	const dataArray = new Uint8Array(analyser.frequencyBinCount);
	let wasSpeaking = false;
	let silenceFrames = 0;
	const SPEAK_THRESHOLD = 15; // RMS amplitude threshold
	const SILENCE_FRAMES_NEEDED = 6; // ~300ms at 50ms interval

	speakingInterval = setInterval(() => {
		if (!analyser || muted) {
			if (wasSpeaking) {
				wasSpeaking = false;
				selfSpeaking = false;
				notify();
				wsSend(ws, { type: 'voice_speaking', channel_id: currentChannelId, speaking: false });
			}
			return;
		}
		analyser.getByteTimeDomainData(dataArray);
		// Calculate RMS
		let sum = 0;
		for (let i = 0; i < dataArray.length; i++) {
			const v = (dataArray[i] - 128) / 128;
			sum += v * v;
		}
		const rms = Math.sqrt(sum / dataArray.length) * 100;

		if (rms > SPEAK_THRESHOLD) {
			silenceFrames = 0;
			if (!wasSpeaking) {
				wasSpeaking = true;
				selfSpeaking = true;
				notify();
				wsSend(ws, { type: 'voice_speaking', channel_id: currentChannelId, speaking: true });
			}
		} else {
			silenceFrames++;
			if (wasSpeaking && silenceFrames >= SILENCE_FRAMES_NEEDED) {
				wasSpeaking = false;
				selfSpeaking = false;
				notify();
				wsSend(ws, { type: 'voice_speaking', channel_id: currentChannelId, speaking: false });
			}
		}
	}, 50);
}

async function fetchTurnCredentials(): Promise<RTCIceServer | null> {
	try {
		const res = await fetch('/api/turn/credentials');
		if (!res.ok) return null;
		const data = await res.json();
		return { urls: data.urls, username: data.username, credential: data.credential };
	} catch {
		return null;
	}
}

function stopSpeakingDetection() {
	if (speakingInterval) {
		clearInterval(speakingInterval);
		speakingInterval = null;
	}
	if (audioContext) {
		audioContext.close().catch(() => {});
		audioContext = null;
		analyser = null;
	}
}

async function handleOffer(ws: WebSocket, data: Record<string, unknown>) {
	try {
		const isRenegotiation = pc !== null && pc.connectionState !== 'closed';
		console.log('[voice] handleOffer:', isRenegotiation ? 'renegotiation' : 'initial', 'pc state:', pc?.connectionState ?? 'null');

		if (isRenegotiation) {
			// Renegotiation: reuse existing PC — just update SDP and answer
			await pc!.setRemoteDescription(new RTCSessionDescription(data.sdp as RTCSessionDescriptionInit));

			// Reattach local track to any new empty sendonly transceiver (added for new participants)
			if (localStream) {
				const audioTrack = localStream.getAudioTracks()[0];
				if (audioTrack) {
					const emptySender = pc!.getTransceivers().find(
						t => t.direction === 'sendonly' && t.sender.track === null
					);
					if (emptySender) {
						await emptySender.sender.replaceTrack(audioTrack);
					}
				}
			}

			const answer = await pc!.createAnswer();
			await pc!.setLocalDescription(answer);

			wsSend(ws, {
				type: 'voice_answer',
				channel_id: currentChannelId,
				sdp: answer
			});
			console.log('[voice] renegotiation answer sent');
			return;
		}

		// Initial offer: create new PeerConnection
		stopSpeakingDetection();
		if (pc) {
			pc.close();
		}
		// Clean up any existing remote audio elements
		document.querySelectorAll('audio[data-voice-remote]').forEach(el => el.remove());

		const iceServers: RTCIceServer[] = [{ urls: 'stun:stun.l.google.com:19302' }];
		const turnCreds = await fetchTurnCredentials();
		if (turnCreds) iceServers.push(turnCreds);

		pc = new RTCPeerConnection({ iceServers });

		pc.onicecandidate = (event) => {
			if (event.candidate && currentChannelId) {
				wsSend(ws, {
					type: 'voice_ice_candidate',
					channel_id: currentChannelId,
					candidate: event.candidate.toJSON()
				});
			}
		};

		pc.oniceconnectionstatechange = () => {
			console.log('[voice] ICE connection state:', pc?.iceConnectionState);
		};

		pc.ontrack = (event) => {
			const stream = event.streams[0] ?? new MediaStream([event.track]);
			const trackId = event.track.id;
			const streamId = stream.id;
			console.log('[voice] ontrack: track kind=%s, stream id=%s, track id=%s', event.track.kind, streamId, trackId);

			// Camera tracks have stream ID starting with "camera-"
			if (streamId.startsWith('camera-')) {
				if (event.track.kind === 'video') {
					// Extract user ID from stream ID: "camera-{userId}"
					const cameraUserId = streamId.slice(7); // "camera-".length = 7
					let cameraStream = cameraParticipants.get(cameraUserId);
					if (!cameraStream) {
						cameraStream = new MediaStream();
						cameraParticipants.set(cameraUserId, cameraStream);
					}
					cameraStream.addTrack(event.track);
					event.track.addEventListener('ended', () => {
						const s = cameraParticipants.get(cameraUserId);
						if (s) {
							s.removeTrack(event.track);
							if (s.getTracks().length === 0) {
								cameraParticipants.delete(cameraUserId);
							}
						}
						notifyCamera();
					});
					notifyCamera();
				}
				return;
			}

			// Screen share tracks have stream ID starting with "screen-"
			if (streamId.startsWith('screen-')) {
				if (event.track.kind === 'video') {
					// Create or update the remote screen stream
					if (!remoteScreenStream) {
						remoteScreenStream = new MediaStream();
					}
					remoteScreenStream.addTrack(event.track);
					event.track.addEventListener('ended', () => {
						if (remoteScreenStream) {
							remoteScreenStream.removeTrack(event.track);
							if (remoteScreenStream.getTracks().length === 0) {
								remoteScreenStream = null;
							}
						}
						notifyScreenShare();
					});
					notifyScreenShare();
				} else if (event.track.kind === 'audio') {
					// Screen share system audio — play it directly
					if (remoteScreenStream) {
						remoteScreenStream.addTrack(event.track);
					}
					const screenAudio = document.createElement('audio');
					screenAudio.setAttribute('data-screen-audio', 'true');
					screenAudio.autoplay = true;
					screenAudio.srcObject = new MediaStream([event.track]);
					document.body.appendChild(screenAudio);
					screenAudio.play().catch(() => {});
					event.track.addEventListener('ended', () => {
						screenAudio.remove();
					});
				}
				return;
			}

			// Regular voice audio track
			// Deduplicate: if we already have an audio element for this stream, skip
			const existing = document.querySelector(`audio[data-voice-stream="${streamId}"]`);
			if (existing) return;

			const audio = document.createElement('audio');
			audio.setAttribute('data-voice-remote', 'true');
			audio.setAttribute('data-voice-stream', streamId);
			audio.autoplay = true;
			audio.srcObject = stream;
			if (deafened) audio.muted = true;
			document.body.appendChild(audio);
			audio.play().catch(() => {});
		};

		// Set remote description first so transceivers are created from the offer
		await pc.setRemoteDescription(new RTCSessionDescription(data.sdp as RTCSessionDescriptionInit));

		// Get local audio
		localStream = await navigator.mediaDevices.getUserMedia({
			audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }
		});
		const audioTrack = localStream.getAudioTracks()[0];
		if (audioTrack) {
			audioTrack.enabled = !muted;
			// Find the transceiver for sending our mic audio.
			// After setRemoteDescription with a recvonly offer, the local direction is sendonly.
			const sendTransceiver = pc.getTransceivers().find(
				t => t.direction === 'sendonly' && t.sender.track === null
			);
			if (sendTransceiver) {
				await sendTransceiver.sender.replaceTrack(audioTrack);
			} else {
				pc.addTrack(audioTrack, localStream);
			}
		}

		const answer = await pc.createAnswer();
		await pc.setLocalDescription(answer);

		wsSend(ws, {
			type: 'voice_answer',
			channel_id: currentChannelId,
			sdp: answer
		});

		// Start speaking detection on local mic
		startSpeakingDetection(ws, localStream);
		console.log('[voice] initial connection established, answer sent');
	} catch (e) {
		const msg = e instanceof Error ? e.message : String(e);
		console.error('Voice offer handling failed:', e);
		setError('Voice connection failed: ' + msg);
	}
}

function handleIceCandidate(data: Record<string, unknown>) {
	if (pc && data.candidate) {
		pc.addIceCandidate(new RTCIceCandidate(data.candidate as RTCIceCandidateInit)).catch(e => {
			console.error('Failed to add ICE candidate:', e);
		});
	}
}

function handleState(data: Record<string, unknown>) {
	const channelId = data.channel_id as string;
	const list = data.participants as VoiceParticipant[] | undefined;
	if (channelId && list) {
		participants.set(channelId, list);
	} else if (channelId) {
		participants.delete(channelId);
	}
	notify();
}

function handleStateAll(data: Record<string, unknown>) {
	const channels = data.channels as Record<string, VoiceParticipant[]> | undefined;
	if (!channels) return;
	participants = new Map(Object.entries(channels));
	notify();
}

// Mic test: captures mic, analyses RMS level, loops audio back so user hears themselves.
// Returns a cleanup function that stops everything.
export async function startMicTest(onLevel: (level: number) => void): Promise<() => void> {
	if (!navigator.mediaDevices) {
		throw new Error('Mic not available — requires HTTPS or localhost');
	}
	console.log('[voice] starting mic test...');
	const stream = await navigator.mediaDevices.getUserMedia({
		audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }
	});
	console.log('[voice] mic test: got audio stream, tracks:', stream.getAudioTracks().length);
	const ctx = new AudioContext();
	const source = ctx.createMediaStreamSource(stream);
	const analyserNode = ctx.createAnalyser();
	analyserNode.fftSize = 512;
	analyserNode.smoothingTimeConstant = 0.4;
	source.connect(analyserNode);
	// Loopback so user hears themselves
	source.connect(ctx.destination);

	const dataArray = new Uint8Array(analyserNode.frequencyBinCount);
	const interval = setInterval(() => {
		analyserNode.getByteTimeDomainData(dataArray);
		let sum = 0;
		for (let i = 0; i < dataArray.length; i++) {
			const v = (dataArray[i] - 128) / 128;
			sum += v * v;
		}
		const rms = Math.sqrt(sum / dataArray.length) * 100;
		onLevel(Math.min(100, rms));
	}, 50);

	return () => {
		clearInterval(interval);
		stream.getTracks().forEach(t => t.stop());
		ctx.close().catch(() => {});
	};
}

function handleScreenShareStarted(data: Record<string, unknown>) {
	screenSharerUserId = data.user_id as string;
	screenSharerUsername = data.username as string;
	screenShareChannelId = data.channel_id as string;
	notifyScreenShare();
	notify();
}

function handleScreenShareStopped(_data: Record<string, unknown>) {
	screenSharerUserId = null;
	screenSharerUsername = null;
	screenShareChannelId = null;
	remoteScreenStream = null;
	// Clean up screen audio elements
	document.querySelectorAll('audio[data-screen-audio]').forEach(el => el.remove());
	notifyScreenShare();
	notify();
}

function handleSpeaking(data: Record<string, unknown>) {
	const channelId = data.channel_id as string;
	const userId = data.user_id as string;
	const speaking = data.speaking as boolean;
	const list = participants.get(channelId);
	if (list) {
		const p = list.find(u => u.user_id === userId);
		if (p) {
			p.speaking = speaking;
			notify();
		}
	}
}
