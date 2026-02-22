<script lang="ts">
	import type { Attachment } from '$lib/types';

	let { attachment, onclose }: {
		attachment: Attachment;
		onclose: () => void;
	} = $props();

	function uploadUrl(filePath: string): string {
		return `/uploads/${filePath}`;
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="lightbox-overlay" onclick={onclose} role="dialog" aria-modal="true" aria-label="Image preview">
	<button class="lightbox-close" onclick={onclose} aria-label="Close">X</button>
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="lightbox-content" onclick={(e) => e.stopPropagation()}>
		<img
			src={uploadUrl(attachment.file_path)}
			alt={attachment.original_name}
		/>
		<div class="lightbox-footer">
			<span class="lightbox-filename">{attachment.original_name}</span>
			<a
				class="lightbox-open-link"
				href={uploadUrl(attachment.file_path)}
				target="_blank"
				rel="noopener"
			>
				Open original
			</a>
		</div>
	</div>
</div>

<style>
	.lightbox-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.85);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 200;
		animation: lightbox-fade-in 0.15s ease-out;
	}

	@keyframes lightbox-fade-in {
		from { opacity: 0; }
		to { opacity: 1; }
	}

	.lightbox-close {
		position: fixed;
		top: 16px;
		right: 16px;
		width: 36px;
		height: 36px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: rgba(0, 0, 0, 0.5);
		color: var(--text-primary);
		border-radius: 50%;
		font-size: 14px;
		font-weight: 600;
		z-index: 201;
		cursor: pointer;
	}

	.lightbox-close:hover {
		background: rgba(255, 255, 255, 0.15);
	}

	.lightbox-content {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 12px;
		max-width: 90vw;
		max-height: 90vh;
	}

	.lightbox-content img {
		max-width: 90vw;
		max-height: calc(90vh - 40px);
		object-fit: contain;
		border-radius: 4px;
	}

	.lightbox-footer {
		display: flex;
		align-items: center;
		gap: 12px;
		color: var(--text-muted);
		font-size: 13px;
	}

	.lightbox-filename {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 400px;
	}

	.lightbox-open-link {
		color: var(--accent);
		text-decoration: none;
		white-space: nowrap;
	}

	.lightbox-open-link:hover {
		text-decoration: underline;
	}
</style>
