import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Activity,
  BellRing,
  Bot,
  Clock3,
  Database,
  KeyRound,
  LayoutDashboard,
  Lock,
  LogOut,
  Menu,
  Play,
  RefreshCw,
  Save,
  Settings2,
  ShieldCheck,
  SlidersHorizontal,
  Square,
  Terminal,
  UserCircle2,
  WandSparkles,
  X,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

type ServiceStatus = {
  running: boolean
  startedAt?: string
  uptimeSec?: number
  lastError?: string
}

type AuthStatus = {
  authorized: boolean
  configured: boolean
  error?: string
  session_file?: string
}

type SettingsResponse = {
  keys: string[]
  values: Record<string, string>
}

type LogEntry = {
  time: string
  level: string
  message: string
  attrs?: Record<string, unknown>
}

type LoginResponse = {
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

type UpdateAdminCredentialsResponse = {
  ok: boolean
  reloginRequired?: boolean
  username?: string
}

type AdminSession = {
  configured: boolean
  authenticated: boolean
  username?: string
}

type AgentDefinition = {
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

type VariableValue = {
  key: string
  type: 'text' | 'secret'
  value: string
  masked?: boolean
  updatedAt?: string
}

type ConversationSummary = {
  chatId: string
  chatName: string
  lastMessage: string
  lastMessageAt: string
  unreadIncoming: number
  effectiveMode: 'auto' | 'manual'
  hasManualOverride: boolean
  mode?: 'auto' | 'manual'
}

type ConversationMessage = {
  id: number
  chatId: string
  telegramMessageId: string
  senderName: string
  direction: 'me' | 'other_person'
  text: string
  createdAt: string
}

type ConversationsResponse = {
  globalAutoReplyEnabled: boolean
  conversations: ConversationSummary[]
}

type ConversationMessagesResponse = {
  chatId: string
  messages: ConversationMessage[]
}

type ConversationStreamEvent = {
  type?: string
  chatId?: string
  telegramMessageId?: string
  direction?: 'me' | 'other_person'
  text?: string
  mode?: string
  createdAt?: string
  occurredAt?: string
}

type ConfirmDialogState = {
  open: boolean
  title: string
  description: string
  confirmLabel: string
  confirmVariant: 'default' | 'destructive'
  runner: (() => Promise<void>) | null
  successText: string
}

type MainPage = 'dashboard' | 'logs' | 'agents' | 'settings'
type SettingsPage = 'soul' | 'telegram' | 'setting' | 'variables' | 'user'

const API_BASE_RAW = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.trim()
const API_BASE = API_BASE_RAW ? API_BASE_RAW.replace(/\/+$/, '') : ''

const ONBOARDING_REQUIRED_KEYS = ['TG_API_ID', 'TG_API_HASH', 'OPENAI_BASE_URL', 'OPENAI_API_KEY', 'OPENAI_MODEL']
const MAIN_VISIBLE_SETTINGS = ['OPENAI_BASE_URL', 'OPENAI_API_KEY', 'OPENAI_MODEL', 'AI_CONTEXT_MESSAGE_LIMIT', 'AUTO_REPLY_DEBOUNCE_SECONDS', 'AUTO_REPLY_ENABLED']

const booleanSettingKeys = new Set(['AUTO_REPLY_ENABLED'])
const numericSettingKeys = new Set(['AI_CONTEXT_MESSAGE_LIMIT', 'AUTO_REPLY_DEBOUNCE_SECONDS'])
const secretSettingKeys = new Set(['OPENAI_API_KEY', 'TG_API_HASH'])

const settingLabels: Record<string, string> = {
  TG_API_ID: 'Telegram API ID',
  TG_API_HASH: 'Telegram API Hash',
  OPENAI_BASE_URL: 'OpenAI Base URL',
  OPENAI_API_KEY: 'OpenAI API Key',
  OPENAI_MODEL: 'OpenAI Model',
  AI_CONTEXT_MESSAGE_LIMIT: 'Context Message Limit',
  AUTO_REPLY_DEBOUNCE_SECONDS: 'Reply Debounce Seconds',
  AUTO_REPLY_ENABLED: 'Auto Reply Enabled',
}

function normalizeSettings(payload: SettingsResponse | undefined): SettingsResponse {
  const normalizedValues: Record<string, string> = {}
  if (payload?.values && typeof payload.values === 'object') {
    for (const [key, value] of Object.entries(payload.values)) {
      if (value === undefined || value === null) {
        continue
      }
      normalizedValues[key] = typeof value === 'string' ? value : String(value)
    }
  }

  const keys = Array.isArray(payload?.keys)
    ? payload.keys
    : Object.keys(normalizedValues)

  return { keys, values: normalizedValues }
}

async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
    ...init,
  })

  const payload = (await response.json().catch(() => ({}))) as { error?: string }
  if (!response.ok) {
    throw new Error(payload.error || `Request failed: ${response.status}`)
  }
  return payload as T
}

