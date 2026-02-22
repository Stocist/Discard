<script lang="ts">
	import favicon from '$lib/assets/favicon.svg';
	import '../app.css';
	import ServerSidebar from '$lib/components/ServerSidebar.svelte';

	let { children } = $props();

	let sidebarOpen = $state(false);
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Discard</title>
</svelte:head>

<div class="app-shell">
	<button class="hamburger" onclick={() => (sidebarOpen = !sidebarOpen)} aria-label="Toggle sidebar">
		<span class="hamburger-icon">{sidebarOpen ? '\u2715' : '\u2630'}</span>
	</button>

	<div class="sidebar-wrapper" class:open={sidebarOpen}>
		<ServerSidebar />
	</div>

	{#if sidebarOpen}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="sidebar-backdrop" onclick={() => (sidebarOpen = false)} onkeydown={() => {}}></div>
	{/if}

	<div class="main-content">
		{@render children()}
	</div>
</div>

<style>
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
