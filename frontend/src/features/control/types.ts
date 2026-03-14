export type ServiceStatus = {
  running: boolean
  startedAt?: string
  uptimeSec?: number
  lastError?: string
}

export type AuthStatus = {
  authorized: boolean
  configured: boolean
  error?: string
  session_file?: string
}

export type SettingsResponse = {
  keys: string[]
  values: Record<string, string>
}

export type LogEntry = {
  time: string
  level: string
  message: string
  attrs?: Record<string, unknown>
}

export type LoginResponse = {
  ok: boolean
  otpRequested?: boolean
  started?: boolean
  startError?: string
  message?: string
  deliveryType?: string
  nextType?: string
  timeoutSec?: number
  alreadyLogged?: boolean
}

export type UpdateAdminCredentialsResponse = {
  ok: boolean
  reloginRequired?: boolean
  username?: string
}

export type AdminSession = {
  configured: boolean
  authenticated: boolean
  username?: string
}

export type AgentDefinition = {
  id: string
  name: string
  description: string
  intents: string[]
  tools: string[]
  variables: string[]
  visibility?: 'public' | 'private'
  allowUsers?: string[]
  model?: string
  temperature?: number
  body: string
  markdown?: string
  updatedAt?: string
}

export type VariableValue = {
  key: string
  type: 'text' | 'secret'
  value: string
  masked?: boolean
  updatedAt?: string
}

export type ConversationSummary = {
  chatId: string
  chatName: string
  lastMessage: string
  lastMessageAt: string
  unreadIncoming: number
  effectiveMode: 'auto' | 'manual'
  hasManualOverride: boolean
  mode?: 'auto' | 'manual'
  escalatedManual?: boolean
  manualReason?: string
}

export type ConversationMessage = {
  id: number
  chatId: string
  telegramMessageId: string
  senderName: string
  direction: 'me' | 'other_person'
  text: string
  createdAt: string
}

export type ConversationsResponse = {
  globalAutoReplyEnabled: boolean
  conversations: ConversationSummary[]
}

export type ConversationMessagesResponse = {
  chatId: string
  messages: ConversationMessage[]
}

export type ConversationStreamEvent = {
  type?: string
  chatId?: string
  telegramMessageId?: string
  direction?: 'me' | 'other_person'
  text?: string
  mode?: string
  reason?: string
  createdAt?: string
  occurredAt?: string
}

export type BehaviorQuietHoursWindow = {
  start: string
  end: string
}

export type BehaviorToneRules = {
  allow: string[]
  deny: string[]
}

export type BehaviorEscalationPolicy = {
  failureThreshold: number
  triggerKeywords: string[]
}

export type BehaviorPolicy = {
  debounceSeconds: number
  cooldownSeconds: number
  quietHours: BehaviorQuietHoursWindow[]
  maxReplyWords: number
  shortReplyProbability: number
  oneWordReplyProbability: number
  followUpProbability: number
  splitMessageProbability: number
  toneRules: BehaviorToneRules
  escalation: BehaviorEscalationPolicy
}

export type BehaviorRuntimeState = {
  chatId: string
  lastIncomingAt?: string
  lastReplyAt?: string
  debounceUntil?: string
  pendingTriggerMessageId?: string
  pendingPreview?: string
  failureCount: number
  escalatedManual: boolean
  reason?: string
  updatedAt?: string
}

export type BehaviorResponse = {
  path: string
  loadedAt: string
  policy: BehaviorPolicy
  ok?: boolean
  restarted?: boolean
}

export type BehaviorRuntimeResponse = {
  path: string
  loadedAt: string
  policy: BehaviorPolicy
  states: BehaviorRuntimeState[]
}

export type PersonaGroup = {
  id: string
  name: string
  slug: string
  description: string
  path: string
  memberCount: number
  createdAt?: string
  updatedAt?: string
}

export type PersonaGroupMember = {
  id: number
  groupId: string
  userId?: string
  username?: string
  normalizedUsername?: string
  createdAt?: string
  updatedAt?: string
}

export type PersonaUserProfile = {
  id: string
  label: string
  userId?: string
  username?: string
  normalizedUsername?: string
  path: string
  enabled: boolean
  createdAt?: string
  updatedAt?: string
}

export type ResolvedPersona = {
  source: 'soul_only' | 'group' | 'user_override'
  groupId?: string
  groupName?: string
  userProfileId?: string
  userLabel?: string
  composedPrompt: string
}

export type PersonaResolveResponse = {
  chatId?: string
  userId?: string
  username?: string
  resolved: ResolvedPersona
}

export type ConfirmDialogState = {
  open: boolean
  title: string
  description: string
  confirmLabel: string
  confirmVariant: 'default' | 'destructive'
  runner: (() => Promise<void>) | null
  successText: string
}

export type MainPage = 'dashboard' | 'logs' | 'agents' | 'settings'
export type SettingsPage = 'soul' | 'telegram' | 'setting' | 'behavior' | 'variables' | 'user' | 'persona-groups' | 'persona-users'
