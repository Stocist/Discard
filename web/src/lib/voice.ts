// Voice channel state management.
// Uses callback pattern (same as presence in ws.ts) since $state() doesn't work in .ts files.

export interface VoiceParticipant {
	user_id: string;
	username?: string;
	avatar_path?: string | null;
	muted: boolean;
	deafened: boolean;
	speaking: boolean;
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
		ws.send(JSON.stringify(data));
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
	// Clean up remote audio elements
	document.querySelectorAll('audio[data-voice-remote]').forEach(el => el.remove());
	currentChannelId = null;
	muted = false;
	deafened = false;
	notify();
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

// Called from ws.ts for all voice_* messages
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

		pc = new RTCPeerConnection({
			iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
		});

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
			console.log('[voice] ontrack: new remote track, stream id:', stream.id);
			// Deduplicate: if we already have an audio element for this stream, skip
			const existing = document.querySelector(`audio[data-voice-stream="${stream.id}"]`);
			if (existing) return;

			const audio = document.createElement('audio');
			audio.setAttribute('data-voice-remote', 'true');
			audio.setAttribute('data-voice-stream', stream.id);
			audio.autoplay = true;
			audio.srcObject = stream;
			if (deafened) audio.muted = true;
			document.body.appendChild(audio);
			// Explicitly play to handle autoplay policy; ignore errors (user gesture propagation)
			audio.play().catch(() => {});
		};

		// Set remote description first so transceivers are created from the offer
		await pc.setRemoteDescription(new RTCSessionDescription(data.sdp as RTCSessionDescriptionInit));

		// Get local audio
		localStream = await navigator.mediaDevices.getUserMedia({ audio: true });
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
	const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
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
