import type { AuditLog } from '$lib/services';

export function auditLogEventLabel(eventType?: string) {
	switch (eventType) {
		case 'tool_call':
			return 'Tool Call';
		case 'resource_read':
			return 'Resource Read';
		case 'prompt_get':
			return 'Prompt Get';
		case 'mcp_request':
			return 'MCP Request';
		default:
			return eventType || '--';
	}
}

export function auditLogOutcomeLabel(outcome?: string, responseStatus?: number) {
	if (outcome === 'success') return 'Success';
	if (outcome === 'error') return 'Error';
	if (responseStatus) return responseStatus >= 400 ? 'Error' : 'Success';
	return '--';
}

export function auditLogSourceLabel(sourceType?: string) {
	if (!sourceType || sourceType === 'mcp') return 'MCP Server';
	if (sourceType === 'local_agent') return 'Local Agent';
	return sourceType;
}

export function isLocalAgentAuditLog(auditLog: Pick<AuditLog, 'sourceType' | 'mcpID'>) {
	return (
		auditLog.sourceType === 'local_agent' || (!auditLog.mcpID && auditLog.sourceType !== 'mcp')
	);
}
