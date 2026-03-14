import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { apiRequest } from '@/features/control/api'
import {
  ONBOARDING_REQUIRED_KEYS,
  API_BASE,
} from '@/features/control/constants'
import { useBehaviorPolicyEditor } from '@/features/control/hooks/useBehaviorPolicyEditor'
import { useManualConversationNotifications } from '@/features/control/hooks/useManualConversationNotifications'
import { formatCurrentPageLabel, readRouteState } from '@/features/control/routeState'
import {
  normalizeBehaviorPolicy,
  normalizePhoneWithPlus,
  parseAllowUsersInput,
  readInitialPhone,
  normalizeSettings,
} from '@/features/control/utils'
import type {
  AdminSession,
  AgentDefinition,
  AuthStatus,
  BehaviorResponse,
  BehaviorRuntimeResponse,
  ConfirmDialogState,
  ConversationMessage,
  ConversationsResponse,
  ConversationStreamEvent,
  ConversationSummary,
  ConversationMessagesResponse,
  LogEntry,
  LoginResponse,
  MainPage,
  ServiceStatus,
  SettingsPage,
  SettingsResponse,
  UpdateAdminCredentialsResponse,
  VariableValue,
} from '@/features/control/types'

export function useControlCenter() {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
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
  const conversationThreadRef = useRef<HTMLDivElement | null>(null)
  const lastRenderedMessageKeyRef = useRef('')
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const {
    behaviorPolicy,
    setBehaviorPolicy,
    behaviorLoadedAt,
    behaviorPath,
    behaviorRuntimeStates,
    behaviorQuietHoursInput,
    behaviorAllowTonesInput,
    behaviorDenyTonesInput,
    behaviorTriggerKeywordsInput,
    syncBehaviorPolicyState,
    updateBehaviorQuietHoursInput,
    updateBehaviorAllowTonesInput,
    updateBehaviorDenyTonesInput,
    updateBehaviorTriggerKeywordsInput,
  } = useBehaviorPolicyEditor()

  const { activePage, activeSettingsPage } = useMemo(
    () => readRouteState(pathname),
    [pathname],
  )

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
  const {
    notificationPermission,
    syncNotificationPermission,
    ensureNotificationAudioContext,
    requestBrowserNotifications,
    notifyForManualConversation,
  } = useManualConversationNotifications({
    getConversations: () => conversationsRef.current,
    getSelectedChatId: () => selectedChatIdRef.current,
    getGlobalAutoReplyEnabled: () => globalAutoReplyEnabledRef.current,
    navigateToDashboard: () => void navigate({ to: '/dashboard' }),
    onSelectChat: setSelectedChatId,
    onSetMessage: setMessage,
  })

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

      const [auth, service, settingsData, soul, logPayload, agentsPayload, varsPayload, behaviorRuntime] = await Promise.all([
        apiRequest<AuthStatus>('/api/auth/status'),
        apiRequest<ServiceStatus>('/api/service/status'),
        apiRequest<SettingsResponse>('/api/settings'),
        apiRequest<{ content: string }>('/api/soul'),
        apiRequest<{ logs: LogEntry[] }>('/api/logs?limit=200'),
        apiRequest<{ agents: AgentDefinition[] }>('/api/agents'),
        apiRequest<{ values: VariableValue[] }>('/api/variables'),
        apiRequest<BehaviorRuntimeResponse>('/api/behavior/runtime'),
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
      syncBehaviorPolicyState(behaviorRuntime.policy, {
        loadedAt: behaviorRuntime.loadedAt,
        path: behaviorRuntime.path,
        states: behaviorRuntime.states || [],
      })
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

  const currentPageLabel = formatCurrentPageLabel(activePage, activeSettingsPage)

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

  const saveBehaviorPolicy = async () => {
    const nextPolicy = normalizeBehaviorPolicy(behaviorPolicy)
    if (nextPolicy.maxReplyWords <= 0) {
      setMessage('Behavior max reply words must be a positive number.')
      return
    }
    if (nextPolicy.escalation.failureThreshold <= 0) {
      setMessage('Behavior failure threshold must be a positive number.')
      return
    }
    const probabilityFields = [
      ['Short reply probability', behaviorPolicy.shortReplyProbability],
      ['One word reply probability', behaviorPolicy.oneWordReplyProbability],
      ['Follow-up probability', behaviorPolicy.followUpProbability],
      ['Split message probability', behaviorPolicy.splitMessageProbability],
    ] as const
    for (const [label, value] of probabilityFields) {
      if (value < 0 || value > 1) {
        setMessage(`${label} must be between 0 and 1.`)
        return
      }
    }

    setBusy(true)
    setMessage('')
    try {
      const result = await apiRequest<BehaviorResponse>('/api/behavior', {
        method: 'PUT',
        body: JSON.stringify(nextPolicy),
      })
      syncBehaviorPolicyState(result.policy, {
        loadedAt: result.loadedAt,
        path: result.path,
        states: behaviorRuntimeStates,
      })
      setMessage('Behavior policy saved.')
      await refreshState()
    } catch (err) {
      handleRequestError(err)
    } finally {
      setBusy(false)
    }
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
    const to = page === 'dashboard'
      ? '/dashboard'
      : page === 'logs'
        ? '/logs'
        : page === 'agents'
          ? '/agents'
          : `/settings/${activeSettingsPage}`
    void navigate({ to })
    setMobileNavOpen(false)
  }

  const openSettingsPage = (page: SettingsPage) => {
    void navigate({ to: `/settings/${page}` })
    setMobileNavOpen(false)
  }


  return {
    loading,
    busy,
    message,
    adminSession,
    adminUsername,
    adminPassword,
    currentAdminPassword,
    newAdminUsername,
    newAdminPassword,
    confirmAdminPassword,
    authStatus,
    serviceStatus,
    settings,
    settingsValues,
    settingsKeys,
    savedSettings,
    savedSettingsValues,
    soulText,
    soulLoadedAt,
    soulSavedAt,
    behaviorPolicy,
    behaviorLoadedAt,
    behaviorPath,
    behaviorRuntimeStates,
    behaviorQuietHoursInput,
    behaviorAllowTonesInput,
    behaviorDenyTonesInput,
    behaviorTriggerKeywordsInput,
    phone,
    code,
    password,
    logs,
    logMode,
    globalAutoReplyEnabled,
    conversations,
    selectedChatId,
    manualReplyText,
    agents,
    editingAgentID,
    allowUsersInput,
    agentForm,
    variables,
    variableForm,
    confirmDialog,
    mobileNavOpen,
    notificationPermission,
    activePage,
    activeSettingsPage,
    needsConfig,
    needsAuth,
    needsAppLogin,
    showOnboarding,
    canManageWorker,
    telegramConnection,
    telegramPhoneLabel,
    soulCharacterCount,
    currentPageLabel,
    filteredLogs,
    selectedConversation,
    activeMessages,
    conversationThreadRef,
    setAdminUsername,
    setAdminPassword,
    setCurrentAdminPassword,
    setNewAdminUsername,
    setNewAdminPassword,
    setConfirmAdminPassword,
    setPhone,
    setCode,
    setPassword,
    setLogMode,
    setSelectedChatId,
    setManualReplyText,
    setAllowUsersInput,
    setAgentForm,
    setVariableForm,
    setSoulText,
    setBehaviorPolicy,
    setMobileNavOpen,
    updateSetting,
    saveSettingsSubset,
    loginAdmin,
    logoutAdmin,
    updateAdminCredentials,
    requestOtp,
    verifyLogin,
    clearTelegramVerifyForm,
    refreshState,
    runAction,
    runConfirmedAction,
    requestBrowserNotifications,
    saveSoul,
    updateBehaviorQuietHoursInput,
    updateBehaviorAllowTonesInput,
    updateBehaviorDenyTonesInput,
    updateBehaviorTriggerKeywordsInput,
    saveBehaviorPolicy,
    saveAgent,
    resetAgentForm,
    startEditAgent,
    deleteAgent,
    loadConversationMessages,
    setConversationMode,
    sendManualReply,
    saveVariable,
    deleteVariable,
    logoutTelegram,
    openPage,
    openSettingsPage,
    handleConfirmAction,
    closeConfirmDialog,
  }
}
