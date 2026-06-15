<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { Group, type AuditLog } from '$lib/services';
	import { profile, userDeviceSettings } from '$lib/stores';
	import { formatLogTimestamp } from '$lib/time';
	import {
		auditLogEventLabel,
		auditLogOutcomeLabel,
		auditLogSourceLabel,
		isLocalAgentAuditLog
	} from './labels';
	import { X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		auditLog: AuditLog & {
			user: string;
		};
		onClose: () => void;
	}

	let { auditLog, onClose }: Props = $props();

	let hasAuditorAccess = $derived(profile.current.groups.includes(Group.AUDITOR));
	let isLocalAgent = $derived(isLocalAgentAuditLog(auditLog));

	// Full payloads are returned by the API for auditors, or for users viewing
	// their own non-PowerUserWorkspace MCP server logs. Admin without Auditor sees metadata.
	const shouldShowPayload = $derived(
		hasAuditorAccess ||
			(!isLocalAgent && auditLog.userID === profile.current.id && !auditLog.powerUserWorkspaceID)
	);

	function hasBody(body: unknown) {
		if (body == null) return false;
		if (typeof body === 'object' && !Array.isArray(body)) {
			return Object.keys(body).length > 0;
		}
		return true;
	}

	function clientDisplay(client: AuditLog['client']) {
		if (!client) return '';
		const version = client.version?.trim();
		if (!version || version.toLowerCase() === 'unknown') return client.name;
		return `${client.name}/${version}`;
	}
</script>

