<script lang="ts">
	let { channelName, onSend }: {
		channelName: string;
		onSend: (content: string) => void;
	} = $props();

	let content = $state('');

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}

	function send() {
		const trimmed = content.trim();
		if (!trimmed) return;
		onSend(trimmed);
		content = '';
	}
</script>

<div class="input-wrapper">
	<textarea
		class="message-input"
		placeholder="Message #{channelName}"
		bind:value={content}
		onkeydown={handleKeydown}
		rows="1"
	></textarea>
</div>

<style>
	.input-wrapper {
		padding: 0 16px 16px;
	}

	.message-input {
		width: 100%;
		padding: 12px 16px;
		background: var(--bg-input);
		border-radius: 8px;
		color: var(--text-primary);
		resize: none;
		min-height: 44px;
		max-height: 200px;
		line-height: 1.4;
	}

	.message-input::placeholder {
		color: var(--text-muted);
	}
</style>
