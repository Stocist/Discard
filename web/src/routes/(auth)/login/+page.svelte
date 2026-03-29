<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { fetchTailscaleStatus } from '$lib/api';

	type DetectionState = 'loading' | 'polling' | 'detected' | 'authenticated' | 'error';
	type Platform = 'mac' | 'windows' | 'linux' | 'ios' | 'android' | 'unknown';

	let detection: DetectionState = $state('loading');
	let errorMessage = $state('');
	let pollTimer: ReturnType<typeof setInterval> | null = $state(null);
	let showSetup = $state(false);
	let userPlatform: Platform = $state('unknown');

	const reason = $derived(new URL($page.url).searchParams.get('reason'));

	function detectPlatform(): Platform {
		const ua = navigator.userAgent.toLowerCase();
		if (/iphone|ipad|ipod/.test(ua)) return 'ios';
		if (/android/.test(ua)) return 'android';
		if (/macintosh|mac os/.test(ua)) return 'mac';
		if (/windows/.test(ua)) return 'windows';
		if (/linux/.test(ua)) return 'linux';
		return 'unknown';
	}

	const platformLabels: Record<Platform, string> = {
		mac: 'macOS',
		windows: 'Windows',
		linux: 'Linux',
		ios: 'iOS',
		android: 'Android',
		unknown: 'your device'
	};

	const downloadLinks: Record<Platform, string> = {
		mac: 'https://tailscale.com/download/mac',
		windows: 'https://tailscale.com/download/windows',
		linux: 'https://tailscale.com/download/linux',
		ios: 'https://tailscale.com/download/ios',
		android: 'https://tailscale.com/download/android',
		unknown: 'https://tailscale.com/download'
	};

	async function checkStatus(): Promise<boolean> {
		try {
			const status = await fetchTailscaleStatus();
			if (status.authenticated) {
				detection = 'authenticated';
				stopPolling();
				setTimeout(() => {
					goto('/', { replaceState: true });
				}, 1200);
				return true;
			} else if (status.on_tailscale) {
				detection = 'detected';
				errorMessage = 'You are on the Tailscale network but not authorized for this server. Ask the admin to share this node with you.';
				return false;
			} else {
				if (detection === 'loading') {
					detection = 'polling';
				}
				return false;
			}
		} catch {
			if (detection === 'loading') {
				detection = 'error';
				errorMessage = 'Unable to reach the server. Check your connection and try again.';
			}
			return false;
		}
	}

	function startPolling() {
		stopPolling();
		detection = 'polling';
		pollTimer = setInterval(checkStatus, 4000);
	}

	function stopPolling() {
		if (pollTimer) {
			clearInterval(pollTimer);
			pollTimer = null;
		}
	}

	function retry() {
		errorMessage = '';
		detection = 'loading';
		checkStatus().then(() => {
			if (detection !== 'authenticated') {
				startPolling();
			}
		});
	}

	onMount(() => {
		userPlatform = detectPlatform();

		checkStatus().then(() => {
			if (detection !== 'authenticated') {
				startPolling();
			}
		});

		return () => {
			stopPolling();
		};
	});
</script>

