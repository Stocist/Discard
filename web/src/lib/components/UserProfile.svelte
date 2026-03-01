<script lang="ts">
	import type { User } from '$lib/types';
	import { updateMe } from '$lib/api';
	import ImageCropper from './ImageCropper.svelte';

	let { user, onclose, onsave }: {
		user: User;
		onclose: () => void;
		onsave?: (updated: User) => void;
	} = $props();

	let displayName = $state(user.display_name ?? '');
	let saving = $state(false);
	let error = $state('');
	let avatarFile = $state<File | null>(null);
	let avatarPreview = $state<string | null>(
		user.avatar_path ? `/uploads/${user.avatar_path}` : null
	);
	let fileInput: HTMLInputElement;
	let showCropper = $state(false);
	let cropperSrc = $state('');

	function handleAvatarSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		if (!file.type.startsWith('image/')) {
			error = 'Please select an image file.';
			return;
		}
		cropperSrc = URL.createObjectURL(file);
		showCropper = true;
		// Reset input so the same file can be re-selected
		input.value = '';
	}

	function handleCropSave(blob: Blob) {
		avatarFile = new File([blob], 'avatar.png', { type: 'image/png' });
		avatarPreview = URL.createObjectURL(blob);
		showCropper = false;
	}

	function handleCropCancel() {
		showCropper = false;
	}

	async function handleSave() {
		if (saving) return;
		const trimmed = displayName.trim();
		if (trimmed.length > 64) {
			error = 'Display name must be 64 characters or less.';
			return;
		}
		const nameChanged = trimmed !== (user.display_name ?? '');
		if (!nameChanged && !avatarFile) {
			onclose();
			return;
		}
		saving = true;
		error = '';
		try {
			const updated = await updateMe(
				nameChanged ? trimmed : undefined,
				avatarFile ?? undefined
			);
			if (onsave) onsave(updated);
			onclose();
		} catch (e) {
			error = 'Failed to update profile.';
			console.error(e);
		} finally {
			saving = false;
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
		<h2>User Profile</h2>

		{#if error}
			<p class="error">{error}</p>
		{/if}

		<div class="avatar-section">
			<label class="field-label">Avatar</label>
			<button type="button" class="avatar-upload" onclick={() => fileInput.click()}>
				{#if avatarPreview}
					<img src={avatarPreview} alt="Avatar" class="avatar-preview" />
				{:else}
					<span class="avatar-letter">{user.username.charAt(0).toUpperCase()}</span>
				{/if}
				<span class="avatar-overlay">Change</span>
			</button>
			<input
				bind:this={fileInput}
				type="file"
				accept="image/*"
				hidden
				onchange={handleAvatarSelect}
			/>
		</div>

		<form onsubmit={(e) => { e.preventDefault(); handleSave(); }}>
			<label class="field-label" for="profile-username">Username</label>
			<input
				id="profile-username"
				type="text"
				value={user.username}
				disabled
				class="disabled-input"
			/>

			<label class="field-label" for="profile-display-name">Display Name</label>
			<input
				id="profile-display-name"
				type="text"
				maxlength="64"
				placeholder={user.username}
				bind:value={displayName}
			/>

			<div class="modal-actions">
				<button type="button" class="cancel-btn" onclick={onclose}>Cancel</button>
				<button type="submit" class="save-btn" disabled={saving}>
					{saving ? 'Saving...' : 'Save'}
				</button>
			</div>
		</form>
	</div>
</div>

{#if showCropper}
	<ImageCropper
		src={cropperSrc}
		shape="circle"
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

	.field-label {
		display: block;
		font-size: 12px;
		font-weight: 600;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.02em;
		margin-bottom: 6px;
	}

	.avatar-section {
		margin-bottom: 16px;
	}

	.avatar-upload {
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

	.avatar-preview {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.avatar-letter {
		font-size: 32px;
		font-weight: 600;
		color: var(--text-primary);
	}

	.avatar-overlay {
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

	.avatar-upload:hover .avatar-overlay {
		opacity: 1;
	}

	.modal input {
		width: 100%;
		padding: 10px 12px;
		background: var(--bg-input);
		border-radius: 4px;
		color: var(--text-primary);
		margin-bottom: 16px;
	}

	.disabled-input {
		opacity: 0.5;
		cursor: not-allowed;
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
</style>
