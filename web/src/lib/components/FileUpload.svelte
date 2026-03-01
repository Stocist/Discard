<script lang="ts">
	let { files = $bindable(), onRemove }: {
		files: File[];
		onRemove: (index: number) => void;
	} = $props();

	let previews = $state<string[]>([]);
	let prevUrls: string[] = [];
	let lightboxSrc = $state<string | null>(null);

	$effect(() => {
		// Track only `files` as dependency — avoid reading `previews` here.
		const _files = files;
		// Revoke old object URLs.
		for (const url of prevUrls) {
			if (url) URL.revokeObjectURL(url);
		}
		// Generate new previews for image files.
		const urls = _files.map((f) =>
			f.type.startsWith('image/') ? URL.createObjectURL(f) : ''
		);
		prevUrls = urls;
		previews = urls;
	});

	function formatSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}
</script>

{#if lightboxSrc}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="lightbox-overlay" onclick={() => lightboxSrc = null}>
		<img src={lightboxSrc} alt="Preview" class="lightbox-img" />
	</div>
{/if}

<svelte:window onkeydown={(e) => { if (e.key === 'Escape') lightboxSrc = null; }} />

{#if files.length > 0}
	<div class="file-preview-strip">
		{#each files as file, i (file.name + file.size + i)}
			<div class="preview-item">
				<button class="remove-btn" onclick={() => onRemove(i)} aria-label="Remove file">x</button>
				{#if previews[i]}
					<button class="thumb-btn" onclick={() => lightboxSrc = previews[i]} aria-label="Preview {file.name}">
						<img class="preview-thumb" src={previews[i]} alt={file.name} />
					</button>
				{:else}
					<div class="preview-file-icon">📎</div>
				{/if}
				<div class="preview-name" title={file.name}>{file.name}</div>
				<div class="preview-size">{formatSize(file.size)}</div>
			</div>
		{/each}
	</div>
{/if}

<style>
	.file-preview-strip {
		display: flex;
		gap: 8px;
		padding: 8px 0;
		overflow-x: auto;
	}

	.preview-item {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		width: 80px;
		flex-shrink: 0;
	}

	.remove-btn {
		position: absolute;
		top: -4px;
		right: -4px;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--danger, #e74c3c);
		color: white;
		font-size: 12px;
		line-height: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
		border: none;
		padding: 0;
		z-index: 1;
	}

	.remove-btn:hover {
		opacity: 0.8;
	}

	.preview-thumb {
		width: 64px;
		height: 64px;
		object-fit: cover;
		border-radius: 6px;
	}

	.preview-file-icon {
		width: 64px;
		height: 64px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg-secondary);
		border-radius: 6px;
		font-size: 24px;
	}

	.preview-name {
		font-size: 11px;
		color: var(--text-primary);
		max-width: 80px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		margin-top: 4px;
	}

	.preview-size {
		font-size: 10px;
		color: var(--text-muted);
	}

	.thumb-btn {
		background: none;
		border: none;
		padding: 0;
		cursor: pointer;
		border-radius: 6px;
	}

	.thumb-btn:hover .preview-thumb {
		opacity: 0.8;
	}

	.lightbox-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.85);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
		cursor: pointer;
	}

	.lightbox-img {
		max-width: 90vw;
		max-height: 90vh;
		object-fit: contain;
		border-radius: 8px;
	}
</style>
