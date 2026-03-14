import { apiRequest } from '@/features/control/api'
import { normalizeSettings } from '@/features/control/utils'
import type {
  AdminSession,
  AgentDefinition,
  AuthStatus,
  BehaviorRuntimeResponse,
  ConversationMessagesResponse,
  ConversationsResponse,
  LogEntry,
  PersonaGroup,
  PersonaGroupMember,
  PersonaResolveResponse,
  PersonaUserProfile,
  ServiceStatus,
  SettingsResponse,
  VariableValue,
} from '@/features/control/types'

export const controlQueryKeys = {
  base: ['control'] as const,
  adminSession: () => ['control', 'adminSession'] as const,
  authStatus: () => ['control', 'authStatus'] as const,
  serviceStatus: () => ['control', 'serviceStatus'] as const,
  settings: () => ['control', 'settings'] as const,
  soul: () => ['control', 'soul'] as const,
  logs: () => ['control', 'logs'] as const,
  agents: () => ['control', 'agents'] as const,
  variables: () => ['control', 'variables'] as const,
  behaviorRuntime: () => ['control', 'behaviorRuntime'] as const,
  conversations: () => ['control', 'conversations'] as const,
  conversationMessages: (chatId: string) => ['control', 'conversationMessages', chatId] as const,
  personaGroups: () => ['control', 'personaGroups'] as const,
  personaGroupMembers: (groupId: string) => ['control', 'personaGroupMembers', groupId] as const,
  personaUsers: () => ['control', 'personaUsers'] as const,
  personaResolve: (chatId: string, userId: string, username: string) => ['control', 'personaResolve', chatId, userId, username] as const,
}

export function fetchAdminSession() {
  return apiRequest<AdminSession>('/api/admin/me')
}

export function fetchAuthStatus() {
  return apiRequest<AuthStatus>('/api/auth/status')
}

export function fetchServiceStatus() {
  return apiRequest<ServiceStatus>('/api/service/status')
}

export async function fetchSettings() {
  const payload = await apiRequest<SettingsResponse>('/api/settings')
  return normalizeSettings(payload)
}

export function fetchSoul() {
  return apiRequest<{ content: string }>('/api/soul')
}

export async function fetchLogs() {
  const payload = await apiRequest<{ logs: LogEntry[] }>('/api/logs?limit=200')
  return payload.logs || []
}

export async function fetchAgents() {
  const payload = await apiRequest<{ agents: AgentDefinition[] }>('/api/agents')
  return payload.agents || []
}

export async function fetchVariables() {
  const payload = await apiRequest<{ values: VariableValue[] }>('/api/variables')
  return payload.values || []
}

export function fetchBehaviorRuntime() {
  return apiRequest<BehaviorRuntimeResponse>('/api/behavior/runtime')
}

export function fetchConversations() {
  return apiRequest<ConversationsResponse>('/api/conversations?limit=200')
}

export function fetchConversationMessages(chatId: string) {
  return apiRequest<ConversationMessagesResponse>(`/api/conversations/${encodeURIComponent(chatId)}/messages?limit=100`)
}

export async function fetchPersonaGroups() {
  const payload = await apiRequest<{ groups: PersonaGroup[] }>('/api/persona/groups')
  return payload.groups || []
}

export async function fetchPersonaGroupMembers(groupId: string) {
  const payload = await apiRequest<{ members: PersonaGroupMember[] }>(`/api/persona/groups/${encodeURIComponent(groupId)}/members`)
  return payload.members || []
}

export async function fetchPersonaUsers() {
  const payload = await apiRequest<{ users: PersonaUserProfile[] }>('/api/persona/users')
  return payload.users || []
}

export function fetchPersonaResolve(chatId: string, userId = '', username = '') {
  const q = new URLSearchParams()
  if (chatId.trim()) q.set('chatId', chatId.trim())
  if (userId.trim()) q.set('userId', userId.trim())
  if (username.trim()) q.set('username', username.trim())
  return apiRequest<PersonaResolveResponse>(`/api/persona/resolve?${q.toString()}`)
}
