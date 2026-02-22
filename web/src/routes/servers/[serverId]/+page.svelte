<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { listChannels } from '$lib/api';

	let loading = $state(true);
	let error = $state('');

	$effect(() => {
		const sid = page.params.serverId;
		if (!sid) return;

		loading = true;
		error = '';

		(async () => {
			try {
				const channels = await listChannels(sid);
				if (channels.length > 0) {
					goto(`/servers/${sid}/channels/${channels[0].id}`, { replaceState: true });
					return;
				}
				error = 'No channels found in this server.';
			} catch (e) {
				console.error('Failed to load channels:', e);
				error = 'Failed to load channels.';
			} finally {
				loading = false;
			}
		})();
	});
</script>

<div class="placeholder">
	{#if loading}
		<p>Loading channels...</p>
	{:else if error}
		<p>{error}</p>
	{/if}
</div>

<style>
	.placeholder {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-muted);
	}
</style>
