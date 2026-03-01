<script lang="ts">
	import type { Server } from '$lib/types';
	import { updateServer, deleteServer } from '$lib/api';
	import ImageCropper from './ImageCropper.svelte';

	let { server, onclose, ondelete, onsave }: {
		server: Server;
		onclose: () => void;
		ondelete: () => void;
		onsave?: (updated: Server) => void;
	} = $props();

	let name = $state(server.name);
	let saving = $state(false);
	let deleting = $state(false);
	let confirmDelete = $state(false);
	let error = $state('');
	let iconFile = $state<File | null>(null);
	let iconPreview = $state<string | null>(server.icon_path ? `/uploads/${server.icon_path}` : null);
	let fileInput: HTMLInputElement;
	let showCropper = $state(false);
	let cropperSrc = $state('');

	function handleIconSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		if (!file.type.startsWith('image/')) {
			error = 'Please select an image file.';
			return;
		}
		cropperSrc = URL.createObjectURL(file);
		showCropper = true;
		input.value = '';
	}

	function handleCropSave(blob: Blob) {
		iconFile = new File([blob], 'icon.png', { type: 'image/png' });
		iconPreview = URL.createObjectURL(blob);
		showCropper = false;
	}

	function handleCropCancel() {
		showCropper = false;
	}

	async function handleSave() {
		const trimmed = name.trim();
		if (!trimmed || saving) return;
		if (trimmed === server.name && !iconFile) {
			onclose();
			return;
		}
		saving = true;
		error = '';
		try {
			const updated = await updateServer(server.id, trimmed, iconFile ?? undefined);
			if (onsave) onsave(updated);
			onclose();
		} catch (e) {
			error = 'Failed to update server settings.';
			console.error(e);
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (deleting) return;
		deleting = true;
		error = '';
		try {
			await deleteServer(server.id);
			ondelete();
		} catch (e) {
			error = 'Failed to delete server.';
			console.error(e);
		} finally {
			deleting = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="modal-overlay" onclick={onclose}>
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="modal" onclick={(e) => e.stopPropagation()}>
		<h2>Server Settings</h2>

		{#if error}
			<p class="error">{error}</p>
		{/if}

		<div class="icon-section">
			<label class="field-label">Server Icon</label>
			<button type="button" class="icon-upload" onclick={() => fileInput.click()}>
				{#if iconPreview}
					<img src={iconPreview} alt="Server icon" class="icon-preview" />
				{:else}
					<span class="icon-letter">{server.name.charAt(0).toUpperCase()}</span>
				{/if}
				<span class="icon-overlay">Change</span>
			</button>
			<input
				bind:this={fileInput}
				type="file"
				accept="image/*"
				hidden
				onchange={handleIconSelect}
			/>
		</div>

		<form onsubmit={(e) => { e.preventDefault(); handleSave(); }}>
			<label class="field-label" for="server-name">Server Name</label>
			<input
				id="server-name"
				type="text"
				maxlength="100"
				bind:value={name}
			/>
			<div class="modal-actions">
				<button type="button" class="cancel-btn" onclick={onclose}>Cancel</button>
				<button type="submit" class="save-btn" disabled={!name.trim() || saving}>
					{saving ? 'Saving...' : 'Save'}
				</button>
			</div>
		</form>

		<div class="danger-zone">
			<h3>Danger Zone</h3>
			{#if !confirmDelete}
				<button class="delete-btn" onclick={() => (confirmDelete = true)}>
					Delete Server
				</button>
			{:else}
				<p class="delete-warning">This will permanently delete the server and all its channels and messages. This cannot be undone.</p>
				<div class="delete-actions">
					<button class="cancel-btn" onclick={() => (confirmDelete = false)}>Cancel</button>
					<button class="delete-confirm-btn" onclick={handleDelete} disabled={deleting}>
						{deleting ? 'Deleting...' : 'Yes, Delete Server'}
					</button>
				</div>
			{/if}
		</div>
	</div>
</div>

{#if showCropper}
	<ImageCropper
		src={cropperSrc}
		shape="square"
		onsave={handleCropSave}
		oncancel={handleCropCancel}
	/>
{/if}

<style>
	.modal-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.7);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 100;
	}

	.modal {
		background: var(--bg-primary);
		border-radius: 8px;
		padding: 24px;
		width: 440px;
		max-width: 90vw;
	}

	.modal h2 {
		margin-bottom: 16px;
		font-size: 20px;
	}

	.error {
		color: #ef4444;
		font-size: 13px;
		margin-bottom: 12px;
	}

	.icon-section {
		margin-bottom: 16px;
	}

	.icon-upload {
		position: relative;
		width: 80px;
		height: 80px;
		border-radius: 50%;
		background: var(--bg-sidebar);
		display: flex;
		align-items: center;
		justify-content: center;
		overflow: hidden;
		cursor: pointer;
	}

	.icon-preview {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.icon-letter {
		font-size: 32px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.icon-overlay {
		position: absolute;
		inset: 0;
		background: rgba(0, 0, 0, 0.6);
		color: white;
		font-size: 12px;
		font-weight: 600;
		text-transform: uppercase;
		display: flex;
		align-items: center;
		justify-content: center;
		opacity: 0;
		transition: opacity 0.15s;
	}

	.icon-upload:hover .icon-overlay {
		opacity: 1;
	}

	.field-label {
		display: block;
		font-size: 12px;
		font-weight: 600;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.02em;
		margin-bottom: 6px;
	}

	.modal input {
		width: 100%;
		padding: 10px 12px;
		background: var(--bg-input);
		border-radius: 4px;
		color: var(--text-primary);
		margin-bottom: 16px;
	}

	.modal-actions {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
	}

	.cancel-btn {
		padding: 8px 16px;
		color: var(--text-muted);
	}

	.cancel-btn:hover {
		color: var(--text-primary);
	}

	.save-btn {
		padding: 8px 16px;
		background: var(--accent);
		color: white;
		border-radius: 4px;
		font-weight: 500;
	}

	.save-btn:hover:not(:disabled) {
		background: var(--accent-hover);
	}

	.save-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.danger-zone {
		margin-top: 24px;
		padding-top: 16px;
		border-top: 1px solid var(--border);
	}

	.danger-zone h3 {
		font-size: 14px;
		font-weight: 600;
		color: #ef4444;
		margin-bottom: 12px;
	}

	.delete-btn {
		padding: 8px 16px;
		background: transparent;
		border: 1px solid #ef4444;
		color: #ef4444;
		border-radius: 4px;
		font-weight: 500;
	}

	.delete-btn:hover {
		background: #ef4444;
		color: white;
	}

	.delete-warning {
		font-size: 13px;
		color: var(--text-muted);
		margin-bottom: 12px;
		line-height: 1.5;
	}

	.delete-actions {
		display: flex;
		gap: 8px;
	}

	.delete-confirm-btn {
		padding: 8px 16px;
		background: #ef4444;
		color: white;
		border-radius: 4px;
		font-weight: 500;
	}

	.delete-confirm-btn:hover:not(:disabled) {
		background: #dc2626;
	}

	.delete-confirm-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