<div class="login-page">
	<div class="login-card">
		<div class="logo-section">
			<h1 class="logo">Discard</h1>
			<p class="tagline">A private space for your friend group</p>
		</div>

		<div class="features">
			<div class="feature">
				<span class="feature-icon">#</span>
				<span>Text channels with markdown, file sharing, and images</span>
			</div>
			<div class="feature">
				<span class="feature-icon">&#x266a;</span>
				<span>Voice channels for hanging out together</span>
			</div>
			<div class="feature">
				<span class="feature-icon">&#x26BF;</span>
				<span>Private and secure on your own Tailscale network</span>
			</div>
		</div>

		{#if reason === 'unauthenticated' && detection === 'polling'}
			<div class="notice">
				Connect to the Tailscale network to continue.
			</div>
		{:else if reason === 'not_invited'}
			<div class="notice">
				You're on Tailscale, but this node hasn't been shared with you yet. Ask the admin for access.
			</div>
		{/if}

		<!-- Detection status -->
		<div class="status-section">
			{#if detection === 'loading'}
				<div class="status-indicator">
					<div class="spinner"></div>
					<span class="status-text">Checking connection...</span>
				</div>
			{:else if detection === 'polling'}
				<div class="status-indicator">
					<div class="pulse-dot"></div>
					<span class="status-text">Waiting for Tailscale connection...</span>
				</div>
				<p class="status-hint">
					Connect to Tailscale and this page will update automatically.
				</p>
			{:else if detection === 'detected'}
				<div class="status-indicator detected">
					<div class="pulse-dot detected"></div>
					<span class="status-text">Tailscale detected, but not authorized</span>
				</div>
				{#if errorMessage}
					<div class="error-message">{errorMessage}</div>
				{/if}
			{:else if detection === 'authenticated'}
				<div class="status-indicator success">
					<div class="check-icon">&#x2713;</div>
					<span class="status-text">Connected! Redirecting...</span>
				</div>
			{:else if detection === 'error'}
				<div class="status-indicator error">
					<div class="error-icon">!</div>
					<span class="status-text">Could not reach server</span>
				</div>
				{#if errorMessage}
					<div class="error-message">{errorMessage}</div>
				{/if}
				<button class="retry-btn" onclick={retry}>
					Retry connection
				</button>
			{/if}
		</div>

		<!-- Guided setup (expandable) -->
		<div class="setup-section">
			<button class="setup-toggle" onclick={() => showSetup = !showSetup}>
				<span class="setup-toggle-label">
					{showSetup ? 'Hide' : 'Need'} setup instructions{showSetup ? '' : '?'}
				</span>
				<span class="setup-toggle-arrow" class:open={showSetup}>&#x25B8;</span>
			</button>

			{#if showSetup}
				<div class="setup-steps">
					<div class="step">
						<div class="step-number">1</div>
						<div class="step-content">
							<p class="step-title">Install Tailscale</p>
							<p class="step-desc">
								Tailscale creates a private network between devices. It's free and takes a couple of minutes.
							</p>
							<a
								class="step-link"
								href={downloadLinks[userPlatform]}
								target="_blank"
								rel="noopener noreferrer"
							>
								Download for {platformLabels[userPlatform]}
							</a>
						</div>
					</div>

					<div class="step">
						<div class="step-number">2</div>
						<div class="step-content">
							<p class="step-title">Sign in to Tailscale</p>
							<p class="step-desc">
								{#if userPlatform === 'mac'}
									Open Tailscale from the menu bar and sign in with your Google, Microsoft, or GitHub account.
								{:else if userPlatform === 'windows'}
									Open Tailscale from the system tray and sign in with your Google, Microsoft, or GitHub account.
								{:else if userPlatform === 'linux'}
									Run <code>sudo tailscale up</code> and follow the link to sign in.
								{:else if userPlatform === 'ios' || userPlatform === 'android'}
									Open the Tailscale app and sign in with your Google, Microsoft, or GitHub account.
								{:else}
									Open Tailscale and sign in with your Google, Microsoft, or GitHub account.
								{/if}
							</p>
						</div>
					</div>

					<div class="step">
						<div class="step-number">3</div>
						<div class="step-content">
							<p class="step-title">Get invited to the network</p>
							<p class="step-desc">
								Ask the server admin to share this node with your Tailscale account. Once they do, this page will detect it automatically.
							</p>
						</div>
					</div>

					<div class="step">
						<div class="step-number">4</div>
						<div class="step-content">
							<p class="step-title">You're in!</p>
							<p class="step-desc">
								Once connected, this page will detect Tailscale and log you in automatically. No passwords needed.
							</p>
						</div>
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.login-page {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 100vh;
		background: var(--bg-primary);
		padding: 24px;
	}

	.login-card {
		width: 100%;
		max-width: 440px;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 28px;
	}

	.logo-section {
		text-align: center;
	}

	.logo {
		font-size: 42px;
		font-weight: 700;
		color: var(--accent);
		letter-spacing: -1px;
	}

	.tagline {
		color: var(--text-muted);
		font-size: 16px;
		margin-top: 6px;
	}

	.features {
		display: flex;
		flex-direction: column;
		gap: 12px;
		width: 100%;
		background: var(--bg-secondary);
		border-radius: 8px;
		padding: 20px;
	}

	.feature {
		display: flex;
		align-items: center;
		gap: 12px;
		font-size: 14px;
		color: var(--text-primary);
	}

	.feature-icon {
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg-tertiary);
		border-radius: 6px;
		font-size: 14px;
		flex-shrink: 0;
	}

	.notice {
		width: 100%;
		padding: 12px 16px;
		background: var(--bg-secondary);
		border-left: 3px solid var(--accent);
		border-radius: 4px;
		color: var(--text-primary);
		font-size: 13px;
	}

	.error-message {
		width: 100%;
		padding: 12px 16px;
		background: rgba(220, 38, 38, 0.1);
		border-left: 3px solid #dc2626;
		border-radius: 4px;
		color: #fca5a5;
		font-size: 13px;
	}

	/* Status section */
	.status-section {
		width: 100%;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 12px;
		padding: 20px;
		background: var(--bg-secondary);
		border-radius: 8px;
		transition: all 0.3s ease;
	}

	.status-indicator {
		display: flex;
		align-items: center;
		gap: 12px;
		transition: all 0.3s ease;
	}

	.status-text {
		font-size: 15px;
		color: var(--text-primary);
		transition: color 0.3s ease;
	}

	.status-hint {
		font-size: 13px;
		color: var(--text-muted);
		text-align: center;
	}

	/* Spinner for loading state */
	.spinner {
		width: 20px;
		height: 20px;
		border: 2px solid var(--bg-tertiary);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to { transform: rotate(360deg); }
	}

	/* Pulsing dot for polling state */
	.pulse-dot {
		width: 12px;
		height: 12px;
		border-radius: 50%;
		background: var(--text-muted);
		animation: pulse 2s ease-in-out infinite;
		flex-shrink: 0;
	}

	.pulse-dot.detected {
		background: #f59e0b;
		animation: pulse-amber 2s ease-in-out infinite;
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.4; transform: scale(1); }
		50% { opacity: 1; transform: scale(1.2); }
	}

	@keyframes pulse-amber {
		0%, 100% { opacity: 0.5; transform: scale(1); box-shadow: 0 0 0 0 rgba(245, 158, 11, 0.3); }
		50% { opacity: 1; transform: scale(1.2); box-shadow: 0 0 0 6px rgba(245, 158, 11, 0); }
	}

	/* Success check icon */
	.check-icon {
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--accent);
		color: #1c1917;
		border-radius: 50%;
		font-size: 16px;
		font-weight: 700;
		animation: pop-in 0.4s cubic-bezier(0.34, 1.56, 0.64, 1);
	}

	@keyframes pop-in {
		0% { transform: scale(0); opacity: 0; }
		100% { transform: scale(1); opacity: 1; }
	}

	.status-indicator.success .status-text {
		color: var(--accent);
		font-weight: 600;
	}

	/* Error icon */
	.error-icon {
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: rgba(220, 38, 38, 0.2);
		color: #fca5a5;
		border-radius: 50%;
		font-size: 16px;
		font-weight: 700;
	}

	.status-indicator.detected .status-text {
		color: #f59e0b;
	}

	.retry-btn {
		padding: 10px 24px;
		background: var(--bg-tertiary);
		color: var(--text-primary);
		font-size: 14px;
		font-weight: 500;
		border-radius: 6px;
		cursor: pointer;
		transition: background 0.15s;
	}

	.retry-btn:hover {
		background: var(--border);
	}

	/* Setup section */
	.setup-section {
		width: 100%;
		background: var(--bg-secondary);
		border-radius: 8px;
		overflow: hidden;
	}

	.setup-toggle {
		width: 100%;
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 16px 20px;
		color: var(--text-muted);
		font-size: 14px;
		cursor: pointer;
		transition: color 0.15s;
	}

	.setup-toggle:hover {
		color: var(--text-primary);
	}

	.setup-toggle-label {
		font-weight: 500;
	}

	.setup-toggle-arrow {
		transition: transform 0.2s ease;
		font-size: 12px;
	}

	.setup-toggle-arrow.open {
		transform: rotate(90deg);
	}

	.setup-steps {
		display: flex;
		flex-direction: column;
		gap: 0;
		padding: 0 20px 20px;
		animation: slide-down 0.2s ease;
	}

	@keyframes slide-down {
		from { opacity: 0; transform: translateY(-8px); }
		to { opacity: 1; transform: translateY(0); }
	}

	.step {
		display: flex;
		gap: 14px;
		padding: 14px 0;
		border-top: 1px solid var(--bg-tertiary);
	}

	.step-number {
		width: 24px;
		height: 24px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg-tertiary);
		color: var(--accent);
		border-radius: 50%;
		font-size: 12px;
		font-weight: 700;
		flex-shrink: 0;
		margin-top: 1px;
	}

	.step-content {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}

	.step-title {
		font-size: 14px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.step-desc {
		font-size: 13px;
		color: var(--text-muted);
		line-height: 1.5;
	}

	.step-desc code {
		background: var(--bg-tertiary);
		padding: 1px 5px;
		border-radius: 3px;
		font-family: 'SF Mono', 'Fira Code', Menlo, monospace;
		font-size: 0.9em;
	}

	.step-link {
		display: inline-block;
		padding: 6px 16px;
		background: var(--bg-tertiary);
		color: var(--accent);
		font-size: 13px;
		font-weight: 500;
		border-radius: 6px;
		margin-top: 6px;
		transition: background 0.15s;
		width: fit-content;
	}

	.step-link:hover {
		background: var(--border);
	}
</style>
