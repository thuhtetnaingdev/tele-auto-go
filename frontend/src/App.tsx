import { useEffect, useMemo, useState } from 'react'
import {
  Activity,
  CheckCircle2,
  KeyRound,
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

type AdminSession = {
  configured: boolean
  authenticated: boolean
  username?: string
}

const API_BASE =
  (import.meta.env.VITE_API_BASE_URL as string | undefined)?.trim() ||
  (typeof window !== 'undefined' ? window.location.origin : 'http://localhost:3000')

const ONBOARDING_REQUIRED_KEYS = ['TG_API_ID', 'TG_API_HASH', 'TG_PHONE', 'OPENAI_BASE_URL', 'OPENAI_API_KEY', 'OPENAI_MODEL']
const MAIN_VISIBLE_SETTINGS = ['TG_PHONE', 'OPENAI_BASE_URL', 'OPENAI_API_KEY', 'OPENAI_MODEL', 'AUTO_REPLY_ENABLED']

const booleanSettingKeys = new Set(['AUTO_REPLY_ENABLED'])
const secretSettingKeys = new Set(['OPENAI_API_KEY', 'TG_API_HASH'])

const settingLabels: Record<string, string> = {
  TG_API_ID: 'Telegram API ID',
  TG_API_HASH: 'Telegram API Hash',
  TG_PHONE: 'Telegram Phone',
  OPENAI_BASE_URL: 'OpenAI Base URL',
  OPENAI_API_KEY: 'OpenAI API Key',
  OPENAI_MODEL: 'OpenAI Model',
  AUTO_REPLY_ENABLED: 'Auto Reply Enabled',
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

export default function App() {
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')

  const [adminSession, setAdminSession] = useState<AdminSession>({ configured: false, authenticated: true })
  const [adminUsername, setAdminUsername] = useState('')
  const [adminPassword, setAdminPassword] = useState('')

  const [authStatus, setAuthStatus] = useState<AuthStatus>({ authorized: false, configured: false })
  const [serviceStatus, setServiceStatus] = useState<ServiceStatus>({ running: false })
  const [settings, setSettings] = useState<SettingsResponse>({ keys: [], values: {} })
  const [soulText, setSoulText] = useState('')

  const [phone, setPhone] = useState('')
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')

  const [logs, setLogs] = useState<LogEntry[]>([])

  const requiredMissing = useMemo(
    () => ONBOARDING_REQUIRED_KEYS.filter((key) => !(settings.values[key] || '').trim()),
    [settings.values],
  )

  const needsConfig = requiredMissing.length > 0
  const needsAuth = !authStatus.authorized
  const needsAppLogin = adminSession.configured && !adminSession.authenticated
  const showOnboarding = !loading && !needsAppLogin && (needsConfig || needsAuth)

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

      if (admin.configured && !admin.authenticated) {
        setLoading(false)
        return
      }

      const [auth, service, settingsData, soul, logPayload] = await Promise.all([
        apiRequest<AuthStatus>('/api/auth/status'),
        apiRequest<ServiceStatus>('/api/service/status'),
        apiRequest<SettingsResponse>('/api/settings'),
        apiRequest<{ content: string }>('/api/soul'),
        apiRequest<{ logs: LogEntry[] }>('/api/logs?limit=200'),
      ])
      setAuthStatus(auth)
      setServiceStatus(service)
      setSettings(settingsData)
      setSoulText(soul.content || '')
      setLogs(logPayload.logs || [])
      if (!phone.trim() && (settingsData.values.TG_PHONE || '').trim()) {
        setPhone(settingsData.values.TG_PHONE)
      }
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
    if (adminSession.configured && !adminSession.authenticated) {
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
  }, [adminSession.configured, adminSession.authenticated])

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

  const saveSettingsSubset = async (keys: string[], successText: string) => {
    const values: Record<string, string> = {}
    for (const key of keys) {
      values[key] = settings.values[key] ?? ''
    }
    await runAction(
      () => apiRequest('/api/settings', { method: 'PUT', body: JSON.stringify({ values }) }).then(() => undefined),
      successText,
    )
  }

  const requestOtp = async () => {
    if (!phone.trim()) {
      setMessage('Phone is required.')
      return
    }

    setBusy(true)
    setMessage('')
    try {
      const result = await apiRequest<LoginResponse>('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify({ phone }),
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
    if (!code.trim()) {
      setMessage('OTP code is required for verify login.')
      return
    }

    await runAction(
      () =>
        apiRequest<LoginResponse>('/api/auth/login', {
          method: 'POST',
          body: JSON.stringify({ phone, code, password }),
        }).then(() => undefined),
      'Login verified and session saved.',
    )
  }

  const updateSetting = (key: string, value: string) => {
    setSettings((prev) => ({
      ...prev,
      values: {
        ...prev.values,
        [key]: value,
      },
    }))
  }

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
              <CardDescription>Sign in with admin username and password configured during installation.</CardDescription>
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
              <Button onClick={() => void loginAdmin()} disabled={busy} className="w-full">
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
                First we configure required credentials, then verify Telegram login. After that, dashboard will open.
              </p>
              <div className="mt-4 flex flex-wrap gap-2">
                <Badge variant={!needsConfig ? 'default' : 'secondary'}>{!needsConfig ? 'Step 1 Done' : 'Step 1: Configure App'}</Badge>
                <Badge variant={!needsAuth ? 'default' : 'secondary'}>{!needsAuth ? 'Step 2 Done' : 'Step 2: Verify Telegram'}</Badge>
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
                      value={settings.values[key] ?? ''}
                      onChange={(event) => updateSetting(key, event.target.value)}
                    />
                  </div>
                ))}
                <div className="md:col-span-2">
                  <Button onClick={() => void saveSettingsSubset(ONBOARDING_REQUIRED_KEYS, 'Settings saved. Continue to Telegram verify.')} disabled={busy}>
                    <Save className="size-4" /> Save & Continue
                  </Button>
                </div>
              </CardContent>
            </Card>

            <Card className={needsConfig ? 'opacity-60' : ''}>
              <CardHeader>
                <CardTitle>Step 2: Telegram Verify</CardTitle>
                <CardDescription>Request OTP, then verify login with OTP code and optional 2FA password.</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="onboard-phone">Phone</Label>
                  <div className="flex flex-col gap-2 sm:flex-row">
                    <Input
                      id="onboard-phone"
                      className="sm:flex-1"
                      placeholder="+1..."
                      value={phone}
                      onChange={(event) => setPhone(event.target.value)}
                      disabled={needsConfig}
                    />
                    <Button onClick={() => void requestOtp()} disabled={busy || needsConfig || !phone.trim()}>
                      <KeyRound className="size-4" /> Request OTP
                    </Button>
                  </div>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="onboard-code">OTP Code</Label>
                  <Input
                    id="onboard-code"
                    placeholder="Telegram OTP"
                    value={code}
                    onChange={(event) => setCode(event.target.value)}
                    disabled={needsConfig}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="onboard-password">2FA Password</Label>
                  <Input
                    id="onboard-password"
                    type="password"
                    placeholder="Optional"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    disabled={needsConfig}
                  />
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button onClick={() => void verifyLogin()} disabled={busy || needsConfig}>
                    <ShieldCheck className="size-4" /> Verify Login
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => void runAction(() => apiRequest('/api/auth/logout', { method: 'POST' }), 'Telegram logged out')}
                    disabled={busy || needsConfig}
                  >
                    <LogOut className="size-4" /> Telegram Logout
                  </Button>
                </div>
                {!authStatus.configured && authStatus.error ? <p className="text-xs text-destructive">{authStatus.error}</p> : null}
              </CardContent>
            </Card>

            {!needsConfig && !needsAuth ? (
              <Card>
                <CardContent className="flex items-center justify-between gap-3 p-6">
                  <p className="text-sm text-muted-foreground">Setup is complete. Dashboard unlocked.</p>
                  <Badge>
                    <CheckCircle2 className="mr-1 size-3" /> Ready
                  </Badge>
                </CardContent>
              </Card>
            ) : null}
          </section>
        ) : (
          <>
            <header className="relative overflow-hidden rounded-2xl border border-border/60 bg-gradient-to-r from-amber-100 via-orange-50 to-teal-100 p-6 shadow-sm dark:from-amber-950/30 dark:via-background dark:to-teal-900/20">
              <div className="space-y-2">
                <h1 className="font-display text-3xl tracking-tight">Tele Auto Control Center</h1>
                <p className="max-w-3xl text-sm text-muted-foreground">Backend control for worker state, logs, app settings, and SOUL prompt.</p>
              </div>
              <div className="mt-4 flex flex-wrap items-center gap-2">
                <Badge variant={authStatus.authorized ? 'default' : 'secondary'}>
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
              <div className="mt-4 flex flex-wrap gap-2">
                <Button variant="outline" onClick={() => void refreshState()} disabled={busy || loading}>
                  <RefreshCw className="size-4" /> Refresh
                </Button>
                <Button
                  onClick={() => void runAction(() => apiRequest('/api/service/start', { method: 'POST' }), 'Worker started')}
                  disabled={busy || serviceStatus.running}
                >
                  <Play className="size-4" /> Start
                </Button>
                <Button
                  variant="secondary"
                  onClick={() => void runAction(() => apiRequest('/api/service/restart', { method: 'POST' }), 'Worker restarted')}
                  disabled={busy || !serviceStatus.running}
                >
                  <RefreshCw className="size-4" /> Restart
                </Button>
                <Button
                  variant="destructive"
                  onClick={() => void runAction(() => apiRequest('/api/service/stop', { method: 'POST' }), 'Worker stopped')}
                  disabled={busy || !serviceStatus.running}
                >
                  <Square className="size-4" /> Stop
                </Button>
                <Button variant="outline" onClick={() => void logoutAdmin()} disabled={busy}>
                  <LogOut className="size-4" /> Sign Out
                </Button>
              </div>
              {message ? <p className="mt-3 text-sm text-primary">{message}</p> : null}
            </header>

            <section className="grid gap-6 lg:grid-cols-2">
              <Card>
                <CardHeader>
                  <CardTitle>Settings</CardTitle>
                  <CardDescription>Only daily-use settings are shown. Advanced values are hidden and fixed.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  {MAIN_VISIBLE_SETTINGS.filter((key) => settings.keys.includes(key)).map((key) => {
                    const value = settings.values[key] ?? ''
                    if (booleanSettingKeys.has(key)) {
                      return (
                        <div key={key} className="flex items-center justify-between rounded-lg border border-border/60 bg-background p-3">
                          <div>
                            <Label>{settingLabels[key] || key}</Label>
                            <p className="text-xs text-muted-foreground">Feature toggle</p>
                          </div>
                          <Switch checked={value.toLowerCase() === 'true'} onCheckedChange={(checked) => updateSetting(key, checked ? 'true' : 'false')} />
                        </div>
                      )
                    }
                    return (
                      <div key={key} className="space-y-2">
                        <Label>{settingLabels[key] || key}</Label>
                        <Input
                          type={secretSettingKeys.has(key) ? 'password' : 'text'}
                          value={value}
                          onChange={(event) => updateSetting(key, event.target.value)}
                        />
                      </div>
                    )
                  })}
                  <Button onClick={() => void saveSettingsSubset(MAIN_VISIBLE_SETTINGS.filter((key) => settings.keys.includes(key)), 'Settings saved')} disabled={busy}>
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
                  <Textarea value={soulText} onChange={(event) => setSoulText(event.target.value)} className="min-h-[250px] font-mono text-xs" />
                  <Button
                    onClick={() =>
                      void runAction(
                        () => apiRequest('/api/soul', { method: 'PUT', body: JSON.stringify({ content: soulText }) }).then(() => undefined),
                        'SOUL updated',
                      )
                    }
                    disabled={busy}
                  >
                    <Save className="size-4" /> Save SOUL
                  </Button>
                </CardContent>
              </Card>
            </section>

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Terminal className="size-4" /> Realtime Logs
                </CardTitle>
                <CardDescription>Streaming from backend SSE endpoint `/api/logs/stream`.</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="max-h-[380px] overflow-auto rounded-lg border border-border/60 bg-zinc-950 p-3 font-mono text-xs text-zinc-100">
                  {logs.map((entry, index) => (
                    <div key={`${entry.time}-${index}`} className="mb-1 break-all">
                      <span className="text-zinc-400">[{entry.time}]</span>{' '}
                      <span className={entry.level.includes('error') ? 'text-rose-300' : entry.level.includes('warn') ? 'text-amber-300' : 'text-emerald-300'}>
                        {entry.level.toUpperCase()}
                      </span>{' '}
                      <span>{entry.message}</span>
                    </div>
                  ))}
                  {logs.length === 0 ? <p className="text-zinc-400">No logs yet.</p> : null}
                </div>
              </CardContent>
            </Card>
          </>
        )}
      </div>
    </main>
  )
}
