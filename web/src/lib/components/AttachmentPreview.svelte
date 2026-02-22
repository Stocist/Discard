<script lang="ts">
	import type { Attachment } from '$lib/types';
	import ImageLightbox from './ImageLightbox.svelte';

	let { attachments }: {
		attachments: Attachment[];
	} = $props();

	let lightboxAttachment = $state<Attachment | null>(null);

	function isImage(att: Attachment): boolean {
		return att.mime_type?.startsWith('image/') ?? false;
	}

	function formatSize(bytes: number | null): string {
		if (bytes == null) return '';
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function uploadUrl(filePath: string): string {
		return `/uploads/${filePath}`;
	}
</script>

{#if attachments.length > 0}
	<div class="attachments">
		{#each attachments as att (att.id)}
			{#if isImage(att)}
				<button class="image-attachment" onclick={() => (lightboxAttachment = att)}>
					<img
						src={uploadUrl(att.file_path)}
						alt={att.original_name}
						loading="lazy"
					/>
				</button>
			{:else}
				<a class="file-attachment" href={uploadUrl(att.file_path)} download={att.original_name}>
					<span class="file-icon">📎</span>
					<span class="file-info">
						<span class="file-name">{att.original_name}</span>
						<span class="file-size">{formatSize(att.file_size)}</span>
					</span>
				</a>
			{/if}
		{/each}
	</div>

	{#if lightboxAttachment}
		<ImageLightbox attachment={lightboxAttachment} onclose={() => (lightboxAttachment = null)} />
	{/if}
{/if}

<style>
	.attachments {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		margin-top: 4px;
	}

	.image-attachment {
		display: block;
		max-width: 400px;
		border-radius: 8px;
		overflow: hidden;
		padding: 0;
		background: none;
		border: none;
		cursor: pointer;
	}

	.image-attachment img {
		display: block;
		max-width: 100%;
		height: auto;
		border-radius: 8px;
	}

	.image-attachment:hover img {
		opacity: 0.9;
	}

	.file-attachment {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 12px;
		background: var(--bg-secondary);
		border-radius: 8px;
		color: var(--text-primary);
		text-decoration: none;
		max-width: 300px;
	}

	.file-attachment:hover {
		background: var(--bg-hover);
	}

	.file-icon {
		font-size: 20px;
		flex-shrink: 0;
	}

	.file-info {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.file-name {
		font-size: 13px;
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.file-size {
		font-size: 11px;
		color: var(--text-muted);
	}
</style>
