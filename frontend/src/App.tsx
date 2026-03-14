import {
  Activity,
  Bot,
  Clock3,
  KeyRound,
  Lock,
  LogOut,
  Menu,
  Play,
  RefreshCw,
  Save,
  ShieldCheck,
  Square,
  UserCircle2,
  WandSparkles,
  X,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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
import {
  ONBOARDING_REQUIRED_KEYS,
  secretSettingKeys,
  settingLabels,
} from '@/features/control/constants'
import { apiRequest } from '@/features/control/api'
import { AdminLoginView } from '@/features/control/components/AdminLoginView'
import { AgentsCard } from '@/features/control/components/AgentsCard'
import { LogsCard } from '@/features/control/components/LogsCard'
import { NavigationMenu } from '@/features/control/components/NavigationMenu'
import { RealtimeConversationsCard } from '@/features/control/components/RealtimeConversationsCard'
import { SettingsPanels } from '@/features/control/components/SettingsPanels'
import { useControlCenter } from '@/features/control/hooks/useControlCenter'
import { formatLocalTime, formatUptime, normalizePhoneWithPlus } from '@/features/control/utils'

export default function App() {
  const {
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
    settingsValues,
    settingsKeys,
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
  } = useControlCenter()
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
      <AdminLoginView
        adminSession={adminSession}
        adminUsername={adminUsername}
        adminPassword={adminPassword}
        busy={busy}
        message={message}
        onAdminUsernameChange={setAdminUsername}
        onAdminPasswordChange={setAdminPassword}
        onLogin={loginAdmin}
      />
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
                  <div className="space-y-4">
                    <NavigationMenu
                      activePage={activePage}
                      activeSettingsPage={activeSettingsPage}
                      onOpenPage={openPage}
                      onOpenSettingsPage={openSettingsPage}
                    />
                  </div>
                </aside>

                <div className="grid items-start gap-6 xl:grid-cols-[250px_minmax(0,1fr)]">
                  <aside className="hidden self-start space-y-4 rounded-2xl border border-border/60 bg-card/70 p-3 xl:sticky xl:top-24 xl:block">
                    <NavigationMenu
                      activePage={activePage}
                      activeSettingsPage={activeSettingsPage}
                      onOpenPage={openPage}
                      onOpenSettingsPage={openSettingsPage}
                    />
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
                      <RealtimeConversationsCard
                        globalAutoReplyEnabled={globalAutoReplyEnabled}
                        notificationPermission={notificationPermission}
                        onEnableNotifications={requestBrowserNotifications}
                        conversations={conversations}
                        selectedChatId={selectedChatId}
                        onSelectChatId={setSelectedChatId}
                        selectedConversation={selectedConversation}
                        busy={busy}
                        onSetConversationMode={setConversationMode}
                        activeMessages={activeMessages}
                        conversationThreadRef={conversationThreadRef}
                        formatLocalTime={formatLocalTime}
                        manualReplyText={manualReplyText}
                        onManualReplyTextChange={setManualReplyText}
                        onSendManualReply={sendManualReply}
                        resolvedPersona={resolvedPersona?.resolved}
                      />
                    </section>
                  ) : null}

                  {activePage === 'logs' ? (
                    <LogsCard
                      logMode={logMode}
                      onLogModeChange={setLogMode}
                      filteredLogs={filteredLogs}
                    />
                  ) : null}

                  {activePage === 'agents' ? (
                    <AgentsCard
                      busy={busy}
                      agents={agents}
                      editingAgentID={editingAgentID}
                      allowUsersInput={allowUsersInput}
                      agentForm={agentForm}
                      onAgentFormChange={setAgentForm}
                      onAllowUsersInputChange={setAllowUsersInput}
                      onSaveAgent={saveAgent}
                      onResetAgentForm={resetAgentForm}
                      onStartEditAgent={startEditAgent}
                      onDeleteAgent={deleteAgent}
                    />
                  ) : null}

                  <SettingsPanels
                    activePage={activePage}
                    activeSettingsPage={activeSettingsPage}
                    settingsKeys={settingsKeys}
                    settingsValues={settingsValues}
                    updateSetting={updateSetting}
                    saveSettingsSubset={saveSettingsSubset}
                    busy={busy}
                    soulCharacterCount={soulCharacterCount}
                    soulLoadedAt={soulLoadedAt}
                    soulSavedAt={soulSavedAt}
                    soulText={soulText}
                    onSoulTextChange={setSoulText}
                    onSaveSoul={saveSoul}
                    formatLocalTime={formatLocalTime}
                    behaviorPolicy={behaviorPolicy}
                    behaviorLoadedAt={behaviorLoadedAt}
                    behaviorPath={behaviorPath}
                    behaviorRuntimeStates={behaviorRuntimeStates}
                    behaviorQuietHoursInput={behaviorQuietHoursInput}
                    behaviorAllowTonesInput={behaviorAllowTonesInput}
                    behaviorDenyTonesInput={behaviorDenyTonesInput}
                    behaviorTriggerKeywordsInput={behaviorTriggerKeywordsInput}
                    onBehaviorPolicyChange={setBehaviorPolicy}
                    onBehaviorQuietHoursInputChange={updateBehaviorQuietHoursInput}
                    onBehaviorAllowTonesInputChange={updateBehaviorAllowTonesInput}
                    onBehaviorDenyTonesInputChange={updateBehaviorDenyTonesInput}
                    onBehaviorTriggerKeywordsInputChange={updateBehaviorTriggerKeywordsInput}
                    onSaveBehaviorPolicy={saveBehaviorPolicy}
                    telegramConnectionLabel={telegramConnection.label}
                    telegramPhoneLabel={telegramPhoneLabel}
                    authAuthorized={authStatus.authorized}
                    onLogoutTelegram={logoutTelegram}
                    variableForm={variableForm}
                    onVariableFormChange={setVariableForm}
                    onSaveVariable={saveVariable}
                    variables={variables}
                    onDeleteVariable={deleteVariable}
                    personaGroups={personaGroups}
                    selectedPersonaGroupId={selectedPersonaGroupId}
                    onSelectPersonaGroupId={setSelectedPersonaGroupId}
                    personaGroupMembers={personaGroupMembers}
                    personaGroupForm={personaGroupForm}
                    onPersonaGroupFormChange={setPersonaGroupForm}
                    onSavePersonaGroup={savePersonaGroup}
                    onDeletePersonaGroup={deletePersonaGroup}
                    personaMemberUserId={personaMemberUserId}
                    onPersonaMemberUserIdChange={setPersonaMemberUserId}
                    personaMemberUsername={personaMemberUsername}
                    onPersonaMemberUsernameChange={setPersonaMemberUsername}
                    onAddPersonaGroupMember={addPersonaGroupMember}
                    onDeletePersonaGroupMember={deletePersonaGroupMember}
                    personaUsers={personaUsers}
                    selectedPersonaUserId={selectedPersonaUserId}
                    onSelectPersonaUserId={setSelectedPersonaUserId}
                    personaUserForm={personaUserForm}
                    onPersonaUserFormChange={setPersonaUserForm}
                    onSavePersonaUser={savePersonaUser}
                    onDeletePersonaUser={deletePersonaUser}
                    currentAdminPassword={currentAdminPassword}
                    newAdminUsername={newAdminUsername}
                    newAdminPassword={newAdminPassword}
                    confirmAdminPassword={confirmAdminPassword}
                    onCurrentAdminPasswordChange={setCurrentAdminPassword}
                    onNewAdminUsernameChange={setNewAdminUsername}
                    onNewAdminPasswordChange={setNewAdminPassword}
                    onConfirmAdminPasswordChange={setConfirmAdminPassword}
                    onUpdateAdminCredentials={updateAdminCredentials}
                  />
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
