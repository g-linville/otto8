<script lang="ts">
	import type { ChatMessageItemToolCall } from '$lib/services/nanobot/types';
	import { parseToolFilePath } from '$lib/services/nanobot/utils';
	import FileItem from '$lib/components/nanobot/FileItem.svelte';
	import { Trash2 } from 'lucide-svelte';
	interface Props {
		item: ChatMessageItemToolCall;
		onFileOpen?: (filename: string) => void;
		onFileDelete?: (uri: string) => void;
	}

	let { item, onFileOpen, onFileDelete }: Props = $props();

	const pending = $derived(item.hasMore);
	const filename = $derived(item.arguments ? (parseToolFilePath(item) ?? '') : (item.name ?? ''));
	const name = $derived(filename ? filename.split('/').pop() : null);
	const fileUri = $derived(filename.startsWith('file:///') ? filename : `file:///${filename}`);
</script>

<div
	class="rounded-field border-base-300 bg-base-100 group mt-3 mb-2 w-full border shadow-xs transition-colors"
>
	<div class="flex items-center">
		<button
			class="tooltip hover:bg-base-300 flex min-w-0 flex-1 items-center gap-2 rounded-l-[inherit] p-3 transition-colors"
			data-tip={`Open ${filename}`}
			onclick={() => {
				onFileOpen?.(filename);
			}}
			disabled={pending}
		>
			<FileItem uri={filename} compact />

			{#if pending}
				<span class="skeleton skeleton-text bg-transparent text-sm">...</span>
			{:else}
				<span class="text-sm">{name}</span>
			{/if}
		</button>
		{#if onFileDelete && !pending}
			<button
				class="btn btn-ghost btn-square btn-xs tooltip mr-2 opacity-0 transition-opacity group-hover:opacity-100"
				data-tip="Delete file"
				onclick={(e: MouseEvent) => {
					e.stopPropagation();
					onFileDelete(fileUri);
				}}
			>
				<Trash2 class="size-4" />
			</button>
		{/if}
	</div>
</div>
