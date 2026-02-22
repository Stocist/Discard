<script lang="ts">
	import favicon from '$lib/assets/favicon.svg';
	import '../app.css';
	import ServerSidebar from '$lib/components/ServerSidebar.svelte';

	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Discard</title>
</svelte:head>

<div class="app-shell">
	<ServerSidebar />
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
	}

	.main-content {
		flex: 1;
		display: flex;
		min-width: 0;
		overflow: hidden;
	}
</style>
