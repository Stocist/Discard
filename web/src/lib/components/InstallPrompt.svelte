<script lang="ts">
	import { onMount } from 'svelte';

	let deferredPrompt: any = null;
	let visible = $state(false);

	onMount(() => {
		if (localStorage.getItem('discard-install-dismissed')) return;

		const handleBeforeInstall = (e: Event) => {
			e.preventDefault();
			deferredPrompt = e;
			visible = true;
		};

		const handleInstalled = () => {
			visible = false;
			deferredPrompt = null;
		};

		window.addEventListener('beforeinstallprompt', handleBeforeInstall);
		window.addEventListener('appinstalled', handleInstalled);

		return () => {
			window.removeEventListener('beforeinstallprompt', handleBeforeInstall);
			window.removeEventListener('appinstalled', handleInstalled);
		};
	});

	function install() {
		if (!deferredPrompt) return;
		deferredPrompt.prompt();
		deferredPrompt.userChoice.then(() => {
			deferredPrompt = null;
			visible = false;
		});
	}

	function dismiss() {
		visible = false;
		localStorage.setItem('discard-install-dismissed', '1');
	}
</script>

{#if visible}
	<div class="install-banner">
		<span class="install-text">Install Discard for a better experience</span>
		<button class="install-btn" onclick={install}>Install</button>
		<button class="dismiss-btn" onclick={dismiss} aria-label="Dismiss">&times;</button>
	</div>
{/if}

<style>
	.install-banner {
		position: fixed;
		bottom: 16px;
		left: 50%;
		transform: translateX(-50%);
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 10px 16px;
		background: var(--bg-secondary);
		border: 1px solid var(--border);
		border-radius: 8px;
		z-index: 1000;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
	}

	.install-text {
		font-size: 13px;
		color: var(--text-primary);
		white-space: nowrap;
	}

	.install-btn {
		font-size: 12px;
		padding: 4px 12px;
		border-radius: 4px;
		background: var(--accent);
		color: var(--bg-primary);
		font-weight: 600;
		transition: background 0.15s;
	}

	.install-btn:hover {
		background: var(--accent-hover);
	}

	.dismiss-btn {
		font-size: 18px;
		line-height: 1;
		color: var(--text-muted);
		padding: 0 2px;
		transition: color 0.15s;
	}

	.dismiss-btn:hover {
		color: var(--text-primary);
	}
</style>
