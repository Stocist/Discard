import { expect, test, type Browser, type BrowserContext, type Page } from '@playwright/test';

type VoiceStats = {
	connectionState: string;
	iceConnectionState: string;
	signalingState: string;
	outboundBytes: number;
	inboundBytes: number;
	localCandidateType: string | null;
	localCandidateTypes: string[];
	iceErrors: unknown[];
	allConnectionStates: string[];
	configuration: RTCConfiguration | null;
};

for (const relayOnly of [false, true]) {
	test(`two browsers exchange audio ${relayOnly ? 'through TURN' : 'directly'}`, async ({ browser }) => {
		const first = await voiceContext(browser, 0, relayOnly);
		const firstPage = await first.newPage();
		await firstPage.goto('/api/me?dev_user=0');

		const serverResponse = await firstPage.request.post('/api/servers', {
			data: { name: `Voice E2E ${relayOnly ? 'relay' : 'direct'} ${Date.now()}` }
		});
		expect(serverResponse.ok()).toBeTruthy();
		const server = await serverResponse.json();
		const voiceResponse = await firstPage.request.post(`/api/servers/${server.id}/channels`, {
			data: { name: 'Voice Test', type: 'voice' }
		});
		expect(voiceResponse.ok()).toBeTruthy();

		const second = await voiceContext(browser, 1, relayOnly);
		const secondPage = await second.newPage();
		await secondPage.goto('/api/me?dev_user=1');

		await firstPage.goto(`/servers/${server.id}`);
		await joinVoice(firstPage);
		await expect.poll(() => voiceStats(firstPage), { timeout: 20_000 }).toMatchObject({ connectionState: 'connected' });
		await expect.poll(() => voiceStats(firstPage).then((stats) => stats.outboundBytes)).toBeGreaterThan(0);
		const firstBytesBeforeRenegotiation = (await voiceStats(firstPage)).outboundBytes;

		await secondPage.goto(`/servers/${server.id}`);
		await joinVoice(secondPage);
		await expect.poll(() => voiceStats(secondPage), { timeout: 20_000 }).toMatchObject({ connectionState: 'connected' });

		await expect.poll(() => voiceStats(firstPage).then((stats) => stats.inboundBytes), { timeout: 20_000 }).toBeGreaterThan(0);
		await expect.poll(() => voiceStats(secondPage).then((stats) => stats.inboundBytes), { timeout: 20_000 }).toBeGreaterThan(0);
		await expect.poll(() => voiceStats(firstPage).then((stats) => stats.outboundBytes)).toBeGreaterThan(firstBytesBeforeRenegotiation);

		if (relayOnly) {
			await expect.poll(() => voiceStats(firstPage).then((stats) => stats.localCandidateTypes)).toContain('relay');
			await expect.poll(() => voiceStats(secondPage).then((stats) => stats.localCandidateTypes)).toContain('relay');
		}

		await first.close();
		await second.close();
	});
}

async function voiceContext(browser: Browser, devUser: number, relayOnly: boolean): Promise<BrowserContext> {
	const context = await browser.newContext({
		ignoreHTTPSErrors: true,
		permissions: ['microphone']
	});
	await context.addInitScript(({ forceRelay, user }) => {
		const NativePeerConnection = window.RTCPeerConnection;
		class InstrumentedPeerConnection extends NativePeerConnection {
			constructor(configuration: RTCConfiguration = {}) {
				super({ ...configuration, ...(forceRelay ? { iceTransportPolicy: 'relay' as RTCIceTransportPolicy } : {}) });
				(window as unknown as { __discardPCs: RTCPeerConnection[] }).__discardPCs ??= [];
				(window as unknown as { __discardPCs: RTCPeerConnection[] }).__discardPCs.push(this);
				this.addEventListener('icecandidateerror', (event) => {
					const target = window as unknown as { __discardIceErrors?: unknown[] };
					target.__discardIceErrors ??= [];
					target.__discardIceErrors.push({ url: event.url, errorCode: event.errorCode, errorText: event.errorText });
				});
			}
		}
		Object.defineProperty(window, 'RTCPeerConnection', { value: InstrumentedPeerConnection });
		localStorage.setItem('discard-e2e-user', String(user));
	}, { forceRelay: relayOnly, user: devUser });
	return context;
}

async function joinVoice(page: Page) {
	const voiceButton = page.locator('button.voice-channel', { hasText: 'Voice Test' });
	await expect(voiceButton).toBeVisible();
	await voiceButton.click();
}

async function voiceStats(page: Page): Promise<VoiceStats> {
	return page.evaluate(async () => {
		const pcs = (window as unknown as { __discardPCs?: RTCPeerConnection[] }).__discardPCs ?? [];
		const pc = pcs.findLast((candidate) => candidate.signalingState !== 'closed') ?? pcs.at(-1);
		if (!pc) return {
			connectionState: 'missing', iceConnectionState: 'missing', signalingState: 'missing',
			outboundBytes: 0, inboundBytes: 0, localCandidateType: null, localCandidateTypes: [], iceErrors: [], allConnectionStates: [], configuration: null
		};
		const report = await pc.getStats();
		let outboundBytes = 0;
		let inboundBytes = 0;
		let localCandidateType: string | null = null;
		const localCandidateTypes: string[] = [];
		report.forEach((stat) => {
			if (stat.type === 'outbound-rtp' && stat.kind === 'audio') outboundBytes += stat.bytesSent ?? 0;
			if (stat.type === 'inbound-rtp' && stat.kind === 'audio') inboundBytes += stat.bytesReceived ?? 0;
			if (stat.type === 'transport' && stat.selectedCandidatePairId) {
				const pair = report.get(stat.selectedCandidatePairId);
				const local = pair?.localCandidateId ? report.get(pair.localCandidateId) : null;
				localCandidateType = local?.candidateType ?? null;
			}
			if (stat.type === 'local-candidate' && stat.candidateType) localCandidateTypes.push(stat.candidateType);
		});
		return {
			connectionState: pc.connectionState,
			iceConnectionState: pc.iceConnectionState,
			signalingState: pc.signalingState,
			outboundBytes,
			inboundBytes,
			localCandidateType,
			localCandidateTypes,
			iceErrors: (window as unknown as { __discardIceErrors?: unknown[] }).__discardIceErrors ?? [],
			allConnectionStates: pcs.map((candidate) => `${candidate.connectionState}/${candidate.iceConnectionState}/${candidate.signalingState}`),
			configuration: pc.getConfiguration()
		};
	});
}
