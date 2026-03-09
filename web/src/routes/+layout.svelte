<script lang="ts">
	import favicon from '$lib/assets/favicon.svg';
	import '../app.css';

	let { children } = $props();

	// Dev mode: persist ?dev_user=N as a cookie so API/WS requests use the right identity
	if (typeof window !== 'undefined') {
		const devUser = new URLSearchParams(window.location.search).get('dev_user');
		if (devUser !== null) {
			document.cookie = `dev_user=${devUser}; path=/; max-age=86400`;
		}
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Discard</title>
</svelte:head>

{@render children()}
