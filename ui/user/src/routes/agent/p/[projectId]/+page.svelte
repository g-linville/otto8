<script lang="ts">
	import ProjectStartThread from '$lib/components/nanobot/ProjectStartThread.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { getContext } from 'svelte';
	import type { ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import { page } from '$app/state';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { errors } from '$lib/stores';

	let { data } = $props();
	let agent = $derived(data.agent);
	let projectId = $derived(data.projects[0].id);
	let tid = $derived(page.url.searchParams.get('tid'));
	let session = $derived($nanobotChat?.sessions?.find((s) => s.id === tid));
	let browserBaseUrl = $derived(data.agent.connectURL);

	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);

	let displayChat = $derived($nanobotChat?.chat);

	let fileToDelete = $state<string | undefined>();
	let deleting = $state(false);

	function handleFileDelete(uri: string) {
		fileToDelete = uri;
	}

	async function confirmDeleteFile() {
		if (!fileToDelete || !$nanobotChat?.api) return;
		deleting = true;
		try {
			await $nanobotChat.api.deleteFile(fileToDelete);
			nanobotChat.update((data) => {
				if (data) {
					data.resources = data.resources.filter((r) => r.uri !== fileToDelete);
				}
				return data;
			});
			displayChat?.refreshResources();
			fileToDelete = undefined;
		} catch (err) {
			errors.append(`Failed to delete file: ${err}`);
		} finally {
			deleting = false;
		}
	}
</script>

{#if displayChat}
	{#key displayChat}
		<ProjectStartThread
			agentId={agent.id}
			{projectId}
			{browserBaseUrl}
			browserAvailable={projectLayout.browserAvailable}
			bind:browserViewerOpen={projectLayout.browserViewerOpen}
			chat={displayChat}
			onFileOpen={projectLayout.handleFileOpen}
			onFileDelete={handleFileDelete}
			suppressEmptyState
			onThreadContentWidth={projectLayout.setThreadContentWidth}
		/>
	{/key}
{/if}

<Confirm
	msg="Delete this file?"
	show={fileToDelete !== undefined}
	loading={deleting}
	onsuccess={confirmDeleteFile}
	oncancel={() => (fileToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | {session?.title || 'Untitled'}</title>
</svelte:head>
