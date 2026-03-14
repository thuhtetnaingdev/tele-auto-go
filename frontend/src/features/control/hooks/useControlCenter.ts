import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { apiRequest } from '@/features/control/api'
import {
  ONBOARDING_REQUIRED_KEYS,
  API_BASE,
} from '@/features/control/constants'
import { useBehaviorPolicyEditor } from '@/features/control/hooks/useBehaviorPolicyEditor'
import { useManualConversationNotifications } from '@/features/control/hooks/useManualConversationNotifications'
import {
  controlQueryKeys,
  fetchAdminSession,
  fetchAgents,
  fetchAuthStatus,
  fetchBehaviorRuntime,
  fetchConversationMessages,
  fetchConversations,
  fetchLogs,
  fetchPersonaGroupMembers,
  fetchPersonaGroups,
  fetchPersonaResolve,
  fetchPersonaUsers,
  fetchServiceStatus,
  fetchSettings,
  fetchSoul,
  fetchVariables,
} from '@/features/control/queries'
import { formatCurrentPageLabel, readRouteState } from '@/features/control/routeState'
import {
  normalizeBehaviorPolicy,
  normalizePhoneWithPlus,
  parseAllowUsersInput,
  readInitialPhone,
} from '@/features/control/utils'
import type {
  AdminSession,
  AgentDefinition,
  AuthStatus,
  BehaviorResponse,
  ConfirmDialogState,
  ConversationMessage,
  ConversationStreamEvent,
  ConversationSummary,
  LogEntry,
  LoginResponse,
  MainPage,
  PersonaGroup,
  PersonaGroupMember,
  PersonaResolveResponse,
  PersonaUserProfile,
  ServiceStatus,
  SettingsPage,
  SettingsResponse,
  UpdateAdminCredentialsResponse,
  VariableValue,
} from '@/features/control/types'

