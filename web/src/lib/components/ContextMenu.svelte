<script lang="ts">
	interface MenuItem {
		label: string;
		action: () => void;
		danger?: boolean;
	}

	let { x, y, items, onClose }: {
		x: number;
		y: number;
		items: MenuItem[];
		onClose: () => void;
	} = $props();

	let menuEl: HTMLDivElement | undefined = $state();

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onClose();
	}

	function handleClickOutside(e: MouseEvent) {
		if (menuEl && !menuEl.contains(e.target as Node)) {
			onClose();
		}
	}

	$effect(() => {
		document.addEventListener('mousedown', handleClickOutside);
		document.addEventListener('keydown', handleKeydown);
		return () => {
			document.removeEventListener('mousedown', handleClickOutside);
			document.removeEventListener('keydown', handleKeydown);
		};
	});

	// Adjust position so menu doesn't go off-screen
	let adjustedX = $derived.by(() => {
		const menuWidth = 180;
		if (x + menuWidth > window.innerWidth) return window.innerWidth - menuWidth - 8;
		return x;
	});

	let adjustedY = $derived.by(() => {
		const menuHeight = items.length * 36 + 8;
		if (y + menuHeight > window.innerHeight) return window.innerHeight - menuHeight - 8;
		return y;
	});
</script>

<div class="context-menu" bind:this={menuEl} style="left: {adjustedX}px; top: {adjustedY}px;">
	{#each items as item}
		<button
			class="context-item"
			class:danger={item.danger}
			onclick={() => { item.action(); onClose(); }}
		>
			{item.label}
		</button>
	{/each}
</div>

<style>
	.context-menu {
		position: fixed;
		z-index: 1000;
		min-width: 160px;
		background: var(--bg-secondary);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 4px;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
	}

	.context-item {
		display: block;
		width: 100%;
		text-align: left;
		padding: 8px 12px;
		font-size: 13px;
		color: var(--text-primary);
		border-radius: 4px;
		cursor: pointer;
	}

	.context-item:hover {
		background: var(--bg-hover);
	}

	.context-item.danger {
		color: #ef4444;
	}

	.context-item.danger:hover {
		background: rgba(239, 68, 68, 0.15);
	}
</style>
