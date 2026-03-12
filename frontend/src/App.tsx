import { useEffect, useMemo, useState } from 'react'
import {
  Activity,
  Bot,
  Clock3,
  KeyRound,
  Lock,
  LogOut,
  Play,
  RefreshCw,
  Save,
  ShieldCheck,
  Square,
  Terminal,
  UserCircle2,
  WandSparkles,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

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

const API_BASE_RAW = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.trim()
const API_BASE = API_BASE_RAW ? API_BASE_RAW.replace(/\/+$/, '') : ''

const ONBOARDING_REQUIRED_KEYS = ['TG_API_ID', 'TG_API_HASH', 'OPENAI_BASE_URL', 'OPENAI_API_KEY', 'OPENAI_MODEL']
const MAIN_VISIBLE_SETTINGS = ['OPENAI_BASE_URL', 'OPENAI_API_KEY', 'OPENAI_MODEL', 'AI_CONTEXT_MESSAGE_LIMIT', 'AUTO_REPLY_ENABLED']

const booleanSettingKeys = new Set(['AUTO_REPLY_ENABLED'])
const numericSettingKeys = new Set(['AI_CONTEXT_MESSAGE_LIMIT'])
const secretSettingKeys = new Set(['OPENAI_API_KEY', 'TG_API_HASH'])

const settingLabels: Record<string, string> = {
  TG_API_ID: 'Telegram API ID',
  TG_API_HASH: 'Telegram API Hash',
  OPENAI_BASE_URL: 'OpenAI Base URL',
  OPENAI_API_KEY: 'OpenAI API Key',
  OPENAI_MODEL: 'OpenAI Model',
  AI_CONTEXT_MESSAGE_LIMIT: 'Context Message Limit',
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
  const [soulText, setSoulText] = useState('')
  const [soulLoadedAt, setSoulLoadedAt] = useState('')
  const [soulSavedAt, setSoulSavedAt] = useState('')

  const [phone, setPhone] = useState(readInitialPhone)
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')

  const [logs, setLogs] = useState<LogEntry[]>([])
  const [logMode, setLogMode] = useState<'all' | 'orchestrator'>('all')

  const [agents, setAgents] = useState<AgentDefinition[]>([])
  const [editingAgentID, setEditingAgentID] = useState('')
  const [agentForm, setAgentForm] = useState<AgentDefinition>({
    id: '',
    name: '',
    description: '',
    intents: [],
    tools: ['api_call'],
    variables: [],
    model: '',
    temperature: 0.35,
    body: '',
  })

  const [variables, setVariables] = useState<VariableValue[]>([])
  const [variableForm, setVariableForm] = useState<VariableValue>({ key: '', type: 'text', value: '' })

  const settingsValues = settings && settings.values && typeof settings.values === 'object' ? settings.values : {}
  const settingsKeys = settings && Array.isArray(settings.keys) ? settings.keys : []
  const requiredMissing = useMemo(
    () => ONBOARDING_REQUIRED_KEYS.filter((key) => !(settingsValues[key] || '').trim()),
    [settingsValues],
  )

  const needsConfig = requiredMissing.length > 0
  const needsAuth = !authStatus.authorized
  const needsAppLogin = !adminSession.authenticated
  const showOnboarding = !loading && !needsAppLogin && needsConfig
  const canManageWorker = authStatus.authorized
  const telegramPhoneLabel = normalizePhoneWithPlus(phone) || 'Not set'
  const soulCharacterCount = soulText.length

  const handleRequestError = (err: unknown) => {
    const text = (err as Error).message
    if (text.toLowerCase().includes('unauthorized')) {
      setAdminSession((prev) => ({ ...prev, authenticated: false }))
    }
    setMessage(text)
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
      setSoulText(soul.content || '')
      setSoulLoadedAt(new Date().toISOString())
      setLogs(logPayload.logs || [])
      setAgents(agentsPayload.agents || [])
      setVariables(varsPayload.values || [])
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

  const runAction = async (runner: () => Promise<void>, successText: string) => {
    setBusy(true)
    setMessage('')
    try {
      await runner()
      setMessage(successText)
      await refreshState()
    } catch (err) {
      handleRequestError(err)
    } finally {
      setBusy(false)
    }
  }

  const runConfirmedAction = async (confirmText: string, runner: () => Promise<void>, successText: string) => {
    if (!window.confirm(confirmText)) {
      return
    }
    await runAction(runner, successText)
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
    setAgentForm({
      id: '',
      name: '',
      description: '',
      intents: [],
      tools: ['api_call'],
      variables: [],
      model: '',
      temperature: 0.35,
      body: '',
    })
  }

  const startEditAgent = (agent: AgentDefinition) => {
    setEditingAgentID(agent.id)
    setAgentForm({
      ...agent,
      intents: agent.intents || [],
      tools: ['api_call'],
      variables: agent.variables || [],
      body: agent.body || '',
      temperature: agent.temperature || 0.35,
    })
  }

  const saveAgent = async () => {
    if (!agentForm.id.trim() || !agentForm.name.trim() || !agentForm.body.trim()) {
      setMessage('Agent id, name, and body are required.')
      return
    }
    const payload = {
      ...agentForm,
      id: agentForm.id.trim(),
      name: agentForm.name.trim(),
      description: agentForm.description.trim(),
      intents: agentForm.intents,
      tools: ['api_call'],
      variables: agentForm.variables,
      model: (agentForm.model || '').trim(),
      body: agentForm.body.trim(),
      temperature: Number(agentForm.temperature || 0.35),
    }
    const isEdit = editingAgentID === payload.id
    const path = isEdit ? `/api/agents/${payload.id}` : '/api/agents'
    const method = isEdit ? 'PUT' : 'POST'
    await runAction(
      () => apiRequest(path, { method, body: JSON.stringify(payload) }).then(() => undefined),
      isEdit ? 'Agent updated' : 'Agent created',
    )
    resetAgentForm()
  }

  const deleteAgent = async (id: string) => {
    await runConfirmedAction(
      `Delete agent "${id}"?`,
      () => apiRequest(`/api/agents/${id}`, { method: 'DELETE' }).then(() => undefined),
      'Agent deleted',
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

  const filteredLogs = useMemo(() => {
    if (logMode === 'all') return logs
    return logs.filter((entry) => `${entry.message} ${JSON.stringify(entry.attrs || {})}`.toLowerCase().includes('orchestrator'))
  }, [logs, logMode])

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
    <main className="min-h-screen bg-background px-4 py-8 text-foreground">
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
            <header className="relative overflow-hidden rounded-2xl border border-border/60 bg-gradient-to-r from-amber-100 via-orange-50 to-teal-100 p-6 shadow-sm dark:from-amber-950/30 dark:via-background dark:to-teal-900/20">
              <div className="space-y-2">
                <h1 className="font-display text-3xl tracking-tight">Tele Auto Control Center</h1>
                <p className="max-w-3xl text-sm text-muted-foreground">AI system console for worker state, Telegram auth, settings, and runtime logs.</p>
              </div>
              <div className="mt-4 flex flex-wrap items-center gap-2">
                <Badge variant={authStatus.authorized ? 'default' : 'destructive'}>
                  <ShieldCheck className="mr-1 size-3" />
                  {authStatus.authorized ? 'Telegram Authorized' : 'Not Authorized'}
                </Badge>
                <Badge variant={serviceStatus.running ? 'default' : 'outline'}>
                  <Activity className="mr-1 size-3" />
                  {serviceStatus.running ? `Worker Running (${formatUptime(serviceStatus.uptimeSec)})` : 'Worker Stopped'}
                </Badge>
                {adminSession.username ? <Badge variant="secondary">Signed in as {adminSession.username}</Badge> : null}
                {serviceStatus.lastError ? <Badge variant="destructive">Last Error: {serviceStatus.lastError}</Badge> : null}
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
                  <p className="mt-1 text-sm font-semibold">{authStatus.authorized ? 'Connected' : 'Needs Verify'}</p>
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
                <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border/60 bg-background/60 p-2">
                  <Button size="sm" variant="outline" onClick={() => void refreshState()} disabled={busy || loading}>
                    <RefreshCw className="size-4" /> Refresh
                  </Button>
                  {canManageWorker ? (
                    <>
                      <Button
                        size="sm"
                        variant="success"
                        onClick={() => void runAction(() => apiRequest('/api/service/start', { method: 'POST' }), 'Worker started')}
                        disabled={busy || serviceStatus.running}
                      >
                        <Play className="size-4" /> Start
                      </Button>
                      <Button
                        size="sm"
                        variant="info"
                        onClick={() =>
                          void runConfirmedAction(
                            'Restart worker now?',
                            () => apiRequest('/api/service/restart', { method: 'POST' }).then(() => undefined),
                            'Worker restarted',
                          )
                        }
                        disabled={busy || !serviceStatus.running}
                      >
                        <RefreshCw className="size-4" /> Restart
                      </Button>
                      <Button
                        size="sm"
                        variant="destructive"
                        onClick={() =>
                          void runConfirmedAction(
                            'Stop worker now?',
                            () => apiRequest('/api/service/stop', { method: 'POST' }).then(() => undefined),
                            'Worker stopped',
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
                <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border/60 bg-background/60 p-2">
                  {authStatus.authorized ? (
                    <Button size="sm" variant="outline" onClick={() => void logoutTelegram()} disabled={busy}>
                      <LogOut className="size-4" /> Telegram Sign Out
                    </Button>
                  ) : null}
                  <Button size="sm" variant="outline" onClick={() => void logoutAdmin()} disabled={busy}>
                    <UserCircle2 className="size-4" /> Dashboard Sign Out
                  </Button>
                </div>
              </div>
              {message ? <p className="mt-3 text-sm text-primary">{message}</p> : null}
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
              <>
                <section className="grid gap-6 lg:grid-cols-2">
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
                </section>

                <section className="grid gap-6 lg:grid-cols-2">
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
                          <div key={v.key} className="flex items-center justify-between border-b border-border/40 py-1 last:border-b-0">
                            <div className="min-w-0">
                              <p className="font-medium">{v.key}</p>
                              <p className="text-xs text-muted-foreground">
                                {v.type} • {v.value}
                              </p>
                            </div>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => setVariableForm({ key: v.key, type: v.type, value: '' })}
                            >
                              Edit
                            </Button>
                          </div>
                        ))}
                        {variables.length === 0 ? <p className="text-xs text-muted-foreground">No variables yet.</p> : null}
                      </div>
                    </CardContent>
                  </Card>

                  <Card>
                    <CardHeader>
                      <CardTitle>Agents</CardTitle>
                      <CardDescription>Create Markdown-based agents with frontmatter metadata.</CardDescription>
                    </CardHeader>
                    <CardContent className="space-y-3">
                      <div className="grid gap-2 md:grid-cols-2">
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
                          className="h-9 md:col-span-2"
                        />
                        <Input
                          placeholder="intents: price,order,info"
                          value={agentForm.intents.join(',')}
                          onChange={(event) =>
                            setAgentForm((prev) => ({
                              ...prev,
                              intents: event.target.value.split(',').map((v) => v.trim()).filter(Boolean),
                            }))
                          }
                          className="h-9"
                        />
                        <Input
                          placeholder="variables: API_TOKEN,BASE_URL"
                          value={agentForm.variables.join(',')}
                          onChange={(event) =>
                            setAgentForm((prev) => ({
                              ...prev,
                              variables: event.target.value.split(',').map((v) => v.trim().toUpperCase()).filter(Boolean),
                            }))
                          }
                          className="h-9"
                        />
                      </div>
                      <Textarea
                        placeholder="Agent markdown body instructions..."
                        value={agentForm.body}
                        onChange={(event) => setAgentForm((prev) => ({ ...prev, body: event.target.value }))}
                        className="min-h-[160px] font-mono text-xs"
                      />
                      <div className="flex flex-wrap gap-2">
                        <Button size="sm" onClick={() => void saveAgent()} disabled={busy}>
                          <Save className="size-4" /> {editingAgentID ? 'Update Agent' : 'Create Agent'}
                        </Button>
                        <Button size="sm" variant="outline" onClick={resetAgentForm} disabled={busy}>
                          Clear
                        </Button>
                      </div>
                      <div className="max-h-52 overflow-auto rounded-lg border border-border/60 bg-background p-2 text-sm">
                        {agents.map((agent) => (
                          <div key={agent.id} className="flex items-center justify-between border-b border-border/40 py-1 last:border-b-0">
                            <div className="min-w-0">
                              <p className="font-medium">{agent.id}</p>
                              <p className="truncate text-xs text-muted-foreground">{agent.description || agent.name}</p>
                            </div>
                            <div className="flex gap-1">
                              <Button size="sm" variant="outline" onClick={() => startEditAgent(agent)}>Edit</Button>
                              <Button size="sm" variant="destructive" onClick={() => void deleteAgent(agent.id)}>Delete</Button>
                            </div>
                          </div>
                        ))}
                        {agents.length === 0 ? <p className="text-xs text-muted-foreground">No agents yet.</p> : null}
                      </div>
                    </CardContent>
                  </Card>
                </section>

                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <Terminal className="size-4" /> Realtime Logs
                    </CardTitle>
                    <CardDescription>Streaming from backend SSE endpoint `/api/logs/stream`.</CardDescription>
                    <div className="flex gap-2">
                      <Button size="sm" variant={logMode === 'all' ? 'default' : 'outline'} onClick={() => setLogMode('all')}>
                        All
                      </Button>
                      <Button size="sm" variant={logMode === 'orchestrator' ? 'default' : 'outline'} onClick={() => setLogMode('orchestrator')}>
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
                        </div>
                      ))}
                      {filteredLogs.length === 0 ? <p className="text-zinc-400">No logs yet.</p> : null}
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>Account Security</CardTitle>
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
                    <div className="md:col-span-2 flex flex-wrap items-center gap-2">
                      <Button size="sm" onClick={() => void updateAdminCredentials()} disabled={busy}>
                        <ShieldCheck className="size-4" /> Update Dashboard Account
                      </Button>
                      <span className="text-xs text-muted-foreground">After update, dashboard login is required again.</span>
                    </div>
                  </CardContent>
                </Card>
              </>
            )}
          </>
        )}
      </div>
    </main>
  )
}