export function useControlCenter() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
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
  const [personaGroups, setPersonaGroups] = useState<PersonaGroup[]>([])
  const [selectedPersonaGroupId, setSelectedPersonaGroupId] = useState('')
  const [personaGroupMembers, setPersonaGroupMembers] = useState<PersonaGroupMember[]>([])
  const [personaGroupForm, setPersonaGroupForm] = useState({ id: '', name: '', slug: '', description: '', markdown: '' })
  const [personaMemberUserId, setPersonaMemberUserId] = useState('')
  const [personaMemberUsername, setPersonaMemberUsername] = useState('')
  const [personaUsers, setPersonaUsers] = useState<PersonaUserProfile[]>([])
  const [selectedPersonaUserId, setSelectedPersonaUserId] = useState('')
  const [personaUserForm, setPersonaUserForm] = useState({ id: '', label: '', userId: '', username: '', enabled: true, markdown: '' })
  const [resolvedPersona, setResolvedPersona] = useState<PersonaResolveResponse | null>(null)
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

  const adminSessionQuery = useQuery({
    queryKey: controlQueryKeys.adminSession(),
    queryFn: fetchAdminSession,
    retry: false,
  })
  const secondaryQueriesEnabled = adminSessionQuery.data?.authenticated === true
  const { activePage, activeSettingsPage } = useMemo(
    () => readRouteState(pathname),
    [pathname],
  )
  const isSettingsPage = activePage === 'settings'
  const needsSoulData = isSettingsPage && activeSettingsPage === 'soul'
  const needsBehaviorData = isSettingsPage && activeSettingsPage === 'behavior'
  const needsVariablesData = activePage === 'agents' || (isSettingsPage && activeSettingsPage === 'variables')
  const needsAgentsData = activePage === 'agents'
  const needsLogsData = activePage === 'logs'
  const needsPersonaGroupsData = isSettingsPage && activeSettingsPage === 'persona-groups'
  const needsPersonaUsersData = isSettingsPage && activeSettingsPage === 'persona-users'
  const selectedChatUserID = useMemo(
    () => (selectedChatId.startsWith('user:') ? selectedChatId.slice('user:'.length).trim() : ''),
    [selectedChatId],
  )
  const authStatusQuery = useQuery({
    queryKey: controlQueryKeys.authStatus(),
    queryFn: fetchAuthStatus,
    enabled: secondaryQueriesEnabled,
  })
  const serviceStatusQuery = useQuery({
    queryKey: controlQueryKeys.serviceStatus(),
    queryFn: fetchServiceStatus,
    enabled: secondaryQueriesEnabled,
  })
  const settingsQuery = useQuery({
    queryKey: controlQueryKeys.settings(),
    queryFn: fetchSettings,
    enabled: secondaryQueriesEnabled,
  })
  const soulQuery = useQuery({
    queryKey: controlQueryKeys.soul(),
    queryFn: fetchSoul,
    enabled: secondaryQueriesEnabled && needsSoulData,
  })
  const logsQuery = useQuery({
    queryKey: controlQueryKeys.logs(),
    queryFn: fetchLogs,
    enabled: secondaryQueriesEnabled && needsLogsData,
  })
  const agentsQuery = useQuery({
    queryKey: controlQueryKeys.agents(),
    queryFn: fetchAgents,
    enabled: secondaryQueriesEnabled && needsAgentsData,
  })
  const variablesQuery = useQuery({
    queryKey: controlQueryKeys.variables(),
    queryFn: fetchVariables,
    enabled: secondaryQueriesEnabled && needsVariablesData,
  })
  const behaviorRuntimeQuery = useQuery({
    queryKey: controlQueryKeys.behaviorRuntime(),
    queryFn: fetchBehaviorRuntime,
    enabled: secondaryQueriesEnabled && needsBehaviorData,
  })
  const conversationsQuery = useQuery({
    queryKey: controlQueryKeys.conversations(),
    queryFn: fetchConversations,
    enabled: secondaryQueriesEnabled,
  })
  const personaGroupsQuery = useQuery({
    queryKey: controlQueryKeys.personaGroups(),
    queryFn: fetchPersonaGroups,
    enabled: secondaryQueriesEnabled && needsPersonaGroupsData,
  })
  const personaUsersQuery = useQuery({
    queryKey: controlQueryKeys.personaUsers(),
    queryFn: fetchPersonaUsers,
    enabled: secondaryQueriesEnabled && needsPersonaUsersData,
  })
  const personaResolveQuery = useQuery({
    queryKey: controlQueryKeys.personaResolve(selectedChatId, selectedChatUserID, ''),
    queryFn: () => fetchPersonaResolve(selectedChatId, selectedChatUserID, ''),
    enabled: secondaryQueriesEnabled && activePage === 'dashboard' && !!selectedChatId,
    staleTime: 5000,
  })

  const loading = adminSessionQuery.isLoading || (
    secondaryQueriesEnabled &&
    (
      authStatusQuery.isLoading ||
      settingsQuery.isLoading
    )
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

  useEffect(() => {
    if (adminSessionQuery.data) {
      setAdminSession(adminSessionQuery.data)
      if (adminSessionQuery.data.username && !newAdminUsername.trim()) {
        setNewAdminUsername(adminSessionQuery.data.username)
      }
    }
  }, [adminSessionQuery.data, newAdminUsername])

  useEffect(() => {
    if (authStatusQuery.data) {
      setAuthStatus(authStatusQuery.data)
    }
  }, [authStatusQuery.data])

  useEffect(() => {
    if (serviceStatusQuery.data) {
      setServiceStatus(serviceStatusQuery.data)
    }
  }, [serviceStatusQuery.data])

  useEffect(() => {
    if (settingsQuery.data) {
      setSettings(settingsQuery.data)
      setSavedSettings(settingsQuery.data)
    }
  }, [settingsQuery.data])

  useEffect(() => {
    if (soulQuery.data) {
      setSoulText(soulQuery.data.content || '')
      setSoulLoadedAt(new Date().toISOString())
    }
  }, [soulQuery.data])

  useEffect(() => {
    if (logsQuery.data) {
      setLogs(logsQuery.data)
    }
  }, [logsQuery.data])

  useEffect(() => {
    if (agentsQuery.data) {
      setAgents(agentsQuery.data)
    }
  }, [agentsQuery.data])

  useEffect(() => {
    if (variablesQuery.data) {
      setVariables(variablesQuery.data)
    }
  }, [variablesQuery.data])

  useEffect(() => {
    if (personaGroupsQuery.data) {
      const next = personaGroupsQuery.data || []
      setPersonaGroups(next)
      setSelectedPersonaGroupId((prev) => {
        if (prev && next.some((group) => group.id === prev)) {
          return prev
        }
        return next[0]?.id || ''
      })
    }
  }, [personaGroupsQuery.data])

  useEffect(() => {
    if (personaUsersQuery.data) {
      const next = personaUsersQuery.data || []
      setPersonaUsers(next)
      setSelectedPersonaUserId((prev) => {
        if (prev && next.some((user) => user.id === prev)) {
          return prev
        }
        return next[0]?.id || ''
      })
    }
  }, [personaUsersQuery.data])

  useEffect(() => {
    if (personaResolveQuery.data) {
      setResolvedPersona(personaResolveQuery.data)
    } else {
      setResolvedPersona(null)
    }
  }, [personaResolveQuery.data])

  useEffect(() => {
    if (behaviorRuntimeQuery.data) {
      syncBehaviorPolicyState(behaviorRuntimeQuery.data.policy, {
        loadedAt: behaviorRuntimeQuery.data.loadedAt,
        path: behaviorRuntimeQuery.data.path,
        states: behaviorRuntimeQuery.data.states || [],
      })
    }
  }, [behaviorRuntimeQuery.data, syncBehaviorPolicyState])

  useEffect(() => {
    if (conversationsQuery.data) {
      setGlobalAutoReplyEnabled(conversationsQuery.data.globalAutoReplyEnabled)
      setConversations(conversationsQuery.data.conversations || [])
      setSelectedChatId((prev) => {
        if (prev && (conversationsQuery.data?.conversations || []).some((c) => c.chatId === prev)) {
          return prev
        }
        return (conversationsQuery.data?.conversations || [])[0]?.chatId || ''
      })
    }
  }, [conversationsQuery.data])

  useEffect(() => {
    const queryErrors = [
      adminSessionQuery.error,
      authStatusQuery.error,
      serviceStatusQuery.error,
      settingsQuery.error,
      soulQuery.error,
      logsQuery.error,
      agentsQuery.error,
      variablesQuery.error,
      behaviorRuntimeQuery.error,
      conversationsQuery.error,
      personaGroupsQuery.error,
      personaUsersQuery.error,
      personaResolveQuery.error,
    ]
    const firstError = queryErrors.find(Boolean)
    if (firstError) {
      handleRequestError(firstError)
    }
  }, [
    adminSessionQuery.error,
    authStatusQuery.error,
    serviceStatusQuery.error,
    settingsQuery.error,
    soulQuery.error,
    logsQuery.error,
    agentsQuery.error,
    variablesQuery.error,
    behaviorRuntimeQuery.error,
    conversationsQuery.error,
    personaGroupsQuery.error,
    personaUsersQuery.error,
    personaResolveQuery.error,
  ])

  const refreshConversations = async () => {
    const convPayload = await queryClient.fetchQuery({
      queryKey: controlQueryKeys.conversations(),
      queryFn: fetchConversations,
    })
    setGlobalAutoReplyEnabled(convPayload.globalAutoReplyEnabled)
    setConversations(convPayload.conversations || [])
    setSelectedChatId((prev) => {
      if (prev && (convPayload.conversations || []).some((c) => c.chatId === prev)) {
        return prev
      }
      return (convPayload.conversations || [])[0]?.chatId || ''
    })
  }

  const refreshPersonaGroups = async () => {
    const groups = await queryClient.fetchQuery({
      queryKey: controlQueryKeys.personaGroups(),
      queryFn: fetchPersonaGroups,
    })
    setPersonaGroups(groups || [])
    return groups || []
  }

  const refreshPersonaUsers = async () => {
    const users = await queryClient.fetchQuery({
      queryKey: controlQueryKeys.personaUsers(),
      queryFn: fetchPersonaUsers,
    })
    setPersonaUsers(users || [])
    return users || []
  }

  const loadPersonaGroupMembers = async (groupId: string) => {
    if (!groupId) {
      setPersonaGroupMembers([])
      return
    }
    const members = await queryClient.fetchQuery({
      queryKey: controlQueryKeys.personaGroupMembers(groupId),
      queryFn: () => fetchPersonaGroupMembers(groupId),
    })
    setPersonaGroupMembers(members || [])
  }

  const loadPersonaGroupDetail = async (groupId: string) => {
    if (!groupId) {
      setPersonaGroupForm({ id: '', name: '', slug: '', description: '', markdown: '' })
      setPersonaGroupMembers([])
      return
    }
    const payload = await apiRequest<{ group: PersonaGroup; content?: string }>(`/api/persona/groups/${encodeURIComponent(groupId)}`)
    setPersonaGroupForm({
      id: payload.group.id,
      name: payload.group.name || '',
      slug: payload.group.slug || '',
      description: payload.group.description || '',
      markdown: payload.content || '',
    })
    await loadPersonaGroupMembers(groupId)
  }

  const loadPersonaUserDetail = async (userProfileID: string) => {
    if (!userProfileID) {
      setPersonaUserForm({ id: '', label: '', userId: '', username: '', enabled: true, markdown: '' })
      return
    }
    const payload = await apiRequest<{ user: PersonaUserProfile; content?: string }>(`/api/persona/users/${encodeURIComponent(userProfileID)}`)
    setPersonaUserForm({
      id: payload.user.id,
      label: payload.user.label || '',
      userId: payload.user.userId || '',
      username: payload.user.username || '',
      enabled: payload.user.enabled,
      markdown: payload.content || '',
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
    try {
      await queryClient.invalidateQueries({ queryKey: controlQueryKeys.base })
    } catch (err) {
      handleRequestError(err)
    }
  }

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
      queryClient.setQueryData(controlQueryKeys.logs(), (prev: LogEntry[] | undefined) => {
        const current = prev || []
        setLogs((state) => {
          const next = [...state, parsed]
          if (next.length > 400) return next.slice(next.length - 400)
          return next
        })
        const next = [...current, parsed]
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
    if (!selectedPersonaGroupId) {
      void loadPersonaGroupDetail('').catch(handleRequestError)
      return
    }
    void loadPersonaGroupDetail(selectedPersonaGroupId).catch(handleRequestError)
  }, [adminSession.authenticated, selectedPersonaGroupId])

  useEffect(() => {
    if (!adminSession.authenticated) {
      return
    }
    if (!selectedPersonaUserId) {
      void loadPersonaUserDetail('').catch(handleRequestError)
      return
    }
    void loadPersonaUserDetail(selectedPersonaUserId).catch(handleRequestError)
  }, [adminSession.authenticated, selectedPersonaUserId])

  useEffect(() => {
    if (!adminSession.authenticated) {
      return
    }
    const connectedAt = Date.now()
    const eventSource = new EventSource(`${API_BASE}/api/conversations/stream`, { withCredentials: true })
    eventSource.addEventListener('conversation', (event) => {
      let parsed: ConversationStreamEvent | null = null
      void queryClient.invalidateQueries({ queryKey: controlQueryKeys.conversations() })
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

  const savePersonaGroup = async () => {
    if (!personaGroupForm.name.trim()) {
      setMessage('Persona group name is required.')
      return
    }
    setBusy(true)
    setMessage('')
    try {
      const payload = {
        id: personaGroupForm.id.trim(),
        name: personaGroupForm.name.trim(),
        slug: personaGroupForm.slug.trim(),
        description: personaGroupForm.description.trim(),
        markdown: personaGroupForm.markdown,
      }
      const result = await apiRequest<{ group: PersonaGroup }>(
        personaGroupForm.id ? `/api/persona/groups/${encodeURIComponent(personaGroupForm.id)}` : '/api/persona/groups',
        {
          method: personaGroupForm.id ? 'PUT' : 'POST',
          body: JSON.stringify(payload),
        },
      )
      await refreshPersonaGroups()
      setSelectedPersonaGroupId(result.group.id)
      setMessage(personaGroupForm.id ? 'Persona group updated.' : 'Persona group created.')
    } catch (err) {
      handleRequestError(err)
    } finally {
      setBusy(false)
    }
  }

  const deletePersonaGroup = async (groupId: string) => {
    await runConfirmedAction(
      'Delete selected persona group?',
      () => apiRequest(`/api/persona/groups/${encodeURIComponent(groupId)}`, { method: 'DELETE' }).then(() => undefined),
      'Persona group deleted.',
      { title: 'Delete Persona Group', confirmLabel: 'Delete', confirmVariant: 'destructive' },
    )
    const groups = await refreshPersonaGroups()
    if (!groups.some((item) => item.id === selectedPersonaGroupId)) {
      setSelectedPersonaGroupId(groups[0]?.id || '')
    }
  }

  const addPersonaGroupMember = async () => {
    if (!selectedPersonaGroupId) {
      setMessage('Select a persona group first.')
      return
    }
    const userId = personaMemberUserId.trim()
    const username = personaMemberUsername.trim()
    if (!userId && !username) {
      setMessage('Member requires userId or username.')
      return
    }
    await runAction(
      () =>
        apiRequest(`/api/persona/groups/${encodeURIComponent(selectedPersonaGroupId)}/members`, {
          method: 'POST',
          body: JSON.stringify({ userId, username }),
        }).then(() => undefined),
      'Persona group member added.',
    )
    setPersonaMemberUserId('')
    setPersonaMemberUsername('')
    await loadPersonaGroupMembers(selectedPersonaGroupId)
    await refreshPersonaGroups()
  }

  const deletePersonaGroupMember = async (memberID: number) => {
    if (!selectedPersonaGroupId) {
      return
    }
    await runAction(
      () =>
        apiRequest(`/api/persona/groups/${encodeURIComponent(selectedPersonaGroupId)}/members/${memberID}`, {
          method: 'DELETE',
        }).then(() => undefined),
      'Persona group member removed.',
    )
    await loadPersonaGroupMembers(selectedPersonaGroupId)
    await refreshPersonaGroups()
  }

  const savePersonaUser = async () => {
    if (!personaUserForm.label.trim()) {
      setMessage('Persona user label is required.')
      return
    }
    if (!personaUserForm.userId.trim() && !personaUserForm.username.trim()) {
      setMessage('Persona user requires userId or username.')
      return
    }
    setBusy(true)
    setMessage('')
    try {
      const payload = {
        id: personaUserForm.id.trim(),
        label: personaUserForm.label.trim(),
        userId: personaUserForm.userId.trim(),
        username: personaUserForm.username.trim(),
        enabled: personaUserForm.enabled,
        markdown: personaUserForm.markdown,
      }
      const result = await apiRequest<{ user: PersonaUserProfile }>(
        personaUserForm.id ? `/api/persona/users/${encodeURIComponent(personaUserForm.id)}` : '/api/persona/users',
        {
          method: personaUserForm.id ? 'PUT' : 'POST',
          body: JSON.stringify(payload),
        },
      )
      setPersonaUsers((prev) => {
        const next = [result.user, ...prev.filter((item) => item.id !== result.user.id)]
        queryClient.setQueryData(controlQueryKeys.personaUsers(), next)
        return next
      })
      setSelectedPersonaUserId(result.user.id)
      setMessage(personaUserForm.id ? 'Persona user updated.' : 'Persona user created.')
      void refreshPersonaUsers()
    } catch (err) {
      handleRequestError(err)
    } finally {
      setBusy(false)
    }
  }

  const deletePersonaUser = async (id: string) => {
    await runConfirmedAction(
      'Delete selected persona user profile?',
      () => apiRequest(`/api/persona/users/${encodeURIComponent(id)}`, { method: 'DELETE' }).then(() => undefined),
      'Persona user deleted.',
      { title: 'Delete Persona User', confirmLabel: 'Delete', confirmVariant: 'destructive' },
    )
    const users = await refreshPersonaUsers()
    if (!users.some((item) => item.id === selectedPersonaUserId)) {
      setSelectedPersonaUserId(users[0]?.id || '')
    }
  }

  const loadConversationMessages = async (chatId: string) => {
    if (!chatId) return
    try {
      const payload = await queryClient.fetchQuery({
        queryKey: controlQueryKeys.conversationMessages(chatId),
        queryFn: () => fetchConversationMessages(chatId),
      })
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
    if (needsConfig && page !== 'dashboard') {
      setMessage('Complete Step 1 required settings first.')
      void navigate({ to: '/dashboard' })
      setMobileNavOpen(false)
      return
    }
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
    if (needsConfig) {
      setMessage('Complete Step 1 required settings first.')
      void navigate({ to: '/dashboard' })
      setMobileNavOpen(false)
      return
    }
    void navigate({ to: `/settings/${page}` })
    setMobileNavOpen(false)
  }

  useEffect(() => {
    if (!needsConfig) {
      return
    }
    if (activePage !== 'dashboard') {
      void navigate({ to: '/dashboard' })
    }
  }, [needsConfig, activePage, navigate])


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
    personaGroups,
    selectedPersonaGroupId,
    personaGroupMembers,
    personaGroupForm,
    personaMemberUserId,
    personaMemberUsername,
    personaUsers,
    selectedPersonaUserId,
    personaUserForm,
    resolvedPersona,
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
    setSelectedPersonaGroupId,
    setPersonaGroupForm,
    setPersonaMemberUserId,
    setPersonaMemberUsername,
    setSelectedPersonaUserId,
    setPersonaUserForm,
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
    savePersonaGroup,
    deletePersonaGroup,
    addPersonaGroupMember,
    deletePersonaGroupMember,
    savePersonaUser,
    deletePersonaUser,
    logoutTelegram,
    openPage,
    openSettingsPage,
    handleConfirmAction,
    closeConfirmDialog,
  }
}
