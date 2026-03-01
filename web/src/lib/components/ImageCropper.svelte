<script lang="ts">
	let { src, shape = 'circle', onsave, oncancel }: {
		src: string;
		shape?: 'circle' | 'square';
		onsave: (blob: Blob) => void;
		oncancel: () => void;
	} = $props();

	let canvas: HTMLCanvasElement;
	let containerEl: HTMLDivElement;

	const CROP_SIZE = 280;
	const OUTPUT_SIZE = 512;

	let img = $state<HTMLImageElement | null>(null);
	let imgLoaded = $state(false);
	let zoom = $state(1);
	let offsetX = $state(0);
	let offsetY = $state(0);
	let dragging = $state(false);
	let dragStartX = $state(0);
	let dragStartY = $state(0);
	let dragOffsetX = $state(0);
	let dragOffsetY = $state(0);

	let minZoom = $derived.by(() => {
		if (!img) return 1;
		const scaleX = CROP_SIZE / img.naturalWidth;
		const scaleY = CROP_SIZE / img.naturalHeight;
		return Math.max(scaleX, scaleY);
	});

	let maxZoom = $derived(minZoom * 5);

	$effect(() => {
		const image = new Image();
		image.onload = () => {
			img = image;
			const scaleX = CROP_SIZE / image.naturalWidth;
			const scaleY = CROP_SIZE / image.naturalHeight;
			const fitZoom = Math.max(scaleX, scaleY);
			zoom = fitZoom;
			offsetX = 0;
			offsetY = 0;
			imgLoaded = true;
		};
		image.src = src;
	});

	$effect(() => {
		if (!img || !imgLoaded || !canvas) return;
		drawCanvas(img, zoom, offsetX, offsetY);
	});

	function drawCanvas(image: HTMLImageElement, z: number, ox: number, oy: number) {
		const ctx = canvas.getContext('2d');
		if (!ctx) return;

		const w = canvas.width;
		const h = canvas.height;

		ctx.clearRect(0, 0, w, h);

		const drawW = image.naturalWidth * z;
		const drawH = image.naturalHeight * z;
		const drawX = (w - drawW) / 2 + ox;
		const drawY = (h - drawH) / 2 + oy;

		ctx.drawImage(image, drawX, drawY, drawW, drawH);

		// Draw overlay outside crop area
		ctx.fillStyle = 'rgba(0, 0, 0, 0.6)';
		const cropX = (w - CROP_SIZE) / 2;
		const cropY = (h - CROP_SIZE) / 2;

		if (shape === 'circle') {
			ctx.save();
			ctx.beginPath();
			ctx.rect(0, 0, w, h);
			ctx.arc(w / 2, h / 2, CROP_SIZE / 2, 0, Math.PI * 2, true);
			ctx.fill('evenodd');
			ctx.restore();

			// Draw circle border
			ctx.strokeStyle = 'rgba(255, 255, 255, 0.3)';
			ctx.lineWidth = 2;
			ctx.beginPath();
			ctx.arc(w / 2, h / 2, CROP_SIZE / 2, 0, Math.PI * 2);
			ctx.stroke();
		} else {
			// Top
			ctx.fillRect(0, 0, w, cropY);
			// Bottom
			ctx.fillRect(0, cropY + CROP_SIZE, w, h - cropY - CROP_SIZE);
			// Left
			ctx.fillRect(0, cropY, cropX, CROP_SIZE);
			// Right
			ctx.fillRect(cropX + CROP_SIZE, cropY, w - cropX - CROP_SIZE, CROP_SIZE);

			// Draw square border
			ctx.strokeStyle = 'rgba(255, 255, 255, 0.3)';
			ctx.lineWidth = 2;
			ctx.strokeRect(cropX, cropY, CROP_SIZE, CROP_SIZE);
		}
	}

	function clampOffset(ox: number, oy: number, z: number): [number, number] {
		if (!img) return [ox, oy];
		const drawW = img.naturalWidth * z;
		const drawH = img.naturalHeight * z;
		const maxOx = Math.max(0, (drawW - CROP_SIZE) / 2);
		const maxOy = Math.max(0, (drawH - CROP_SIZE) / 2);
		return [
			Math.max(-maxOx, Math.min(maxOx, ox)),
			Math.max(-maxOy, Math.min(maxOy, oy))
		];
	}

	function handlePointerDown(e: PointerEvent) {
		dragging = true;
		dragStartX = e.clientX;
		dragStartY = e.clientY;
		dragOffsetX = offsetX;
		dragOffsetY = offsetY;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}

	function handlePointerMove(e: PointerEvent) {
		if (!dragging) return;
		const dx = e.clientX - dragStartX;
		const dy = e.clientY - dragStartY;
		[offsetX, offsetY] = clampOffset(dragOffsetX + dx, dragOffsetY + dy, zoom);
	}

	function handlePointerUp() {
		dragging = false;
	}

	function handleZoom(e: Event) {
		const value = parseFloat((e.target as HTMLInputElement).value);
		zoom = value;
		[offsetX, offsetY] = clampOffset(offsetX, offsetY, zoom);
	}

	function handleWheel(e: WheelEvent) {
		e.preventDefault();
		const delta = -e.deltaY * 0.001;
		const newZoom = Math.max(minZoom, Math.min(maxZoom, zoom + delta));
		zoom = newZoom;
		[offsetX, offsetY] = clampOffset(offsetX, offsetY, zoom);
	}

	function handleSave() {
		if (!img) return;

		const offscreen = document.createElement('canvas');
		offscreen.width = OUTPUT_SIZE;
		offscreen.height = OUTPUT_SIZE;
		const ctx = offscreen.getContext('2d');
		if (!ctx) return;

		const canvasW = canvas.width;
		const canvasH = canvas.height;
		const cropX = (canvasW - CROP_SIZE) / 2;
		const cropY = (canvasH - CROP_SIZE) / 2;

		const drawW = img.naturalWidth * zoom;
		const drawH = img.naturalHeight * zoom;
		const drawX = (canvasW - drawW) / 2 + offsetX;
		const drawY = (canvasH - drawH) / 2 + offsetY;

		// Map crop area to source image coordinates
		const srcX = (cropX - drawX) / zoom;
		const srcY = (cropY - drawY) / zoom;
		const srcW = CROP_SIZE / zoom;
		const srcH = CROP_SIZE / zoom;

		if (shape === 'circle') {
			ctx.beginPath();
			ctx.arc(OUTPUT_SIZE / 2, OUTPUT_SIZE / 2, OUTPUT_SIZE / 2, 0, Math.PI * 2);
			ctx.clip();
		}

		ctx.drawImage(img, srcX, srcY, srcW, srcH, 0, 0, OUTPUT_SIZE, OUTPUT_SIZE);

		offscreen.toBlob(
			(blob) => {
				if (blob) onsave(blob);
			},
			'image/png',
			0.95
		);
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') oncancel();
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="cropper-overlay" onclick={oncancel}>
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="cropper-modal" onclick={(e) => e.stopPropagation()} bind:this={containerEl}>
		<h2>Crop Image</h2>

		<div
			class="cropper-canvas-wrap"
			onpointerdown={handlePointerDown}
			onpointermove={handlePointerMove}
			onpointerup={handlePointerUp}
			onpointercancel={handlePointerUp}
			onwheel={handleWheel}
			role="application"
			aria-label="Drag to reposition image"
		>
			<canvas
				bind:this={canvas}
				width={360}
				height={360}
				class="cropper-canvas"
				class:dragging
			></canvas>
			{#if !imgLoaded}
				<div class="cropper-loading">Loading...</div>
			{/if}
		</div>

		<div class="cropper-controls">
			<label class="zoom-label">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="11" cy="11" r="8"></circle>
					<line x1="21" y1="21" x2="16.65" y2="16.65"></line>
					<line x1="8" y1="11" x2="14" y2="11"></line>
				</svg>
				<input
					type="range"
					min={minZoom}
					max={maxZoom}
					step="0.001"
					value={zoom}
					oninput={handleZoom}
					class="zoom-slider"
				/>
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="11" cy="11" r="8"></circle>
					<line x1="21" y1="21" x2="16.65" y2="16.65"></line>
					<line x1="8" y1="11" x2="14" y2="11"></line>
					<line x1="11" y1="8" x2="11" y2="14"></line>
				</svg>
			</label>
		</div>

		<div class="cropper-actions">
			<button type="button" class="cancel-btn" onclick={oncancel}>Cancel</button>
			<button type="button" class="save-btn" onclick={handleSave} disabled={!imgLoaded}>
				Save
			</button>
		</div>
	</div>
</div>

<style>
	.cropper-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.8);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 150;
		animation: cropper-fade-in 0.15s ease-out;
	}

	@keyframes cropper-fade-in {
		from { opacity: 0; }
		to { opacity: 1; }
	}

	.cropper-modal {
		background: var(--bg-primary);
		border-radius: 8px;
		padding: 24px;
		width: 420px;
		max-width: 90vw;
	}

	.cropper-modal h2 {
		margin-bottom: 16px;
		font-size: 20px;
	}

	.cropper-canvas-wrap {
		position: relative;
		width: 360px;
		height: 360px;
		margin: 0 auto 16px;
		border-radius: 6px;
		overflow: hidden;
		background: #0c0a09;
		touch-action: none;
	}

	.cropper-canvas {
		display: block;
		cursor: grab;
		width: 360px;
		height: 360px;
	}

	.cropper-canvas.dragging {
		cursor: grabbing;
	}

	.cropper-loading {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-muted);
		font-size: 14px;
	}

	.cropper-controls {
		margin-bottom: 16px;
	}

	.zoom-label {
		display: flex;
		align-items: center;
		gap: 10px;
		color: var(--text-muted);
	}

	.zoom-slider {
		flex: 1;
		-webkit-appearance: none;
		appearance: none;
		height: 4px;
		background: var(--bg-tertiary);
		border-radius: 2px;
		outline: none;
	}

	.zoom-slider::-webkit-slider-thumb {
		-webkit-appearance: none;
		appearance: none;
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: var(--accent);
		cursor: pointer;
	}

	.zoom-slider::-moz-range-thumb {
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: var(--accent);
		cursor: pointer;
		border: none;
	}

	.cropper-actions {
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
