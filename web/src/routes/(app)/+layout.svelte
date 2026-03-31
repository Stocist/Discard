<script lang="ts">
	import { onMount, setContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import ServerSidebar from '$lib/components/ServerSidebar.svelte';
	import InstallPrompt from '$lib/components/InstallPrompt.svelte';
	import { fetchMe } from '$lib/api';

	let { children } = $props();

	let sidebarOpen = $state(false);
	let authenticated = $state(false);

	// Share sidebar state with child pages via context
	const sidebarState = {
		get open() { return sidebarOpen; },
		close() { sidebarOpen = false; }
	};
	setContext('mobileSidebar', sidebarState);

	// Close sidebar on navigation (e.g. when user taps a channel on mobile)
	let lastPath = $state('');
	$effect(() => {
		const currentPath = page.url.pathname;
		if (lastPath && currentPath !== lastPath) {
			sidebarOpen = false;
		}
		lastPath = currentPath;
	});

	onMount(async () => {
		try {
			await fetchMe();
			authenticated = true;
		} catch {
			goto('/login?reason=unauthenticated', { replaceState: true });
		}
	});
</script>

{#if authenticated}
	<div class="app-shell">
		{#if !sidebarOpen}
			<button class="hamburger" onclick={() => (sidebarOpen = true)} aria-label="Open sidebar">
				<span class="hamburger-icon">&#x2630;</span>
			</button>
		{/if}

		<div class="sidebar-wrapper" class:open={sidebarOpen}>
			<ServerSidebar />
			{#if sidebarOpen}
				<button class="sidebar-close" onclick={() => (sidebarOpen = false)} aria-label="Close sidebar">
					<span class="hamburger-icon">&#x2715;</span>
				</button>
			{/if}
		</div>

		{#if sidebarOpen}
			<!-- svelte-ignore a11y_no_static_element_interactions -->
			<div class="sidebar-backdrop" onclick={() => (sidebarOpen = false)} onkeydown={() => {}}></div>
		{/if}

		<div class="main-content">
			{@render children()}
		</div>

		<InstallPrompt />
	</div>
{:else}
	<div class="loading">
		<p>Loading...</p>
	</div>
{/if}

<style>
	.loading {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100vh;
		color: var(--text-muted);
	}

	.app-shell {
		display: flex;
		height: 100vh;
		width: 100vw;
		overflow: hidden;
		position: relative;
	}

	.main-content {
		flex: 1;
		display: flex;
		min-width: 0;
		overflow: hidden;
	}

	.hamburger {
		display: none;
		position: fixed;
		top: 8px;
		left: 8px;
		z-index: 60;
		width: 36px;
		height: 36px;
		border-radius: 6px;
		background: var(--bg-secondary);
		align-items: center;
		justify-content: center;
		border: 1px solid var(--border);
	}

	.hamburger-icon {
		font-size: 18px;
		line-height: 1;
	}

	.sidebar-close {
		display: none;
	}

	.sidebar-wrapper {
		display: contents;
	}

	.sidebar-backdrop {
		display: none;
	}

	@media (max-width: 768px) {
		.hamburger {
			display: flex;
		}

		.sidebar-wrapper {
			display: flex;
			position: fixed;
			top: 0;
			left: 0;
			bottom: 0;
			z-index: 50;
			transform: translateX(-100%);
			transition: transform 0.2s ease;
		}

		.sidebar-wrapper.open {
			transform: translateX(0);
		}

		.sidebar-close {
			display: flex;
			position: absolute;
			top: 8px;
			right: -44px;
			z-index: 55;
			width: 36px;
			height: 36px;
			border-radius: 6px;
			background: var(--bg-secondary);
			align-items: center;
			justify-content: center;
			border: 1px solid var(--border);
		}

		.sidebar-backdrop {
			display: block;
			position: fixed;
			inset: 0;
			background: rgba(0, 0, 0, 0.5);
			z-index: 40;
		}
	}

	@media (min-width: 769px) and (max-width: 1024px) {
		.hamburger {
			display: none;
		}
	}
</style>