function formatUptime(seconds?: number) {
  if (!seconds || seconds < 1) return '0s'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (h > 0) return `${h}h ${m}m ${s}s`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

function formatLocalTime(ts?: string) {
  if (!ts) return 'N/A'
  const date = new Date(ts)
  if (Number.isNaN(date.getTime())) return 'N/A'
  return date.toLocaleString()
}

function normalizePhoneWithPlus(raw: string) {
  const value = raw.trim()
  if (!value) return ''
  if (value.startsWith('+')) return value
  return `+${value}`
}

function readInitialPhone() {
  if (typeof window === 'undefined') return ''
  return window.localStorage.getItem('tele_auto_last_phone') || ''
}

function normalizeAllowUserToken(raw: string) {
  const trimmed = raw.trim()
  if (!trimmed) return ''
  const withoutAt = trimmed.startsWith('@') ? trimmed.slice(1) : trimmed
  if (!withoutAt) return ''
  if (/^\d+$/.test(withoutAt)) {
    const canonical = withoutAt.replace(/^0+(?=\d)/, '')
    return canonical || '0'
  }
  return withoutAt.toLowerCase()
}

function normalizeAllowUsersList(values: string[]) {
  const next = values
    .map((value) => normalizeAllowUserToken(value))
    .filter(Boolean)
  return Array.from(new Set(next))
}

function parseAllowUsersInput(raw: string) {
  return normalizeAllowUsersList(raw.split(/[\n,]/g))
}

function trimNotificationBody(text?: string) {
  const normalized = (text || '').trim()
  if (normalized.length <= 140) {
    return normalized || 'New message received'
  }
  return `${normalized.slice(0, 137)}...`
}

function logNotificationDebug(stage: string, details?: Record<string, unknown>) {
  const payload = details ? { stage, ...details } : { stage }
  console.info('[tele-auto][notification-sound]', payload)
}

export default function App() {
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')

  const [adminSession, setAdminSession] = useState<AdminSession>({ configured: false, authenticated: false })
  const [adminUsername, setAdminUsername] = useState('')
  const [adminPassword, setAdminPassword] = useState('')
  const [currentAdminPassword, setCurrentAdminPassword] = useState('')
  const [newAdminUsername, setNewAdminUsername] = useState('')
  const [newAdminPassword, setNewAdminPassword] = useState('')
  const [confirmAdminPassword, setConfirmAdminPassword] = useState('')

  const [authStatus, setAuthStatus] = useState<AuthStatus>({ authorized: false, configured: false })
  const [serviceStatus, setServiceStatus] = useState<ServiceStatus>({ running: false })
  const [settings, setSettings] = useState<SettingsResponse>({ keys: [], values: {} })
  const [savedSettings, setSavedSettings] = useState<SettingsResponse>({ keys: [], values: {} })
  const [soulText, setSoulText] = useState('')
  const [soulLoadedAt, setSoulLoadedAt] = useState('')
  const [soulSavedAt, setSoulSavedAt] = useState('')

  const [phone, setPhone] = useState(readInitialPhone)
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')

  const [logs, setLogs] = useState<LogEntry[]>([])
  const [logMode, setLogMode] = useState<'all' | 'orchestrator'>('all')
  const [globalAutoReplyEnabled, setGlobalAutoReplyEnabled] = useState(true)
  const [conversations, setConversations] = useState<ConversationSummary[]>([])
  const [selectedChatId, setSelectedChatId] = useState('')
  const [messagesByChat, setMessagesByChat] = useState<Record<string, ConversationMessage[]>>({})
  const [manualReplyText, setManualReplyText] = useState('')

  const [agents, setAgents] = useState<AgentDefinition[]>([])
  const [editingAgentID, setEditingAgentID] = useState('')
  const [allowUsersInput, setAllowUsersInput] = useState('')
  const [agentForm, setAgentForm] = useState<AgentDefinition>({
    id: '',
    name: '',
    description: '',
    intents: [],
    tools: ['api_call'],
    variables: [],
    visibility: 'public',
    allowUsers: [],
    model: '',
    temperature: 0.35,
    body: '',
  })

  const [variables, setVariables] = useState<VariableValue[]>([])
  const [variableForm, setVariableForm] = useState<VariableValue>({ key: '', type: 'text', value: '' })
  const [confirmDialog, setConfirmDialog] = useState<ConfirmDialogState>({
    open: false,
    title: 'Please Confirm',
    description: '',
    confirmLabel: 'Confirm',
    confirmVariant: 'default',
    runner: null,
    successText: '',
  })
  const confirmResolverRef = useRef<((ok: boolean) => void) | null>(null)
  const conversationsRef = useRef<ConversationSummary[]>([])
  const selectedChatIdRef = useRef('')
  const globalAutoReplyEnabledRef = useRef(true)
  const notifiedEventKeysRef = useRef<Set<string>>(new Set())
  const conversationThreadRef = useRef<HTMLDivElement | null>(null)
  const lastRenderedMessageKeyRef = useRef('')
  const notificationAudioContextRef = useRef<AudioContext | null>(null)
  const [activePage, setActivePage] = useState<MainPage>('dashboard')
  const [activeSettingsPage, setActiveSettingsPage] = useState<SettingsPage>('setting')
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const [notificationPermission, setNotificationPermission] = useState<NotificationPermission | 'unsupported'>(() => {
    if (typeof window === 'undefined' || !('Notification' in window)) {
      return 'unsupported'
    }
    return window.Notification.permission
  })

  const settingsValues = settings && settings.values && typeof settings.values === 'object' ? settings.values : {}
  const savedSettingsValues = savedSettings && savedSettings.values && typeof savedSettings.values === 'object' ? savedSettings.values : {}
  const settingsKeys = settings && Array.isArray(settings.keys) ? settings.keys : []
  const requiredMissing = useMemo(
    () => ONBOARDING_REQUIRED_KEYS.filter((key) => !(savedSettingsValues[key] || '').trim()),
    [savedSettingsValues],
  )

  const needsConfig = requiredMissing.length > 0
  const needsAuth = !authStatus.authorized
  const needsAppLogin = !adminSession.authenticated
  const showOnboarding = !loading && !needsAppLogin && needsConfig
  const canManageWorker = authStatus.authorized
  const telegramConnection = authStatus.authorized
    ? { label: 'Connected', dotClass: 'bg-emerald-500', badgeVariant: 'default' as const }
    : authStatus.configured
      ? { label: 'Pending Verify', dotClass: 'bg-amber-400', badgeVariant: 'secondary' as const }
      : { label: 'Disconnected', dotClass: 'bg-red-500', badgeVariant: 'destructive' as const }
  const telegramPhoneLabel = normalizePhoneWithPlus(phone) || 'Not set'
  const soulCharacterCount = soulText.length

  const handleRequestError = (err: unknown) => {
    const text = (err as Error).message
    if (text.toLowerCase().includes('unauthorized')) {
      setAdminSession((prev) => ({ ...prev, authenticated: false }))
    }
    setMessage(text)
  }

  const syncNotificationPermission = () => {
    if (typeof window === 'undefined' || !('Notification' in window)) {
      setNotificationPermission('unsupported')
      return
    }
    setNotificationPermission(window.Notification.permission)
  }

  const ensureNotificationAudioContext = async () => {
    if (typeof window === 'undefined' || typeof window.AudioContext === 'undefined') {
      logNotificationDebug('audio-context-unsupported')
      return null
    }
    if (!notificationAudioContextRef.current) {
      notificationAudioContextRef.current = new window.AudioContext()
      logNotificationDebug('audio-context-created', { state: notificationAudioContextRef.current.state })
    }
    if (notificationAudioContextRef.current.state === 'suspended') {
      try {
        await notificationAudioContextRef.current.resume()
        logNotificationDebug('audio-context-resumed', { state: notificationAudioContextRef.current.state })
      } catch {
        logNotificationDebug('audio-context-resume-failed', { state: notificationAudioContextRef.current.state })
        return notificationAudioContextRef.current
      }
    }
    return notificationAudioContextRef.current
  }

  const playNotificationSound = async () => {
    const audioContext = await ensureNotificationAudioContext()
    if (!audioContext || audioContext.state !== 'running') {
      logNotificationDebug('sound-skipped', {
        reason: 'audio-context-not-running',
        state: audioContext?.state || 'missing',
      })
      return
    }

    const startAt = audioContext.currentTime
    const oscillator = audioContext.createOscillator()
    const gain = audioContext.createGain()

    oscillator.type = 'sine'
    oscillator.frequency.setValueAtTime(880, startAt)
    oscillator.frequency.exponentialRampToValueAtTime(660, startAt + 0.18)

    gain.gain.setValueAtTime(0.0001, startAt)
    gain.gain.exponentialRampToValueAtTime(0.12, startAt + 0.02)
    gain.gain.exponentialRampToValueAtTime(0.0001, startAt + 0.26)

    oscillator.connect(gain)
    gain.connect(audioContext.destination)
    oscillator.start(startAt)
    oscillator.stop(startAt + 0.28)
    logNotificationDebug('sound-played', { state: audioContext.state, at: startAt })
  }

  const rememberNotifiedEvent = (key: string) => {
    if (!key) return
    const next = notifiedEventKeysRef.current
    next.add(key)
    if (next.size > 400) {
      const oldest = next.values().next().value
      if (oldest) {
        next.delete(oldest)
      }
    }
  }

  const requestBrowserNotifications = async () => {
    if (typeof window === 'undefined' || !('Notification' in window)) {
      logNotificationDebug('permission-request-skipped', { reason: 'notification-unsupported' })
      setMessage('This browser does not support desktop notifications.')
      return
    }
    try {
      await ensureNotificationAudioContext()
      const permission = await window.Notification.requestPermission()
      logNotificationDebug('permission-request-result', { permission })
      setNotificationPermission(permission)
      if (permission === 'granted') {
        setMessage('Browser notifications enabled for manual chats.')
        return
      }
      if (permission === 'denied') {
        setMessage('Browser notifications are blocked. Allow them in browser site settings.')
        return
      }
      setMessage('Browser notification permission was dismissed.')
    } catch {
      logNotificationDebug('permission-request-failed')
      setMessage('Unable to request browser notification permission.')
    }
  }

  const notifyForManualConversation = (event: ConversationStreamEvent, connectedAt: number) => {
    if (typeof window === 'undefined' || !('Notification' in window)) {
      logNotificationDebug('notify-skipped', { reason: 'notification-unsupported' })
      return
    }
    if (window.Notification.permission !== 'granted') {
      logNotificationDebug('notify-skipped', {
        reason: 'permission-not-granted',
        permission: window.Notification.permission,
      })
      return
    }
    if (event.type !== 'message_created' || event.direction !== 'other_person') {
      return
    }

    const eventTime = Date.parse(event.occurredAt || event.createdAt || '')
    if (Number.isFinite(eventTime) && eventTime < connectedAt - 5000) {
      logNotificationDebug('notify-skipped', {
        reason: 'event-before-connection',
        chatId: event.chatId || '',
      })
      return
    }

    const chatID = (event.chatId || '').trim()
    if (!chatID) {
      logNotificationDebug('notify-skipped', { reason: 'missing-chat-id' })
      return
    }
    const conversation = conversationsRef.current.find((item) => item.chatId === chatID)
    const isManual = !globalAutoReplyEnabledRef.current || conversation?.effectiveMode === 'manual'
    if (!isManual) {
      logNotificationDebug('notify-skipped', {
        reason: 'conversation-not-manual',
        chatId: chatID,
        effectiveMode: conversation?.effectiveMode || 'unknown',
      })
      return
    }

    const dedupeKey = `${chatID}:${(event.telegramMessageId || event.occurredAt || event.createdAt || '').trim()}`
    const isActiveVisibleChat =
      typeof document !== 'undefined' &&
      document.visibilityState === 'visible' &&
      selectedChatIdRef.current === chatID
    if (isActiveVisibleChat) {
      const activeChatSoundKey = `${dedupeKey || `${chatID}:active-visible`}:sound-only`
      if (notifiedEventKeysRef.current.has(activeChatSoundKey)) {
        logNotificationDebug('notify-skipped', {
          reason: 'duplicate-active-chat-sound',
          chatId: chatID,
          dedupeKey: activeChatSoundKey,
        })
        return
      }
      rememberNotifiedEvent(activeChatSoundKey)
      logNotificationDebug('notify-sound-only', {
        reason: 'active-visible-chat',
        chatId: chatID,
      })
      void playNotificationSound()
      return
    }
    if (!dedupeKey || notifiedEventKeysRef.current.has(dedupeKey)) {
      logNotificationDebug('notify-skipped', {
        reason: 'duplicate-event',
        chatId: chatID,
        dedupeKey,
      })
      return
    }
    rememberNotifiedEvent(dedupeKey)
    logNotificationDebug('notify-triggered', {
      chatId: chatID,
      dedupeKey,
      permission: window.Notification.permission,
    })
    void playNotificationSound()

    const title = conversation?.chatName || chatID
    const notification = new window.Notification(title, {
      body: trimNotificationBody(event.text),
      tag: dedupeKey,
    })
    notification.onclick = () => {
      window.focus()
      setActivePage('dashboard')
      setSelectedChatId(chatID)
      notification.close()
    }
  }

  const refreshConversations = async () => {
    const convPayload = await apiRequest<ConversationsResponse>('/api/conversations?limit=200')
    setGlobalAutoReplyEnabled(convPayload.globalAutoReplyEnabled)
    setConversations(convPayload.conversations || [])
    setSelectedChatId((prev) => {
      if (prev && (convPayload.conversations || []).some((c) => c.chatId === prev)) {
        return prev
      }
      return (convPayload.conversations || [])[0]?.chatId || ''
    })
  }

  const appendConversationMessage = (event: ConversationStreamEvent) => {
    const chatId = (event.chatId || '').trim()
    const telegramMessageId = (event.telegramMessageId || '').trim()
    const direction = event.direction
    const createdAt = (event.createdAt || event.occurredAt || new Date().toISOString()).trim()
    const text = event.text || ''
    if (!chatId || !telegramMessageId || !direction) {
      return
    }

    const conversation = conversationsRef.current.find((item) => item.chatId === chatId)
    const senderName = direction === 'me' ? 'me' : conversation?.chatName || chatId

    setMessagesByChat((prev) => {
      const current = prev[chatId] || []
      if (current.some((item) => item.telegramMessageId === telegramMessageId)) {
        return prev
      }
      const syntheticID = Date.parse(createdAt)
      const nextMessage: ConversationMessage = {
        id: Number.isFinite(syntheticID) ? syntheticID : Date.now(),
        chatId,
        telegramMessageId,
        senderName,
        direction,
        text,
        createdAt,
      }
      return {
        ...prev,
        [chatId]: [...current, nextMessage].sort((a, b) => {
          if (a.createdAt === b.createdAt) {
            return a.id - b.id
          }
          return new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()
        }),
      }
    })
  }

  const refreshState = async () => {
    setLoading(true)
    try {
      const admin = await apiRequest<AdminSession>('/api/admin/me')
      setAdminSession(admin)
      if (admin.username && !newAdminUsername.trim()) {
        setNewAdminUsername(admin.username)
      }

      if (!admin.authenticated) {
        setLoading(false)
        return
      }

      const [auth, service, settingsData, soul, logPayload, agentsPayload, varsPayload] = await Promise.all([
        apiRequest<AuthStatus>('/api/auth/status'),
        apiRequest<ServiceStatus>('/api/service/status'),
        apiRequest<SettingsResponse>('/api/settings'),
        apiRequest<{ content: string }>('/api/soul'),
        apiRequest<{ logs: LogEntry[] }>('/api/logs?limit=200'),
        apiRequest<{ agents: AgentDefinition[] }>('/api/agents'),
        apiRequest<{ values: VariableValue[] }>('/api/variables'),
      ])
      setAuthStatus(auth)
      setServiceStatus(service)
      const normalizedSettings = normalizeSettings(settingsData)
      setSettings(normalizedSettings)
      setSavedSettings(normalizedSettings)
      setSoulText(soul.content || '')
      setSoulLoadedAt(new Date().toISOString())
      setLogs(logPayload.logs || [])
      setAgents(agentsPayload.agents || [])
      setVariables(varsPayload.values || [])
      await refreshConversations()
    } catch (err) {
      handleRequestError(err)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refreshState()
  }, [])

  useEffect(() => {
    conversationsRef.current = conversations
  }, [conversations])

  useEffect(() => {
    selectedChatIdRef.current = selectedChatId
  }, [selectedChatId])

  useEffect(() => {
    globalAutoReplyEnabledRef.current = globalAutoReplyEnabled
  }, [globalAutoReplyEnabled])

  useEffect(() => {
    syncNotificationPermission()
    if (typeof window === 'undefined') {
      return
    }
    const handleWindowFocus = () => syncNotificationPermission()
    const unlockNotificationAudio = () => {
      void ensureNotificationAudioContext()
    }
    window.addEventListener('focus', handleWindowFocus)
    window.addEventListener('pointerdown', unlockNotificationAudio)
    window.addEventListener('keydown', unlockNotificationAudio)
    return () => {
      window.removeEventListener('focus', handleWindowFocus)
      window.removeEventListener('pointerdown', unlockNotificationAudio)
      window.removeEventListener('keydown', unlockNotificationAudio)
    }
  }, [])

  useEffect(() => {
    if (!adminSession.authenticated) {
      return
    }

    const eventSource = new EventSource(`${API_BASE}/api/logs/stream`, { withCredentials: true })
    eventSource.addEventListener('log', (event) => {
      const parsed = JSON.parse((event as MessageEvent).data) as LogEntry
      setLogs((prev) => {
        const next = [...prev, parsed]
        if (next.length > 400) return next.slice(next.length - 400)
        return next
      })
    })
    eventSource.onerror = () => {
      setMessage('Log stream disconnected. Retry by refreshing state.')
    }
    return () => {
      eventSource.close()
    }
  }, [adminSession.authenticated])

  useEffect(() => {
    if (typeof window === 'undefined') {
      return
    }
    const normalized = normalizePhoneWithPlus(phone)
    if (normalized) {
      window.localStorage.setItem('tele_auto_last_phone', normalized)
    } else {
      window.localStorage.removeItem('tele_auto_last_phone')
    }
  }, [phone])

  useEffect(() => {
    if (!adminSession.authenticated || !selectedChatId) {
      return
    }
    void loadConversationMessages(selectedChatId)
  }, [adminSession.authenticated, selectedChatId])

  useEffect(() => {
    if (!adminSession.authenticated) {
      return
    }
    const connectedAt = Date.now()
    const eventSource = new EventSource(`${API_BASE}/api/conversations/stream`, { withCredentials: true })
    eventSource.addEventListener('conversation', (event) => {
      let parsed: ConversationStreamEvent | null = null
      void refreshConversations()
      try {
        parsed = JSON.parse((event as MessageEvent).data) as ConversationStreamEvent
        if (parsed.type === 'message_created') {
          appendConversationMessage(parsed)
        }
        notifyForManualConversation(parsed, connectedAt)
        if (parsed.chatId && parsed.chatId === selectedChatIdRef.current) {
          void loadConversationMessages(parsed.chatId)
        }
      } catch {
        if (selectedChatIdRef.current) {
          void loadConversationMessages(selectedChatIdRef.current)
        }
      }
    })
    eventSource.onerror = () => {
      setMessage('Conversation stream disconnected. Retry by refreshing state.')
    }
    return () => eventSource.close()
  }, [adminSession.authenticated])

  useEffect(() => {
    if (typeof document === 'undefined') {
      return
    }
    if (mobileNavOpen) {
      document.body.style.overflow = 'hidden'
      return () => {
        document.body.style.overflow = ''
      }
    }
    document.body.style.overflow = ''
  }, [mobileNavOpen])

  useEffect(() => {
    setMobileNavOpen(false)
  }, [activePage, activeSettingsPage])

  const currentPageLabel = activePage === 'settings'
    ? `Settings / ${activeSettingsPage === 'soul'
      ? 'Soul'
      : activeSettingsPage === 'telegram'
        ? 'Telegram'
        : activeSettingsPage === 'setting'
          ? 'Setting'
          : activeSettingsPage === 'variables'
            ? 'Global Variables'
            : 'User'}`
    : activePage === 'agents'
      ? 'Agents'
      : activePage === 'logs'
        ? 'Logs'
        : 'Dashboard'

  const runAction = async (runner: () => Promise<void>, successText: string) => {
    setBusy(true)
    setMessage('')
    let ok = false
    try {
      await runner()
      setMessage(successText)
      await refreshState()
      ok = true
    } catch (err) {
      handleRequestError(err)
    } finally {
      setBusy(false)
    }
    return ok
  }

  const runConfirmedAction = async (
    confirmText: string,
    runner: () => Promise<void>,
    successText: string,
    options?: {
      title?: string
      confirmLabel?: string
      confirmVariant?: 'default' | 'destructive'
    },
  ) => {
    return await new Promise<boolean>((resolve) => {
      confirmResolverRef.current = resolve
      setConfirmDialog({
        open: true,
        title: options?.title || 'Please Confirm',
        description: confirmText,
        confirmLabel: options?.confirmLabel || 'Confirm',
        confirmVariant: options?.confirmVariant || 'default',
        runner,
        successText,
      })
    })
  }

  const closeConfirmDialog = (ok: boolean) => {
    setConfirmDialog((prev) => ({ ...prev, open: false }))
    const resolver = confirmResolverRef.current
    confirmResolverRef.current = null
    if (resolver) {
      resolver(ok)
    }
  }

  const handleConfirmAction = async () => {
    if (!confirmDialog.runner) {
      closeConfirmDialog(false)
      return
    }
    const ok = await runAction(confirmDialog.runner, confirmDialog.successText)
    closeConfirmDialog(ok)
  }

  const saveSettingsSubset = async (keys: string[], successText: string) => {
    const values: Record<string, string> = {}
    for (const key of keys) {
      values[key] = settingsValues[key] ?? ''
    }

    if (keys.includes('AI_CONTEXT_MESSAGE_LIMIT')) {
      const raw = (values.AI_CONTEXT_MESSAGE_LIMIT || '').trim()
      if (!/^\d+$/.test(raw) || Number(raw) <= 0) {
        setMessage('Context Message Limit must be a positive number.')
        return
      }
    }
    if (keys.includes('AUTO_REPLY_DEBOUNCE_SECONDS')) {
      const raw = (values.AUTO_REPLY_DEBOUNCE_SECONDS || '').trim()
      if (!/^\d+$/.test(raw)) {
        setMessage('Reply Debounce Seconds must be 0 or a positive number.')
        return
      }
    }

    await runAction(
      () => apiRequest('/api/settings', { method: 'PUT', body: JSON.stringify({ values }) }).then(() => undefined),
      successText,
    )
  }

  const requestOtp = async () => {
    const phoneForAuth = normalizePhoneWithPlus(phone)
    if (!phoneForAuth) {
      setMessage('Phone is required.')
      return
    }
    setPhone(phoneForAuth)

    setBusy(true)
    setMessage('')
    try {
      const result = await apiRequest<LoginResponse>('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify({ phone: phoneForAuth }),
      })
      if (result.alreadyLogged) {
        setMessage(result.message || 'Already logged in.')
      } else {
        const delivery = result.deliveryType ? ` via ${result.deliveryType}` : ''
        const next = result.nextType ? `, fallback: ${result.nextType}` : ''
        const ttl = result.timeoutSec ? ` (timeout ~${result.timeoutSec}s)` : ''
        setMessage(result.message || `OTP requested${delivery}${next}${ttl}. Check Telegram app first.`)
      }
      await refreshState()
    } catch (err) {
      handleRequestError(err)
    } finally {
      setBusy(false)
    }
  }

  const verifyLogin = async () => {
    const phoneForAuth = normalizePhoneWithPlus(phone)
    if (!phoneForAuth) {
      setMessage('Phone is required for verify login.')
      return
    }
    if (!code.trim()) {
      setMessage('OTP code is required for verify login.')
      return
    }
    setPhone(phoneForAuth)

    await runAction(
      () =>
        apiRequest<LoginResponse>('/api/auth/login', {
          method: 'POST',
          body: JSON.stringify({ phone: phoneForAuth, code, password }),
        }).then(() => undefined),
      'Login verified and session saved.',
    )
  }

  const clearTelegramVerifyForm = () => {
    setCode('')
    setPassword('')
  }

  const saveSoul = async () => {
    await runAction(
      () => apiRequest('/api/soul', { method: 'PUT', body: JSON.stringify({ content: soulText }) }).then(() => undefined),
      'SOUL updated',
    )
    setSoulSavedAt(new Date().toISOString())
  }

  const updateSetting = (key: string, value: string) => {
    setSettings((prev) => ({
      ...prev,
      keys: prev.keys.includes(key) ? prev.keys : [...prev.keys, key],
      values: {
        ...prev.values,
        [key]: value,
      },
    }))
  }

  const resetAgentForm = () => {
    setEditingAgentID('')
    setAllowUsersInput('')
    setAgentForm({
      id: '',
      name: '',
      description: '',
      intents: [],
      tools: ['api_call'],
      variables: [],
      visibility: 'public',
      allowUsers: [],
      model: '',
      temperature: 0.35,
      body: '',
    })
  }

  const startEditAgent = (agent: AgentDefinition) => {
    const existingAllowUsers = agent.allowUsers || []
    setAllowUsersInput(
      existingAllowUsers
        .map((value) => (/^\d+$/.test(value) ? value : `@${value}`))
        .join('\n'),
    )
    setEditingAgentID(agent.id)
    setAgentForm({
      ...agent,
      intents: agent.intents || [],
      tools: ['api_call'],
      variables: agent.variables || [],
      visibility: agent.visibility || 'public',
      allowUsers: existingAllowUsers,
      body: agent.body || '',
      temperature: agent.temperature || 0.35,
    })
  }

  const saveAgent = async () => {
    if (!agentForm.id.trim() || !agentForm.name.trim() || !agentForm.body.trim()) {
      setMessage('Agent id, name, and body are required.')
      return
    }
    const normalizedIntents = agentForm.intents.map((v) => v.trim()).filter(Boolean)
    const normalizedVariables = agentForm.variables.map((v) => v.trim().toUpperCase()).filter(Boolean)
    const normalizedVisibility = agentForm.visibility === 'private' ? 'private' : 'public'
    const normalizedAllowUsers = parseAllowUsersInput(allowUsersInput)
    if (normalizedVisibility === 'private' && normalizedAllowUsers.length === 0) {
      setMessage('Allow users is required when visibility is private.')
      return
    }
    const payload = {
      id: agentForm.id.trim(),
      name: agentForm.name.trim(),
      description: agentForm.description.trim(),
      intents: normalizedIntents,
      tools: ['api_call'],
      variables: normalizedVariables,
      visibility: normalizedVisibility,
      allowUsers: normalizedAllowUsers,
      model: (agentForm.model || '').trim(),
      body: agentForm.body.trim(),
      temperature: Number(agentForm.temperature || 0.35),
    }
    const isEdit = editingAgentID === payload.id
    const path = isEdit ? `/api/agents/${payload.id}` : '/api/agents'
    const method = isEdit ? 'PUT' : 'POST'
    const ok = await runAction(
      () => apiRequest(path, { method, body: JSON.stringify(payload) }).then(() => undefined),
      isEdit ? 'Agent updated' : 'Agent created',
    )
    if (ok) {
      resetAgentForm()
    }
  }

  const deleteAgent = async (id: string) => {
    await runConfirmedAction(
      `Delete agent "${id}"?`,
      () => apiRequest(`/api/agents/${id}`, { method: 'DELETE' }).then(() => undefined),
      'Agent deleted',
      { title: 'Delete Agent', confirmLabel: 'Delete', confirmVariant: 'destructive' },
    )
    if (editingAgentID === id) {
      resetAgentForm()
    }
  }

  const saveVariable = async () => {
    const key = variableForm.key.trim().toUpperCase()
    if (!key) {
      setMessage('Variable key is required.')
      return
    }
    if (!/^[A-Z0-9_]{2,64}$/.test(key)) {
      setMessage('Variable key must match A-Z, 0-9, _ and be 2-64 chars.')
      return
    }
    if (!variableForm.value.trim()) {
      setMessage('Variable value is required.')
      return
    }
    await runAction(
      () =>
        apiRequest('/api/variables', {
          method: 'PUT',
          body: JSON.stringify({
            values: [{ key, type: variableForm.type, value: variableForm.value }],
          }),
        }).then(() => undefined),
      'Variable saved',
    )
    setVariableForm({ key: '', type: 'text', value: '' })
  }

  const loadConversationMessages = async (chatId: string) => {
    if (!chatId) return
    try {
      const payload = await apiRequest<ConversationMessagesResponse>(`/api/conversations/${encodeURIComponent(chatId)}/messages?limit=100`)
      setMessagesByChat((prev) => {
        const merged = new Map<string, ConversationMessage>()
        for (const item of prev[chatId] || []) {
          merged.set(item.telegramMessageId, item)
        }
        for (const item of payload.messages || []) {
          merged.set(item.telegramMessageId, item)
        }
        const next = Array.from(merged.values()).sort((a, b) => {
          if (a.createdAt === b.createdAt) {
            return a.id - b.id
          }
          return new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()
        })
        return { ...prev, [chatId]: next }
      })
    } catch (err) {
      handleRequestError(err)
    }
  }

  const setConversationMode = async (chatId: string, mode: 'auto' | 'manual') => {
    setBusy(true)
    setMessage('')
    try {
      await apiRequest(`/api/conversations/${encodeURIComponent(chatId)}/mode`, {
        method: 'PUT',
        body: JSON.stringify({ mode }),
      })
      setMessage(`Conversation mode set to ${mode}`)
      await refreshConversations()
    } catch (err) {
      handleRequestError(err)
    } finally {
      setBusy(false)
    }
  }

  const sendManualReply = async () => {
    const chatId = selectedChatId
    const text = manualReplyText.trim()
    if (!chatId) {
      setMessage('Select a conversation first.')
      return
    }
    if (!text) {
      setMessage('Reply text is required.')
      return
    }
    setBusy(true)
    setMessage('')
    try {
      await apiRequest(`/api/conversations/${encodeURIComponent(chatId)}/messages`, {
        method: 'POST',
        body: JSON.stringify({ text }),
      })
      setMessage('Manual reply sent')
      setManualReplyText('')
      await Promise.all([refreshConversations(), loadConversationMessages(chatId)])
    } catch (err) {
      handleRequestError(err)
    } finally {
      setBusy(false)
    }
  }

  const deleteVariable = async (key: string) => {
    await runConfirmedAction(
      `Delete variable "${key}"?`,
      () => apiRequest(`/api/variables/${encodeURIComponent(key)}`, { method: 'DELETE' }).then(() => undefined),
      'Variable deleted',
      { title: 'Delete Variable', confirmLabel: 'Delete', confirmVariant: 'destructive' },
    )
    if (variableForm.key.trim().toUpperCase() === key) {
      setVariableForm({ key: '', type: 'text', value: '' })
    }
  }

  const filteredLogs = useMemo(() => {
    if (logMode === 'all') return logs
    return logs.filter((entry) => {
      const blob = `${entry.message} ${JSON.stringify(entry.attrs || {})}`.toLowerCase()
      return blob.includes('orchestrator') || blob.includes('agent_tool_call')
    })
  }, [logs, logMode])
  const selectedConversation = useMemo(
    () => conversations.find((item) => item.chatId === selectedChatId),
    [conversations, selectedChatId],
  )
  const activeMessages = useMemo(
    () => (selectedChatId ? messagesByChat[selectedChatId] || [] : []),
    [messagesByChat, selectedChatId],
  )

  useEffect(() => {
    const latest = activeMessages[activeMessages.length - 1]
    const latestKey = latest ? `${selectedChatId}:${latest.telegramMessageId}` : `${selectedChatId}:empty`
    if (latestKey === lastRenderedMessageKeyRef.current) {
      return
    }
    lastRenderedMessageKeyRef.current = latestKey
    if (typeof window === 'undefined') {
      return
    }
    window.requestAnimationFrame(() => {
      const el = conversationThreadRef.current
      if (!el) {
        return
      }
      el.scrollTop = el.scrollHeight
    })
  }, [activeMessages, selectedChatId])

  const loginAdmin = async () => {
    if (!adminUsername.trim() || !adminPassword) {
      setMessage('Username and password are required.')
      return
    }

    setBusy(true)
    setMessage('')
    try {
      await apiRequest<{ ok: boolean }>('/api/admin/login', {
        method: 'POST',
        body: JSON.stringify({ username: adminUsername, password: adminPassword }),
      })
      setAdminPassword('')
      setMessage('Login successful.')
      await refreshState()
    } catch (err) {
      handleRequestError(err)
    } finally {
      setBusy(false)
    }
  }

  const logoutAdmin = async () => {
    await runAction(
      () => apiRequest('/api/admin/logout', { method: 'POST' }).then(() => undefined),
      'Signed out.',
    )
  }

  const updateAdminCredentials = async () => {
    const nextUsername = newAdminUsername.trim()
    const nextPassword = newAdminPassword

    if (!currentAdminPassword) {
      setMessage('Current dashboard password is required.')
      return
    }
    if (!nextUsername) {
      setMessage('New username is required.')
      return
    }
    if (nextPassword || confirmAdminPassword) {
      if (nextPassword !== confirmAdminPassword) {
        setMessage('New password and confirm password do not match.')
        return
      }
      if (nextPassword.length < 4) {
        setMessage('New password must be at least 4 characters.')
        return
      }
    }
    if (nextUsername === (adminSession.username || '') && !nextPassword) {
      setMessage('No account changes to save.')
      return
    }

    setBusy(true)
    setMessage('')
    try {
      const result = await apiRequest<UpdateAdminCredentialsResponse>('/api/admin/credentials', {
        method: 'PUT',
        body: JSON.stringify({
          currentPassword: currentAdminPassword,
          newUsername: nextUsername,
          newPassword: nextPassword,
        }),
      })
      setCurrentAdminPassword('')
      setNewAdminPassword('')
      setConfirmAdminPassword('')
      setNewAdminUsername(result.username || nextUsername)
      setMessage('Account updated. Please sign in again.')
      setAdminSession((prev) => ({
        ...prev,
        authenticated: false,
        username: result.username || nextUsername,
      }))
    } catch (err) {
      handleRequestError(err)
    } finally {
      setBusy(false)
    }
  }

  const logoutTelegram = async () => {
    if (!authStatus.authorized) {
      setMessage('Telegram is already logged out.')
      return
    }
    await runAction(
      () => apiRequest('/api/auth/logout', { method: 'POST' }).then(() => undefined),
      'Telegram logged out',
    )
  }

  const openPage = (page: MainPage) => {
    setActivePage(page)
    setMobileNavOpen(false)
  }

  const openSettingsPage = (page: SettingsPage) => {
    setActivePage('settings')
    setActiveSettingsPage(page)
    setMobileNavOpen(false)
  }

  const renderNavigation = () => (
    <>
      <p className="px-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Navigation</p>
      <div className="space-y-2">
        <Button
          size="sm"
          variant={activePage === 'dashboard' ? 'default' : 'outline'}
          className="w-full justify-start"
          onClick={() => openPage('dashboard')}
        >
          <LayoutDashboard className="size-4" /> Dashboard
        </Button>
        <Button
          size="sm"
          variant={activePage === 'logs' ? 'default' : 'outline'}
          className="w-full justify-start"
          onClick={() => openPage('logs')}
        >
          <Terminal className="size-4" /> Logs
        </Button>
        <Button
          size="sm"
          variant={activePage === 'agents' ? 'default' : 'outline'}
          className="w-full justify-start"
          onClick={() => openPage('agents')}
        >
          <Bot className="size-4" /> Agents
        </Button>
        <Button
          size="sm"
          variant={activePage === 'settings' ? 'default' : 'outline'}
          className="w-full justify-start"
          onClick={() => openPage('settings')}
        >
          <Settings2 className="size-4" /> Setting
        </Button>
        {activePage === 'settings' ? (
          <div className="ml-3 space-y-1 border-l border-border/60 pl-3">
            <Button size="sm" variant={activeSettingsPage === 'soul' ? 'secondary' : 'ghost'} className="w-full justify-start" onClick={() => openSettingsPage('soul')}>
              <WandSparkles className="size-4" /> Soul
            </Button>
            <Button size="sm" variant={activeSettingsPage === 'telegram' ? 'secondary' : 'ghost'} className="w-full justify-start" onClick={() => openSettingsPage('telegram')}>
              <Bot className="size-4" /> Telegram
            </Button>
            <Button size="sm" variant={activeSettingsPage === 'setting' ? 'secondary' : 'ghost'} className="w-full justify-start" onClick={() => openSettingsPage('setting')}>
              <SlidersHorizontal className="size-4" /> Setting
            </Button>
            <Button size="sm" variant={activeSettingsPage === 'variables' ? 'secondary' : 'ghost'} className="w-full justify-start" onClick={() => openSettingsPage('variables')}>
              <Database className="size-4" /> Global Variables
            </Button>
            <Button size="sm" variant={activeSettingsPage === 'user' ? 'secondary' : 'ghost'} className="w-full justify-start" onClick={() => openSettingsPage('user')}>
              <UserCircle2 className="size-4" /> User
            </Button>
          </div>
        ) : null}
      </div>
    </>
  )

  if (loading) {
    return (
      <main className="min-h-screen bg-background px-4 py-10 text-foreground">
        <div className="mx-auto max-w-5xl rounded-xl border border-border/60 bg-card p-6 text-sm text-muted-foreground">
          Loading control center...
        </div>
      </main>
    )
  }

  if (needsAppLogin) {
    return (
      <main className="min-h-screen bg-background px-4 py-10 text-foreground">
        <div className="mx-auto max-w-md space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <UserCircle2 className="size-5" /> Dashboard Login
              </CardTitle>
              <CardDescription>
                {adminSession.configured
                  ? 'Sign in with admin username and password configured during installation.'
                  : 'Admin login is not configured on server. Set ADMIN_* values and restart backend.'}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="admin-username">Username</Label>
                <Input id="admin-username" value={adminUsername} onChange={(event) => setAdminUsername(event.target.value)} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="admin-password">Password</Label>
                <Input
                  id="admin-password"
                  type="password"
                  value={adminPassword}
                  onChange={(event) => setAdminPassword(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      void loginAdmin()
                    }
                  }}
                />
              </div>
              <Button onClick={() => void loginAdmin()} disabled={busy || !adminSession.configured} className="w-full">
                <ShieldCheck className="size-4" /> Sign In
              </Button>
              {message ? <p className="text-sm text-destructive">{message}</p> : null}
            </CardContent>
          </Card>
        </div>
      </main>
    )
  }

  return (
    <main className="min-h-screen bg-background px-3 py-6 text-foreground sm:px-4 sm:py-8">
      <div className="mx-auto w-full max-w-7xl space-y-6">
        {showOnboarding ? (
          <section className="space-y-5">
            <header className="rounded-2xl border border-border/60 bg-gradient-to-r from-orange-100 via-amber-50 to-teal-100 p-6 dark:from-amber-950/30 dark:via-background dark:to-teal-900/20">
              <div className="flex items-center gap-2">
                <WandSparkles className="size-5" />
                <h1 className="font-display text-2xl">Welcome Setup</h1>
              </div>
              <p className="mt-2 text-sm text-muted-foreground">
                First we configure required credentials. Telegram verify controls are available in the dashboard.
              </p>
              <div className="mt-4 flex flex-wrap gap-2">
                <Badge variant={!needsConfig ? 'default' : 'secondary'}>{!needsConfig ? 'Setup Done' : 'Step 1: Configure App'}</Badge>
              </div>
              {message ? <p className="mt-3 text-sm text-primary">{message}</p> : null}
            </header>

            <Card>
              <CardHeader>
                <CardTitle>Step 1: Required Settings</CardTitle>
                <CardDescription>Only essential values are shown. Advanced options are fixed in application defaults.</CardDescription>
              </CardHeader>
              <CardContent className="grid gap-4 md:grid-cols-2">
                {ONBOARDING_REQUIRED_KEYS.map((key) => (
                  <div key={key} className="space-y-2">
                    <Label>{settingLabels[key] || key}</Label>
                    <Input
                      type={secretSettingKeys.has(key) ? 'password' : 'text'}
                      value={settingsValues[key] ?? ''}
                      onChange={(event) => updateSetting(key, event.target.value)}
                    />
                  </div>
                ))}
                <div className="md:col-span-2">
                  <Button onClick={() => void saveSettingsSubset(ONBOARDING_REQUIRED_KEYS, 'Settings saved. Continue with Telegram verify below.')} disabled={busy}>
                    <Save className="size-4" /> Save Settings
                  </Button>
                </div>
              </CardContent>
            </Card>
          </section>
        ) : (
          <>
            <header className="sticky top-3 z-30 rounded-2xl border border-border/60 bg-card/90 px-4 py-3 shadow-sm backdrop-blur">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div className="flex items-center gap-2">
                  {!needsAuth ? (
                    <Button
                      variant="outline"
                      size="icon"
                      className="xl:hidden"
                      onClick={() => setMobileNavOpen(true)}
                      aria-label="Open navigation menu"
                    >
                      <Menu className="size-4" />
                    </Button>
                  ) : null}
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                    <h1 className="font-brand text-xl leading-tight sm:text-2xl">Tele Auto</h1>
                    <span className="rounded-full border border-border/70 bg-muted/70 px-3 py-1 text-[11px] font-medium text-muted-foreground sm:text-xs">
                      Agentic Telegram Automation
                    </span>
                  </div>
                </div>
                <div className="flex w-full flex-wrap gap-2 lg:w-auto lg:justify-end">
                  <Badge variant={telegramConnection.badgeVariant}>
                    <ShieldCheck className="mr-1 size-3" />
                    {telegramConnection.label}
                  </Badge>
                  <Badge variant={serviceStatus.running ? 'default' : 'outline'}>
                    <Activity className="mr-1 size-3" />
                    {serviceStatus.running ? `Worker Running (${formatUptime(serviceStatus.uptimeSec)})` : 'Worker Stopped'}
                  </Badge>
                  {adminSession.username ? <Badge variant="secondary">Signed in as {adminSession.username}</Badge> : null}
                </div>
              </div>
            </header>

            {needsAuth ? (
              <>
                <Card className="border-primary/25 bg-gradient-to-br from-card via-card to-accent/20">
                  <CardHeader>
                    <CardTitle>Step 2: Telegram Verify</CardTitle>
                    <CardDescription>
                      Request OTP with phone, then verify login with OTP code and optional 2FA password.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div className="space-y-2">
                      <Label htmlFor="dashboard-phone">Phone</Label>
                      <div className="flex flex-col gap-2 sm:flex-row">
                        <Input
                          id="dashboard-phone"
                          className="sm:flex-1"
                          placeholder="+959..."
                          value={phone}
                          onChange={(event) => setPhone(event.target.value)}
                          onBlur={() => {
                            const normalized = normalizePhoneWithPlus(phone)
                            if (normalized !== phone) {
                              setPhone(normalized)
                            }
                          }}
                        />
                        <Button onClick={() => void requestOtp()} disabled={busy || !phone.trim()}>
                          <KeyRound className="size-4" /> Request OTP
                        </Button>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        OTP normally arrives in Telegram app first, then fallback to SMS.
                      </p>
                    </div>
                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="space-y-2">
                        <Label htmlFor="dashboard-code">OTP Code</Label>
                        <Input
                          id="dashboard-code"
                          placeholder="Telegram OTP"
                          value={code}
                          onChange={(event) => setCode(event.target.value)}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="dashboard-password">2FA Password</Label>
                        <Input
                          id="dashboard-password"
                          type="password"
                          placeholder="Optional"
                          value={password}
                          onChange={(event) => setPassword(event.target.value)}
                        />
                      </div>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Button onClick={() => void verifyLogin()} disabled={busy || !code.trim()}>
                        <ShieldCheck className="size-4" /> Verify Login
                      </Button>
                      <Button variant="outline" onClick={clearTelegramVerifyForm} disabled={busy}>
                        Clear
                      </Button>
                    </div>
                    {!authStatus.configured && authStatus.error ? <p className="text-xs text-destructive">{authStatus.error}</p> : null}
                  </CardContent>
                </Card>

                <Card className="border-dashed border-border/80">
                  <CardContent className="flex items-start gap-3 p-6 text-sm text-muted-foreground">
                    <Lock className="mt-0.5 size-4 shrink-0" />
                    <p>
                      Settings, SOUL editor, and realtime logs will unlock after Telegram verification succeeds.
                    </p>
                  </CardContent>
                </Card>
              </>
            ) : (
              <div className="relative">
                <div
                  className={`fixed inset-0 z-40 bg-slate-900/45 transition-opacity duration-200 xl:hidden ${mobileNavOpen ? 'opacity-100' : 'pointer-events-none opacity-0'}`}
                  onClick={() => setMobileNavOpen(false)}
                />
                <aside
                  className={`fixed inset-y-0 left-0 z-50 w-[88%] max-w-[320px] transform border-r border-border/60 bg-background p-4 shadow-2xl transition-transform duration-200 xl:hidden ${mobileNavOpen ? 'translate-x-0' : '-translate-x-full'}`}
                  aria-hidden={!mobileNavOpen}
                >
                  <div className="flex items-center justify-between pb-4">
                    <p className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Menu</p>
                    <Button variant="ghost" size="icon" onClick={() => setMobileNavOpen(false)} aria-label="Close navigation menu">
                      <X className="size-4" />
                    </Button>
                  </div>
                  <div className="space-y-4">{renderNavigation()}</div>
                </aside>

                <div className="grid items-start gap-6 xl:grid-cols-[250px_minmax(0,1fr)]">
                  <aside className="hidden self-start space-y-4 rounded-2xl border border-border/60 bg-card/70 p-3 xl:sticky xl:top-24 xl:block">
                    {renderNavigation()}
                  </aside>

                  <div className="space-y-6">
                    <div className="rounded-xl border border-border/60 bg-card/75 px-4 py-3">
                      <p className="text-xs uppercase tracking-wide text-muted-foreground">Current Page</p>
                      <p className="mt-1 text-lg font-semibold">{currentPageLabel}</p>
                    </div>
                    {message ? (
                      <div className="rounded-xl border border-primary/30 bg-primary/10 px-4 py-2 text-sm text-primary">
                        {message}
                      </div>
                    ) : null}

                  {activePage === 'dashboard' ? (
                    <section className="space-y-4">
                      <div className="relative overflow-hidden rounded-2xl border border-border/60 bg-gradient-to-r from-amber-100 via-orange-50 to-teal-100 p-4 shadow-sm dark:from-amber-950/30 dark:via-background dark:to-teal-900/20 sm:p-6">
                        <div className="space-y-2">
                          <h2 className="font-display text-2xl tracking-tight sm:text-3xl">Tele Auto Control Center</h2>
                          <p className="max-w-3xl text-sm text-muted-foreground">AI system console for worker state, Telegram auth, settings, and runtime logs.</p>
                        </div>
                        <div className="mt-4 flex flex-wrap items-start gap-2 sm:items-center">
                          <Badge variant={telegramConnection.badgeVariant}>
                            <ShieldCheck className="mr-1 size-3" />
                            <span className={`mr-2 inline-block size-2 rounded-full align-middle ${telegramConnection.dotClass}`} />
                            {telegramConnection.label}
                          </Badge>
                          <Badge variant={serviceStatus.running ? 'default' : 'outline'}>
                            <Activity className="mr-1 size-3" />
                            {serviceStatus.running ? `Worker Running (${formatUptime(serviceStatus.uptimeSec)})` : 'Worker Stopped'}
                          </Badge>
                          {adminSession.username ? <Badge variant="secondary">Signed in as {adminSession.username}</Badge> : null}
                          {serviceStatus.lastError ? <Badge variant="destructive" className="max-w-full break-all">Last Error: {serviceStatus.lastError}</Badge> : null}
                        </div>
                        <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                          <div className="rounded-lg border border-border/70 bg-background/70 p-3">
                            <p className="flex items-center gap-1 text-[11px] uppercase tracking-wide text-muted-foreground">
                              <Activity className="size-3" /> Worker
                            </p>
                            <p className="mt-1 text-sm font-semibold">{serviceStatus.running ? 'Running' : 'Stopped'}</p>
                          </div>
                          <div className="rounded-lg border border-border/70 bg-background/70 p-3">
                            <p className="flex items-center gap-1 text-[11px] uppercase tracking-wide text-muted-foreground">
                              <Bot className="size-3" /> Telegram
                            </p>
                            <p className="mt-1 flex items-center gap-2 text-sm font-semibold">
                              <span className={`inline-block size-2.5 rounded-full ${telegramConnection.dotClass}`} />
                              {telegramConnection.label}
                            </p>
                          </div>
                          <div className="rounded-lg border border-border/70 bg-background/70 p-3">
                            <p className="flex items-center gap-1 text-[11px] uppercase tracking-wide text-muted-foreground">
                              <Clock3 className="size-3" /> Uptime
                            </p>
                            <p className="mt-1 text-sm font-semibold">{serviceStatus.running ? formatUptime(serviceStatus.uptimeSec) : '0s'}</p>
                          </div>
                          {authStatus.authorized ? (
                            <div className="rounded-lg border border-border/70 bg-background/70 p-3">
                              <p className="flex items-center gap-1 text-[11px] uppercase tracking-wide text-muted-foreground">
                                <UserCircle2 className="size-3" /> Telegram Phone
                              </p>
                              <p className="mt-1 text-sm font-semibold">{telegramPhoneLabel}</p>
                            </div>
                          ) : null}
                        </div>
                        <div className="mt-4 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                          <div className="flex flex-col gap-2 rounded-lg border border-border/60 bg-background/60 p-2 sm:flex-row sm:flex-wrap sm:items-center">
                            <Button size="sm" variant="outline" className="w-full sm:w-auto" onClick={() => void refreshState()} disabled={busy || loading}>
                              <RefreshCw className="size-4" /> Refresh
                            </Button>
                            {canManageWorker ? (
                              <>
                                <Button
                                  size="sm"
                                  variant="success"
                                  className="w-full sm:w-auto"
                                  onClick={() => void runAction(() => apiRequest('/api/service/start', { method: 'POST' }), 'Worker started')}
                                  disabled={busy || serviceStatus.running}
                                >
                                  <Play className="size-4" /> Start
                                </Button>
                                <Button
                                  size="sm"
                                  variant="info"
                                  className="w-full sm:w-auto"
                                  onClick={() =>
                                    void runConfirmedAction(
                                      'Restart worker now?',
                                      () => apiRequest('/api/service/restart', { method: 'POST' }).then(() => undefined),
                                      'Worker restarted',
                                      { title: 'Restart Worker', confirmLabel: 'Restart' },
                                    )
                                  }
                                  disabled={busy || !serviceStatus.running}
                                >
                                  <RefreshCw className="size-4" /> Restart
                                </Button>
                                <Button
                                  size="sm"
                                  variant="destructive"
                                  className="w-full sm:w-auto"
                                  onClick={() =>
                                    void runConfirmedAction(
                                      'Stop worker now?',
                                      () => apiRequest('/api/service/stop', { method: 'POST' }).then(() => undefined),
                                      'Worker stopped',
                                      { title: 'Stop Worker', confirmLabel: 'Stop', confirmVariant: 'destructive' },
                                    )
                                  }
                                  disabled={busy || !serviceStatus.running}
                                >
                                  <Square className="size-4" /> Stop
                                </Button>
                              </>
                            ) : (
                              <Badge variant="secondary">Verify Telegram to unlock worker controls</Badge>
                            )}
                          </div>
                          <div className="flex flex-col gap-2 rounded-lg border border-border/60 bg-background/60 p-2 sm:flex-row sm:flex-wrap sm:items-center">
                            {authStatus.authorized ? (
                              <Button size="sm" variant="outline" className="w-full sm:w-auto" onClick={() => void logoutTelegram()} disabled={busy}>
                                <LogOut className="size-4" /> Telegram Sign Out
                              </Button>
                            ) : null}
                            <Button size="sm" variant="outline" className="w-full sm:w-auto" onClick={() => void logoutAdmin()} disabled={busy}>
                              <UserCircle2 className="size-4" /> Dashboard Sign Out
                            </Button>
                          </div>
                        </div>
                      </div>
                      <Card>
                        <CardHeader>
                          <CardTitle>Realtime Conversations</CardTitle>
                          <CardDescription>Human takeover chat panel with per-conversation mode control and manual-chat notifications.</CardDescription>
                          <div className="flex flex-wrap items-center gap-2">
                            <Badge variant={globalAutoReplyEnabled ? 'default' : 'destructive'}>
                              Global Auto Reply: {globalAutoReplyEnabled ? 'ON' : 'OFF'}
                            </Badge>
                            {!globalAutoReplyEnabled ? <Badge variant="secondary">Effective mode is Manual for all chats</Badge> : null}
                            {notificationPermission === 'granted' ? (
                              <Badge variant="outline">
                                <BellRing className="mr-1 size-3" /> Manual Chat Notifications On
                              </Badge>
                            ) : null}
                            {notificationPermission === 'default' ? (
                              <Button size="sm" variant="outline" onClick={() => void requestBrowserNotifications()}>
                                <BellRing className="mr-1 size-4" /> Enable Notifications
                              </Button>
                            ) : null}
                            {notificationPermission === 'denied' ? (
                              <Badge variant="secondary">Notifications blocked in browser settings</Badge>
                            ) : null}
                            {notificationPermission === 'unsupported' ? (
                              <Badge variant="secondary">Browser notifications unavailable</Badge>
                            ) : null}
                          </div>
                        </CardHeader>
                        <CardContent>
                          <div className="grid gap-4 lg:grid-cols-[300px_minmax(0,1fr)]">
                            <div className="max-h-[520px] overflow-auto rounded-lg border border-border/60 bg-background">
                              {conversations.map((item) => (
                                <button
                                  key={item.chatId}
                                  type="button"
                                  className={`w-full border-b border-border/40 px-3 py-2 text-left last:border-b-0 ${selectedChatId === item.chatId ? 'bg-primary/10' : 'hover:bg-accent/50'}`}
                                  onClick={() => setSelectedChatId(item.chatId)}
                                >
                                  <p className="truncate text-sm font-medium">{item.chatName || item.chatId}</p>
                                  <p className="truncate text-xs text-muted-foreground">{item.lastMessage}</p>
                                  <div className="mt-1 flex items-center gap-2 text-[11px] text-muted-foreground">
                                    <span>{formatLocalTime(item.lastMessageAt)}</span>
                                    <span>•</span>
                                    <span>{item.effectiveMode.toUpperCase()}</span>
                                    {item.unreadIncoming > 0 ? <Badge variant="outline">{item.unreadIncoming}</Badge> : null}
                                  </div>
                                </button>
                              ))}
                              {conversations.length === 0 ? <p className="p-3 text-sm text-muted-foreground">No conversations yet.</p> : null}
                            </div>

                            <div className="space-y-3 rounded-lg border border-border/60 bg-background p-3">
                              {selectedConversation ? (
                                <>
                                  <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                                    <div>
                                      <p className="text-sm font-semibold">{selectedConversation.chatName || selectedConversation.chatId}</p>
                                      <p className="text-xs text-muted-foreground">{selectedConversation.chatId}</p>
                                    </div>
                                    <div className="flex flex-wrap items-center gap-2">
                                      <Button
                                        size="sm"
                                        variant={selectedConversation.mode === 'manual' ? 'default' : 'outline'}
                                        disabled={busy || !globalAutoReplyEnabled}
                                        onClick={() => void setConversationMode(selectedConversation.chatId, 'manual')}
                                      >
                                        Manual
                                      </Button>
                                      <Button
                                        size="sm"
                                        variant={selectedConversation.mode !== 'manual' ? 'default' : 'outline'}
                                        disabled={busy || !globalAutoReplyEnabled}
                                        onClick={() => void setConversationMode(selectedConversation.chatId, 'auto')}
                                      >
                                        Auto
                                      </Button>
                                      <Badge variant={selectedConversation.effectiveMode === 'manual' ? 'secondary' : 'default'}>
                                        Effective: {selectedConversation.effectiveMode}
                                      </Badge>
                                    </div>
                                  </div>

                                  <div ref={conversationThreadRef} className="max-h-[340px] overflow-auto rounded-lg border border-border/60 bg-card/30 p-2">
                                    {activeMessages.map((m) => (
                                      <div key={`${m.id}-${m.telegramMessageId}`} className={`mb-2 flex ${m.direction === 'me' ? 'justify-end' : 'justify-start'}`}>
                                        <div className={`max-w-[85%] rounded-lg px-3 py-2 text-sm ${m.direction === 'me' ? 'bg-primary text-primary-foreground' : 'bg-muted'}`}>
                                          <p className="whitespace-pre-wrap break-words">{m.text}</p>
                                          <p className="mt-1 text-[10px] opacity-70">{formatLocalTime(m.createdAt)}</p>
                                        </div>
                                      </div>
                                    ))}
                                    {activeMessages.length === 0 ? <p className="text-xs text-muted-foreground">No messages for this chat yet.</p> : null}
                                  </div>

                                  <div className="space-y-2">
                                    <Textarea
                                      placeholder="Type manual reply..."
                                      value={manualReplyText}
                                      onChange={(event) => setManualReplyText(event.target.value)}
                                      className="min-h-[90px]"
                                    />
                                    <div className="flex items-center justify-end">
                                      <Button size="sm" onClick={() => void sendManualReply()} disabled={busy || !manualReplyText.trim()}>
                                        Send Human Reply
                                      </Button>
                                    </div>
                                  </div>
                                </>
                              ) : (
                                <p className="text-sm text-muted-foreground">Select a conversation from the left panel.</p>
                              )}
                            </div>
                          </div>
                        </CardContent>
                      </Card>
                    </section>
                  ) : null}

                  {activePage === 'logs' ? (
                    <Card>
                      <CardHeader>
                        <CardTitle className="flex items-center gap-2">
                          <Terminal className="size-4" /> Realtime Logs
                        </CardTitle>
                        <CardDescription>Streaming from backend SSE endpoint `/api/logs/stream`.</CardDescription>
                        <div className="flex flex-col gap-2 sm:flex-row">
                          <Button size="sm" className="w-full sm:w-auto" variant={logMode === 'all' ? 'default' : 'outline'} onClick={() => setLogMode('all')}>
                            All
                          </Button>
                          <Button size="sm" className="w-full sm:w-auto" variant={logMode === 'orchestrator' ? 'default' : 'outline'} onClick={() => setLogMode('orchestrator')}>
                            Execution Logs
                          </Button>
                        </div>
                      </CardHeader>
                      <CardContent>
                        <div className="max-h-[380px] overflow-auto rounded-lg border border-border/60 bg-zinc-950 p-3 font-mono text-xs text-zinc-100">
                          {filteredLogs.map((entry, index) => (
                            <div key={`${entry.time}-${index}`} className="mb-1 break-all">
                              <span className="text-zinc-400">[{entry.time}]</span>{' '}
                              <span className={entry.level.includes('error') ? 'text-rose-300' : entry.level.includes('warn') ? 'text-amber-300' : 'text-emerald-300'}>
                                {entry.level.toUpperCase()}
                              </span>{' '}
                              <span>{entry.message}</span>
                              {entry.attrs && Object.keys(entry.attrs).length > 0 ? (
                                <>
                                  {' '}
                                  <span className="text-zinc-300">{JSON.stringify(entry.attrs)}</span>
                                </>
                              ) : null}
                            </div>
                          ))}
                          {filteredLogs.length === 0 ? <p className="text-zinc-400">No logs yet.</p> : null}
                        </div>
                      </CardContent>
                    </Card>
                  ) : null}

                  {activePage === 'agents' ? (
                    <Card>
                      <CardHeader>
                        <CardTitle>Agents</CardTitle>
                        <CardDescription>Create Markdown-based agents with frontmatter metadata.</CardDescription>
                      </CardHeader>
                      <CardContent className="space-y-3">
                        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                          <Input
                            placeholder="id (e.g. price_agent)"
                            value={agentForm.id}
                            disabled={Boolean(editingAgentID)}
                            onChange={(event) => setAgentForm((prev) => ({ ...prev, id: event.target.value }))}
                            className="h-9"
                          />
                          <Input
                            placeholder="name"
                            value={agentForm.name}
                            onChange={(event) => setAgentForm((prev) => ({ ...prev, name: event.target.value }))}
                            className="h-9"
                          />
                          <Input
                            placeholder="description"
                            value={agentForm.description}
                            onChange={(event) => setAgentForm((prev) => ({ ...prev, description: event.target.value }))}
                            className="h-9 sm:col-span-2"
                          />
                          <div className="space-y-1 sm:col-span-2">
                            <Label>Visibility</Label>
                            <select
                              className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                              value={agentForm.visibility || 'public'}
                              onChange={(event) =>
                                setAgentForm((prev) => ({
                                  ...prev,
                                  visibility: event.target.value === 'private' ? 'private' : 'public',
                                }))
                              }
                            >
                              <option value="public">public</option>
                              <option value="private">private</option>
                            </select>
                          </div>
                          {agentForm.visibility === 'private' ? (
                            <div className="space-y-1 sm:col-span-2">
                              <Label>Allow Users</Label>
                              <Textarea
                                placeholder="User IDs or usernames (@name), comma or newline separated"
                                value={allowUsersInput}
                                onChange={(event) => setAllowUsersInput(event.target.value)}
                                className="min-h-[76px] font-mono text-xs"
                              />
                            </div>
                          ) : null}
                          <Input
                            placeholder="intents: price,order,info"
                            value={agentForm.intents.join(',')}
                            onChange={(event) =>
                              setAgentForm((prev) => ({
                                ...prev,
                                intents: event.target.value.split(','),
                              }))
                            }
                            className="h-9 sm:col-span-2"
                          />
                          <Input
                            placeholder="variables: API_TOKEN,BASE_URL"
                            value={agentForm.variables.join(',')}
                            onChange={(event) =>
                              setAgentForm((prev) => ({
                                ...prev,
                                variables: event.target.value.split(','),
                              }))
                            }
                            className="h-9 sm:col-span-2"
                          />
                        </div>
                        <Textarea
                          placeholder="Agent markdown body instructions..."
                          value={agentForm.body}
                          onChange={(event) => setAgentForm((prev) => ({ ...prev, body: event.target.value }))}
                          className="min-h-[130px] font-mono text-xs sm:min-h-[160px]"
                        />
                        <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
                          <Button size="sm" className="w-full sm:w-auto" onClick={() => void saveAgent()} disabled={busy}>
                            <Save className="size-4" /> {editingAgentID ? 'Update Agent' : 'Create Agent'}
                          </Button>
                          <Button size="sm" variant="outline" className="w-full sm:w-auto" onClick={resetAgentForm} disabled={busy}>
                            Clear
                          </Button>
                        </div>
                        <div className="max-h-52 overflow-auto rounded-lg border border-border/60 bg-background p-2 text-sm">
                          {agents.map((agent) => (
                            <div key={agent.id} className="flex flex-col gap-2 border-b border-border/40 py-1 last:border-b-0 sm:flex-row sm:items-center sm:justify-between">
                              <div className="min-w-0">
                                <p className="break-all font-medium">{agent.id}</p>
                                <p className="break-words text-xs text-muted-foreground">{agent.description || agent.name}</p>
                                <div className="mt-1 flex flex-wrap gap-1">
                                  <Badge variant={(agent.visibility || 'public') === 'private' ? 'secondary' : 'outline'}>
                                    {(agent.visibility || 'public') === 'private' ? 'Private' : 'Public'}
                                  </Badge>
                                  {(agent.visibility || 'public') === 'private' ? (
                                    <Badge variant="outline">{(agent.allowUsers || []).length} allow users</Badge>
                                  ) : null}
                                </div>
                              </div>
                              <div className="flex w-full gap-1 sm:w-auto">
                                <Button size="sm" variant="outline" className="flex-1 sm:flex-none" onClick={() => startEditAgent(agent)}>Edit</Button>
                                <Button size="sm" variant="destructive" className="flex-1 sm:flex-none" onClick={() => void deleteAgent(agent.id)}>Delete</Button>
                              </div>
                            </div>
                          ))}
                          {agents.length === 0 ? <p className="text-xs text-muted-foreground">No agents yet.</p> : null}
                        </div>
                      </CardContent>
                    </Card>
                  ) : null}

                  {activePage === 'settings' && activeSettingsPage === 'setting' ? (
                    <Card>
                      <CardHeader>
                        <CardTitle>Settings</CardTitle>
                        <CardDescription>Only daily-use settings are shown. Advanced values are hidden and fixed.</CardDescription>
                      </CardHeader>
                      <CardContent className="space-y-3">
                        {MAIN_VISIBLE_SETTINGS.filter((key) => settingsKeys.includes(key)).map((key) => {
                          const value = settingsValues[key] ?? ''
                          if (booleanSettingKeys.has(key)) {
                            return (
                              <div key={key} className="flex items-center justify-between rounded-lg border border-border/60 bg-background p-2.5">
                                <div>
                                  <Label>{settingLabels[key] || key}</Label>
                                  <p className="text-xs text-muted-foreground">Feature toggle</p>
                                </div>
                                <Switch checked={value.toLowerCase() === 'true'} onCheckedChange={(checked) => updateSetting(key, checked ? 'true' : 'false')} />
                              </div>
                            )
                          }
                          return (
                            <div key={key} className="space-y-1">
                              <Label>{settingLabels[key] || key}</Label>
                              <Input
                                type={secretSettingKeys.has(key) ? 'password' : numericSettingKeys.has(key) ? 'number' : 'text'}
                                value={value}
                                onChange={(event) => updateSetting(key, event.target.value)}
                                className="h-9 text-sm"
                              />
                            </div>
                          )
                        })}
                        <Button size="sm" onClick={() => void saveSettingsSubset(MAIN_VISIBLE_SETTINGS.filter((key) => settingsKeys.includes(key)), 'Settings saved')} disabled={busy}>
                          <Save className="size-4" /> Save Settings
                        </Button>
                      </CardContent>
                    </Card>
                  ) : null}

                  {activePage === 'settings' && activeSettingsPage === 'soul' ? (
                    <Card>
                      <CardHeader>
                        <CardTitle>SOUL Prompt</CardTitle>
                        <CardDescription>Update personality profile used by AI reply context.</CardDescription>
                      </CardHeader>
                      <CardContent className="space-y-3">
                        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                          <Badge variant="outline">{soulCharacterCount} chars</Badge>
                          <Badge variant="outline">Loaded: {formatLocalTime(soulLoadedAt)}</Badge>
                          {soulSavedAt ? <Badge variant="outline">Saved: {formatLocalTime(soulSavedAt)}</Badge> : null}
                        </div>
                        <Textarea value={soulText} onChange={(event) => setSoulText(event.target.value)} className="min-h-[235px] font-mono text-xs" />
                        <Button size="sm" onClick={() => void saveSoul()} disabled={busy}>
                          <Save className="size-4" /> Save SOUL
                        </Button>
                      </CardContent>
                    </Card>
                  ) : null}

                  {activePage === 'settings' && activeSettingsPage === 'telegram' ? (
                    <Card>
                      <CardHeader>
                        <CardTitle>Telegram</CardTitle>
                        <CardDescription>Connection and account actions.</CardDescription>
                      </CardHeader>
                      <CardContent className="space-y-3">
                        <div className="grid gap-3 sm:grid-cols-2">
                          <div className="rounded-lg border border-border/60 bg-background p-3">
                            <p className="text-xs uppercase tracking-wide text-muted-foreground">Status</p>
                            <p className="mt-1 text-sm font-semibold">{telegramConnection.label}</p>
                          </div>
                          <div className="rounded-lg border border-border/60 bg-background p-3">
                            <p className="text-xs uppercase tracking-wide text-muted-foreground">Phone</p>
                            <p className="mt-1 text-sm font-semibold">{telegramPhoneLabel}</p>
                          </div>
                        </div>
                        <div className="flex flex-wrap gap-2">
                          <Button size="sm" variant="outline" onClick={() => void logoutTelegram()} disabled={busy || !authStatus.authorized}>
                            <LogOut className="size-4" /> Telegram Sign Out
                          </Button>
                        </div>
                      </CardContent>
                    </Card>
                  ) : null}

                  {activePage === 'settings' && activeSettingsPage === 'variables' ? (
                    <Card>
                      <CardHeader>
                        <CardTitle>Global Variables</CardTitle>
                        <CardDescription>Typed variables (`text`, `secret`) for agent interpolation and api_call tool.</CardDescription>
                      </CardHeader>
                      <CardContent className="space-y-3">
                        <div className="grid gap-2 md:grid-cols-3">
                          <Input
                            placeholder="KEY_NAME"
                            value={variableForm.key}
                            onChange={(event) => setVariableForm((prev) => ({ ...prev, key: event.target.value }))}
                            className="h-9"
                          />
                          <select
                            className="h-9 rounded-md border border-input bg-background px-3 text-sm"
                            value={variableForm.type}
                            onChange={(event) => setVariableForm((prev) => ({ ...prev, type: event.target.value as 'text' | 'secret' }))}
                          >
                            <option value="text">text</option>
                            <option value="secret">secret</option>
                          </select>
                          <Input
                            type={variableForm.type === 'secret' ? 'password' : 'text'}
                            placeholder="value"
                            value={variableForm.value}
                            onChange={(event) => setVariableForm((prev) => ({ ...prev, value: event.target.value }))}
                            className="h-9"
                          />
                        </div>
                        <Button size="sm" onClick={() => void saveVariable()} disabled={busy}>
                          <Save className="size-4" /> Save Variable
                        </Button>
                        <div className="max-h-52 overflow-auto rounded-lg border border-border/60 bg-background p-2 text-sm">
                          {variables.map((v) => (
                            <div key={v.key} className="flex flex-col gap-2 border-b border-border/40 py-1 last:border-b-0 sm:flex-row sm:items-center sm:justify-between">
                              <div className="min-w-0">
                                <p className="font-medium">{v.key}</p>
                                <p className="text-xs text-muted-foreground">
                                  {v.type} • {v.value}
                                </p>
                              </div>
                              <div className="flex w-full gap-1 sm:w-auto">
                                <Button
                                  size="sm"
                                  variant="outline"
                                  className="w-full sm:w-auto"
                                  onClick={() => setVariableForm({ key: v.key, type: v.type, value: '' })}
                                >
                                  Edit
                                </Button>
                                <Button
                                  size="sm"
                                  variant="destructive"
                                  className="w-full sm:w-auto"
                                  onClick={() => void deleteVariable(v.key)}
                                >
                                  Delete
                                </Button>
                              </div>
                            </div>
                          ))}
                          {variables.length === 0 ? <p className="text-xs text-muted-foreground">No variables yet.</p> : null}
                        </div>
                      </CardContent>
                    </Card>
                  ) : null}

                  {activePage === 'settings' && activeSettingsPage === 'user' ? (
                    <Card>
                      <CardHeader>
                        <CardTitle>User</CardTitle>
                        <CardDescription>Change dashboard username and password used for admin login.</CardDescription>
                      </CardHeader>
                      <CardContent className="grid gap-3 md:grid-cols-2">
                        <div className="space-y-1">
                          <Label htmlFor="account-current-password">Current Password</Label>
                          <Input
                            id="account-current-password"
                            type="password"
                            value={currentAdminPassword}
                            onChange={(event) => setCurrentAdminPassword(event.target.value)}
                            className="h-9 text-sm"
                          />
                        </div>
                        <div className="space-y-1">
                          <Label htmlFor="account-username">New Username</Label>
                          <Input
                            id="account-username"
                            value={newAdminUsername}
                            onChange={(event) => setNewAdminUsername(event.target.value)}
                            className="h-9 text-sm"
                          />
                        </div>
                        <div className="space-y-1">
                          <Label htmlFor="account-new-password">New Password</Label>
                          <Input
                            id="account-new-password"
                            type="password"
                            placeholder="Leave blank to keep current password"
                            value={newAdminPassword}
                            onChange={(event) => setNewAdminPassword(event.target.value)}
                            className="h-9 text-sm"
                          />
                        </div>
                        <div className="space-y-1">
                          <Label htmlFor="account-confirm-password">Confirm Password</Label>
                          <Input
                            id="account-confirm-password"
                            type="password"
                            placeholder="Repeat new password"
                            value={confirmAdminPassword}
                            onChange={(event) => setConfirmAdminPassword(event.target.value)}
                            className="h-9 text-sm"
                          />
                        </div>
                        <div className="md:col-span-2 flex flex-col items-start gap-2 sm:flex-row sm:items-center">
                          <Button size="sm" className="w-full sm:w-auto" onClick={() => void updateAdminCredentials()} disabled={busy}>
                            <ShieldCheck className="size-4" /> Update Dashboard Account
                          </Button>
                          <span className="text-xs text-muted-foreground">After update, dashboard login is required again.</span>
                        </div>
                      </CardContent>
                    </Card>
                  ) : null}
                </div>
              </div>
            </div>
            )}
          </>
        )}
      </div>
      <AlertDialog
        open={confirmDialog.open}
        onOpenChange={(open) => {
          if (!open && !busy) {
            closeConfirmDialog(false)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{confirmDialog.title}</AlertDialogTitle>
            <AlertDialogDescription>{confirmDialog.description}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={busy}
              className={
                confirmDialog.confirmVariant === 'destructive'
                  ? 'bg-destructive text-destructive-foreground hover:bg-destructive/90'
                  : undefined
              }
              onClick={(event) => {
                event.preventDefault()
                void handleConfirmAction()
              }}
            >
              {busy ? 'Processing...' : confirmDialog.confirmLabel}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </main>
  )
}
