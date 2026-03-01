<script lang="ts">
	import { createMessage } from '$lib/api';
	import FileUpload from './FileUpload.svelte';

	let { channelId, channelName, onSend }: {
		channelId: string;
		channelName: string;
		onSend: (content: string) => void;
	} = $props();

	let content = $state('');
	let files = $state<File[]>([]);
	let sending = $state(false);
	let fileInput: HTMLInputElement | undefined = $state();

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}

	async function send() {
		const trimmed = content.trim();
		if (!trimmed && files.length === 0) return;
		if (sending) return;

		if (files.length > 0) {
			// Send via REST API with multipart form (supports file attachments).
			sending = true;
			try {
				await createMessage(channelId, trimmed, files);
				content = '';
				files = [];
			} catch (e) {
				console.error('Failed to send message with attachments:', e);
			} finally {
				sending = false;
			}
		} else {
			// Text-only: send via WebSocket for lower latency.
			if (!trimmed) return;
			onSend(trimmed);
			content = '';
		}
	}

	function openFilePicker() {
		fileInput?.click();
	}

	function handleFileSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		if (input.files) {
			files = [...files, ...Array.from(input.files)];
		}
		// Reset so the same file can be selected again.
		input.value = '';
	}

	function removeFile(index: number) {
		files = files.filter((_, i) => i !== index);
	}

	function handlePaste(e: ClipboardEvent) {
		const items = e.clipboardData?.items;
		if (!items) return;

		const imageFiles: File[] = [];
		for (const item of items) {
			if (item.type.startsWith('image/')) {
				const blob = item.getAsFile();
				if (blob) {
					const ext = blob.type.split('/')[1]?.replace('jpeg', 'jpg') || 'png';
					const name = `pasted-image-${Date.now()}.${ext}`;
					const file = new File([blob], name, { type: blob.type });
					imageFiles.push(file);
				}
			}
		}

		if (imageFiles.length > 0) {
			e.preventDefault();
			files = [...files, ...imageFiles];
		}
	}

	function handleDragOver(e: DragEvent) {
		e.preventDefault();
	}

	function handleDrop(e: DragEvent) {
		e.preventDefault();
		if (e.dataTransfer?.files) {
			files = [...files, ...Array.from(e.dataTransfer.files)];
		}
	}
</script>

<div
	class="input-wrapper"
	role="region"
	ondragover={handleDragOver}
	ondrop={handleDrop}
>
	<FileUpload bind:files onRemove={removeFile} />
	<div class="input-row">
		<button class="attach-btn" onclick={openFilePicker} aria-label="Attach files" title="Attach files">
			<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
				<path d="M21.44 11.05l-9.19 9.19a6 6 0 01-8.49-8.49l9.19-9.19a4 4 0 015.66 5.66l-9.2 9.19a2 2 0 01-2.83-2.83l8.49-8.48"/>
			</svg>
		</button>
		<textarea
			class="message-input"
			placeholder="Message #{channelName}"
			bind:value={content}
			onkeydown={handleKeydown}
			onpaste={handlePaste}
			rows="1"
			disabled={sending}
		></textarea>
	</div>
	<input
		bind:this={fileInput}
		type="file"
		multiple
		onchange={handleFileSelect}
		style="display:none"
	/>
</div>

<style>
	.input-wrapper {
		padding: 0 16px 16px;
	}

	.input-row {
		display: flex;
		align-items: center;
		gap: 8px;
		background: var(--bg-input);
		border-radius: 8px;
		padding: 0 8px;
	}

	.attach-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 36px;
		height: 36px;
		flex-shrink: 0;
		color: var(--text-muted);
		background: none;
		border: none;
		border-radius: 4px;
		cursor: pointer;
		padding: 0;
	}

	.attach-btn:hover {
		color: var(--text-primary);
	}

	.message-input {
		flex: 1;
		padding: 12px 8px;
		background: transparent;
		color: var(--text-primary);
		resize: none;
		min-height: 44px;
		max-height: 200px;
		line-height: 1.4;
		border: none;
	}

	.message-input::placeholder {
		color: var(--text-muted);
	}

	.message-input:disabled {
		opacity: 0.6;
	}

	@media (max-width: 768px) {
		.input-wrapper {
			padding: 0 8px 8px;
		}

		.message-input {
			padding: 10px 8px;
		}
	}
</style>