<div class="bg-base-200 text-base-content flex h-full w-[inherit] min-w-[inherit] flex-col">
	<div class="dark:bg-base-300 bg-base-100 relative flex w-full flex-col p-4 pl-5 shadow-xs">
		<div
			class={twMerge(
				'absolute top-0 left-0 h-full w-1',
				auditLog.outcome === 'error' || auditLog.responseStatus >= 400 ? 'bg-error' : 'bg-primary'
			)}
		></div>
		<h3 class="text-lg font-semibold">
			{formatLogTimestamp(auditLog.createdAt, userDeviceSettings.timeFormat)}
		</h3>
		<p class="text-muted-content text-xs font-light">
			{auditLogSourceLabel(auditLog.sourceType)}
			{#if auditLog.eventID}
				<span> - {auditLog.eventID}</span>
			{:else if auditLog.requestID}
				<span> - {auditLog.requestID}</span>
			{/if}
		</p>
		<IconButton onclick={onClose} class="absolute top-1/2 right-4 -translate-y-1/2">
			<X class="size-5" />
		</IconButton>
	</div>
	<div class="default-scrollbar-thin relative h-[calc(100%-60px)] overflow-y-auto pb-4">
		<div class="bg-base-300 absolute top-0 left-0 h-full w-1"></div>

		<div class="flex flex-wrap gap-2 p-4 pl-5">
			<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light">
				<span class="font-medium">Source:</span>
				{auditLogSourceLabel(auditLog.sourceType)}
			</div>
			{#if auditLog.eventType}
				<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light">
					<span class="font-medium">Event:</span>
					{auditLogEventLabel(auditLog.eventType)}
				</div>
			{/if}
			<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light">
				<span class="font-medium">Outcome:</span>
				{auditLogOutcomeLabel(auditLog.outcome, auditLog.responseStatus)}
			</div>
			{#if auditLog.callType}
				<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light">
					<span class="font-medium">{isLocalAgent ? 'Tool Type' : 'Call Type'}:</span>
					{auditLog.callType}
				</div>
			{/if}
			{#if auditLog.sessionID}
				<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light">
					<span class="font-medium">Session ID:</span>
					{auditLog.sessionID}
				</div>
			{/if}
			{#if auditLog.mcpID}
				<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light">
					<span class="font-medium">Server:</span>
					{auditLog.mcpServerDisplayName} ({auditLog.mcpID})
				</div>
			{/if}
			{#if auditLog.deviceID}
				<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light">
					<span class="font-medium">Device:</span>
					{auditLog.deviceID}
				</div>
			{/if}
			{#if auditLog.context?.workspace}
				<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light">
					<span class="font-medium">Workspace:</span>
					{auditLog.context.workspace}
				</div>
			{/if}
			{#if auditLog.mcpServerCatalogEntryName}
				<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light">
					<span class="font-medium">Parent Entry ID:</span>
					{auditLog.mcpServerCatalogEntryName}
				</div>
			{/if}
		</div>

		<div class="p-4 pl-5">
			<h4 class="text-lg font-semibold">{isLocalAgent ? 'Local Agent Request' : 'HTTP Request'}</h4>
			<div class="flex flex-col gap-1 px-4 py-2 text-sm font-light">
				{#if auditLog.user}
					<p><span class="font-medium">User</span>: {auditLog.user}</p>
				{/if}
				{#if auditLog.apiKey}
					<p>
						<span class="font-medium">API Key</span>: {auditLog.apiKey}***
						<span class="text-muted-content text-xs italic">(redacted)</span>
					</p>
				{/if}
				{#if auditLog.userAgent}
					<p><span class="font-medium">User Agent</span>: {auditLog.userAgent}</p>
				{/if}
				{#if auditLog.client}
					<p>
						<span class="font-medium">Client</span>: {clientDisplay(auditLog.client)}
					</p>
				{/if}
				{#if auditLog.clientIP}
					<p><span class="font-medium">Client IP</span>: {auditLog.clientIP}</p>
				{/if}
				{#if auditLog.callIdentifier}
					<p><span class="font-medium">Tool</span>: {auditLog.callIdentifier}</p>
				{/if}
				{#if auditLog.context?.cwd}
					<p><span class="font-medium">CWD</span>: {auditLog.context.cwd}</p>
				{/if}
				{#if auditLog.context?.gitBranch || auditLog.context?.gitRemote}
					<p>
						<span class="font-medium">Git</span>:
						{[auditLog.context?.gitBranch, auditLog.context?.gitRemote].filter(Boolean).join(' - ')}
					</p>
				{/if}
				{#if auditLog.context?.hostname || auditLog.context?.os || auditLog.context?.arch}
					<p>
						<span class="font-medium">Host</span>:
						{[auditLog.context?.hostname, auditLog.context?.os, auditLog.context?.arch]
							.filter(Boolean)
							.join(' - ')}
					</p>
				{/if}
			</div>

			{#if shouldShowPayload && !isLocalAgent}
				{#if auditLog.requestHeaders}
					<p class="my-2 text-base font-semibold">Request Headers</p>

					<div
						class="dark:bg-base-300 bg-base-100 relative flex flex-col gap-2 overflow-hidden rounded-md p-4 pl-5"
					>
						<div class="bg-primary/50 absolute top-0 left-0 h-full w-1"></div>
						<div class="flex flex-col gap-1">
							{#each Object.entries(auditLog.requestHeaders ?? {}) as [key, value] (key)}
								<p>
									<span class="font-medium">{key}</span>: {value}
								</p>
							{/each}
						</div>
					</div>
				{:else if !hasAuditorAccess}
					{@render noAuditorAccessInfo('Request Headers')}
				{/if}
			{/if}

			{#if shouldShowPayload}
				{#if hasBody(auditLog.requestBody)}
					{@render jsonBody(isLocalAgent ? 'Request' : 'Request Body', auditLog.requestBody)}
				{:else if !hasAuditorAccess}
					{@render noAuditorAccessInfo(isLocalAgent ? 'Request' : 'Request Body')}
				{/if}

				{#if !isLocalAgent && hasBody(auditLog.mutatedRequestBody)}
					{@render jsonBody('Mutated Request Body', auditLog.mutatedRequestBody)}
				{/if}
			{:else}
				{@render noAuditorAccessInfo(isLocalAgent ? 'Request' : 'Request Body')}
			{/if}
		</div>

		<div class="p-4 pl-5">
			<div class="flex items-center gap-2">
				<h4 class="text-lg font-semibold">
					{isLocalAgent ? 'Local Agent Response' : 'HTTP Response'}
				</h4>
				{#if !isLocalAgent && auditLog.responseStatus}
					<p
						class={twMerge(
							'w-fit rounded-full px-3 py-1 text-xs font-semibold text-white',
							auditLog.responseStatus >= 400 ? 'bg-error' : 'bg-primary'
						)}
					>
						{auditLog.responseStatus}
					</p>
				{/if}
			</div>

			{#if shouldShowPayload && !isLocalAgent}
				{#if auditLog.responseHeaders}
					<p class="mt-4 mb-2 text-base font-semibold">Response Headers</p>
					<div
						class="dark:bg-base-300 bg-base-100 relative flex flex-col gap-2 overflow-hidden rounded-md p-4 pl-5"
					>
						<div class="bg-primary/50 absolute top-0 left-0 h-full w-1"></div>
						<div class="flex flex-col gap-1">
							{#each Object.entries(auditLog.responseHeaders ?? {}) as [key, value] (key)}
								<p>
									<span class="font-medium">{key}</span>: {value}
								</p>
							{/each}
						</div>
					</div>
				{:else if !hasAuditorAccess}
					{@render noAuditorAccessInfo('Response Headers')}
				{/if}
			{/if}

			{#if auditLog.error}
				<div class="mt-4 flex flex-col">
					<div class="mb-2 text-base font-semibold">
						{isLocalAgent ? 'Error' : 'Response Error'}
					</div>
					<p class="text-error">{auditLog.error}</p>
				</div>
			{/if}

			{#if shouldShowPayload}
				{#if !isLocalAgent && hasBody(auditLog.originalResponseBody)}
					{@render jsonBody('Original Response Body', auditLog.originalResponseBody)}
				{/if}

				{#if hasBody(auditLog.responseBody)}
					{@render jsonBody(isLocalAgent ? 'Response' : 'Response Body', auditLog.responseBody)}
				{:else if !hasAuditorAccess}
					{@render noAuditorAccessInfo(isLocalAgent ? 'Response' : 'Response Body')}
				{/if}

				{#if auditLog.errorDetail && auditLog.errorDetail !== auditLog.error}
					{@render textBody('Error Detail', auditLog.errorDetail)}
				{/if}

				{#if isLocalAgent && hasBody(auditLog.rawEvent)}
					{@render jsonBody('Raw Event', auditLog.rawEvent)}
				{/if}

				{#if !isLocalAgent && auditLog.webhookStatuses && auditLog.webhookStatuses.length > 0}
					{@const statuses = JSON.stringify(auditLog.webhookStatuses, null, 2)}

					<p class="translate-y-2 pt-4 text-base font-semibold">Webhook Statuses</p>
					<div class="relative text-white">
						<pre class="default-scrollbar-thin max-h-96 overflow-y-auto p-4">
						<code class="language-json">{statuses}</code>
					</pre>

						<CopyButton
							classes={{ button: 'absolute right-4 top-4 flex flex-col items-end text-current' }}
							text={statuses}
						/>
					</div>
				{/if}
			{:else}
				{@render noAuditorAccessInfo(isLocalAgent ? 'Response and Raw Event' : 'Response Body')}
			{/if}
		</div>
	</div>
</div>

{#snippet jsonBody(name: string, value: unknown)}
	{@const body = JSON.stringify(value, null, 2)}

	<p class="translate-y-2 pt-4 text-base font-semibold">{name}</p>
	<div class="relative text-white">
		<pre class="default-scrollbar-thin max-h-96 overflow-y-auto p-4">
			<code class="language-json">{body}</code>
		</pre>

		<CopyButton
			classes={{ button: 'absolute right-4 top-4 flex flex-col items-end text-current' }}
			text={body}
		/>
	</div>
{/snippet}

{#snippet textBody(name: string, value: string)}
	<p class="translate-y-2 pt-4 text-base font-semibold">{name}</p>
	<div class="relative text-white">
		<pre class="default-scrollbar-thin max-h-96 overflow-y-auto p-4">
			<code>{value}</code>
		</pre>

		<CopyButton
			classes={{ button: 'absolute right-4 top-4 flex flex-col items-end text-current' }}
			text={value}
		/>
	</div>
{/snippet}

{#snippet noAuditorAccessInfo(name: string)}
	<p class="mt-4 mb-2 text-base font-semibold">{name}</p>
	<div class="text-muted-content text-xs">
		<i>Details are hidden; auditor role is required to access this information.</i>
	</div>
{/snippet}
