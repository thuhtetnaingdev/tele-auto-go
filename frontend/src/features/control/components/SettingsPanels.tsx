import { LogOut, Save, ShieldCheck } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  booleanSettingKeys,
  MAIN_VISIBLE_SETTINGS,
  numericSettingKeys,
  secretSettingKeys,
  settingLabels,
} from '@/features/control/constants'
import type { BehaviorPolicy, BehaviorRuntimeState, MainPage, SettingsPage, VariableValue } from '@/features/control/types'

type SettingsPanelsProps = {
  activePage: MainPage
  activeSettingsPage: SettingsPage
  settingsKeys: string[]
  settingsValues: Record<string, string>
  updateSetting: (key: string, value: string) => void
  saveSettingsSubset: (keys: string[], successText: string) => Promise<void>
  busy: boolean
  soulCharacterCount: number
  soulLoadedAt: string
  soulSavedAt: string
  soulText: string
  onSoulTextChange: (value: string) => void
  onSaveSoul: () => Promise<void>
  formatLocalTime: (value?: string) => string
  behaviorPolicy: BehaviorPolicy
  behaviorLoadedAt: string
  behaviorPath: string
  behaviorRuntimeStates: BehaviorRuntimeState[]
  behaviorQuietHoursInput: string
  behaviorAllowTonesInput: string
  behaviorDenyTonesInput: string
  behaviorTriggerKeywordsInput: string
  onBehaviorPolicyChange: (next: BehaviorPolicy) => void
  onBehaviorQuietHoursInputChange: (value: string) => void
  onBehaviorAllowTonesInputChange: (value: string) => void
  onBehaviorDenyTonesInputChange: (value: string) => void
  onBehaviorTriggerKeywordsInputChange: (value: string) => void
  onSaveBehaviorPolicy: () => Promise<void>
  telegramConnectionLabel: string
  telegramPhoneLabel: string
  authAuthorized: boolean
  onLogoutTelegram: () => Promise<void>
  variableForm: VariableValue
  onVariableFormChange: (next: VariableValue) => void
  onSaveVariable: () => Promise<void>
  variables: VariableValue[]
  onDeleteVariable: (key: string) => Promise<void>
  currentAdminPassword: string
  newAdminUsername: string
  newAdminPassword: string
  confirmAdminPassword: string
  onCurrentAdminPasswordChange: (value: string) => void
  onNewAdminUsernameChange: (value: string) => void
  onNewAdminPasswordChange: (value: string) => void
  onConfirmAdminPasswordChange: (value: string) => void
  onUpdateAdminCredentials: () => Promise<void>
}

export function SettingsPanels({
  activePage,
  activeSettingsPage,
  settingsKeys,
  settingsValues,
  updateSetting,
  saveSettingsSubset,
  busy,
  soulCharacterCount,
  soulLoadedAt,
  soulSavedAt,
  soulText,
  onSoulTextChange,
  onSaveSoul,
  formatLocalTime,
  behaviorPolicy,
  behaviorLoadedAt,
  behaviorPath,
  behaviorRuntimeStates,
  behaviorQuietHoursInput,
  behaviorAllowTonesInput,
  behaviorDenyTonesInput,
  behaviorTriggerKeywordsInput,
  onBehaviorPolicyChange,
  onBehaviorQuietHoursInputChange,
  onBehaviorAllowTonesInputChange,
  onBehaviorDenyTonesInputChange,
  onBehaviorTriggerKeywordsInputChange,
  onSaveBehaviorPolicy,
  telegramConnectionLabel,
  telegramPhoneLabel,
  authAuthorized,
  onLogoutTelegram,
  variableForm,
  onVariableFormChange,
  onSaveVariable,
  variables,
  onDeleteVariable,
  currentAdminPassword,
  newAdminUsername,
  newAdminPassword,
  confirmAdminPassword,
  onCurrentAdminPasswordChange,
  onNewAdminUsernameChange,
  onNewAdminPasswordChange,
  onConfirmAdminPasswordChange,
  onUpdateAdminCredentials,
}: SettingsPanelsProps) {
  if (activePage !== 'settings') {
    return null
  }

  if (activeSettingsPage === 'setting') {
    return (
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
    )
  }

  if (activeSettingsPage === 'soul') {
    return (
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
          <Textarea value={soulText} onChange={(event) => onSoulTextChange(event.target.value)} className="min-h-[235px] font-mono text-xs" />
          <Button size="sm" onClick={() => void onSaveSoul()} disabled={busy}>
            <Save className="size-4" /> Save SOUL
          </Button>
        </CardContent>
      </Card>
    )
  }

  if (activeSettingsPage === 'telegram') {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Telegram</CardTitle>
          <CardDescription>Connection and account actions.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="rounded-lg border border-border/60 bg-background p-3">
              <p className="text-xs uppercase tracking-wide text-muted-foreground">Status</p>
              <p className="mt-1 text-sm font-semibold">{telegramConnectionLabel}</p>
            </div>
            <div className="rounded-lg border border-border/60 bg-background p-3">
              <p className="text-xs uppercase tracking-wide text-muted-foreground">Phone</p>
              <p className="mt-1 text-sm font-semibold">{telegramPhoneLabel}</p>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={() => void onLogoutTelegram()} disabled={busy || !authAuthorized}>
              <LogOut className="size-4" /> Telegram Sign Out
            </Button>
          </div>
        </CardContent>
      </Card>
    )
  }

  if (activeSettingsPage === 'behavior') {
    const escalatedStates = behaviorRuntimeStates.filter((state) => state.escalatedManual)
    return (
      <Card>
        <CardHeader>
          <CardTitle>Behavior Engine</CardTitle>
          <CardDescription>Global reply policy that shapes debounce, cooldown, escalation, quiet hours, reply variability, and final reply constraints.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <div className="rounded-lg border border-border/60 bg-background p-3">
              <p className="text-xs uppercase tracking-wide text-muted-foreground">Policy Loaded</p>
              <p className="mt-1 text-sm font-semibold">{formatLocalTime(behaviorLoadedAt)}</p>
            </div>
            <div className="rounded-lg border border-border/60 bg-background p-3">
              <p className="text-xs uppercase tracking-wide text-muted-foreground">Debounce</p>
              <p className="mt-1 text-sm font-semibold">{behaviorPolicy.debounceSeconds}s</p>
            </div>
            <div className="rounded-lg border border-border/60 bg-background p-3">
              <p className="text-xs uppercase tracking-wide text-muted-foreground">Cooldown</p>
              <p className="mt-1 text-sm font-semibold">{behaviorPolicy.cooldownSeconds}s</p>
            </div>
            <div className="rounded-lg border border-border/60 bg-background p-3">
              <p className="text-xs uppercase tracking-wide text-muted-foreground">Escalated Chats</p>
              <p className="mt-1 text-sm font-semibold">{escalatedStates.length}</p>
            </div>
          </div>
          <div className="rounded-lg border border-border/60 bg-background p-3 text-xs text-muted-foreground">
            Path: {behaviorPath}
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            <div className="space-y-1">
              <Label>Debounce Seconds</Label>
              <Input
                type="number"
                value={behaviorPolicy.debounceSeconds}
                onChange={(event) => onBehaviorPolicyChange({ ...behaviorPolicy, debounceSeconds: Number(event.target.value || 0) })}
                className="h-9"
              />
            </div>
            <div className="space-y-1">
              <Label>Cooldown Seconds</Label>
              <Input
                type="number"
                value={behaviorPolicy.cooldownSeconds}
                onChange={(event) => onBehaviorPolicyChange({ ...behaviorPolicy, cooldownSeconds: Number(event.target.value || 0) })}
                className="h-9"
              />
            </div>
            <div className="space-y-1">
              <Label>Max Reply Words</Label>
              <Input
                type="number"
                value={behaviorPolicy.maxReplyWords}
                onChange={(event) => onBehaviorPolicyChange({ ...behaviorPolicy, maxReplyWords: Number(event.target.value || 0) })}
                className="h-9"
              />
            </div>
            <div className="space-y-1">
              <Label>Failure Threshold</Label>
              <Input
                type="number"
                value={behaviorPolicy.escalation.failureThreshold}
                onChange={(event) =>
                  onBehaviorPolicyChange({
                    ...behaviorPolicy,
                    escalation: { ...behaviorPolicy.escalation, failureThreshold: Number(event.target.value || 0) },
                  })}
                className="h-9"
              />
            </div>
            <div className="space-y-1">
              <Label>Short Reply Probability</Label>
              <Input
                type="number"
                min="0"
                max="1"
                step="0.01"
                value={behaviorPolicy.shortReplyProbability}
                onChange={(event) => onBehaviorPolicyChange({ ...behaviorPolicy, shortReplyProbability: Number(event.target.value || 0) })}
                className="h-9"
              />
            </div>
            <div className="space-y-1">
              <Label>One Word Reply Probability</Label>
              <Input
                type="number"
                min="0"
                max="1"
                step="0.01"
                value={behaviorPolicy.oneWordReplyProbability}
                onChange={(event) => onBehaviorPolicyChange({ ...behaviorPolicy, oneWordReplyProbability: Number(event.target.value || 0) })}
                className="h-9"
              />
            </div>
            <div className="space-y-1">
              <Label>Follow-up Probability</Label>
              <Input
                type="number"
                min="0"
                max="1"
                step="0.01"
                value={behaviorPolicy.followUpProbability}
                onChange={(event) => onBehaviorPolicyChange({ ...behaviorPolicy, followUpProbability: Number(event.target.value || 0) })}
                className="h-9"
              />
            </div>
            <div className="space-y-1">
              <Label>Split Message Probability</Label>
              <Input
                type="number"
                min="0"
                max="1"
                step="0.01"
                value={behaviorPolicy.splitMessageProbability}
                onChange={(event) => onBehaviorPolicyChange({ ...behaviorPolicy, splitMessageProbability: Number(event.target.value || 0) })}
                className="h-9"
              />
            </div>
            <div className="space-y-1 md:col-span-2">
              <Label>Quiet Hours</Label>
              <Textarea
                placeholder="22:00-06:00"
                value={behaviorQuietHoursInput}
                onChange={(event) => onBehaviorQuietHoursInputChange(event.target.value)}
                className="min-h-[82px] font-mono text-xs"
              />
            </div>
            <div className="space-y-1">
              <Label>Allowed Tones</Label>
              <Textarea
                placeholder="natural&#10;brief&#10;human"
                value={behaviorAllowTonesInput}
                onChange={(event) => onBehaviorAllowTonesInputChange(event.target.value)}
                className="min-h-[90px] font-mono text-xs"
              />
            </div>
            <div className="space-y-1">
              <Label>Denied Tones</Label>
              <Textarea
                placeholder="assistant-like&#10;promotional&#10;flirty"
                value={behaviorDenyTonesInput}
                onChange={(event) => onBehaviorDenyTonesInputChange(event.target.value)}
                className="min-h-[90px] font-mono text-xs"
              />
            </div>
            <div className="space-y-1 md:col-span-2">
              <Label>Manual Escalation Trigger Keywords</Label>
              <Textarea
                placeholder="refund&#10;lawyer&#10;urgent issue"
                value={behaviorTriggerKeywordsInput}
                onChange={(event) => onBehaviorTriggerKeywordsInputChange(event.target.value)}
                className="min-h-[90px] font-mono text-xs"
              />
            </div>
          </div>
          <Button size="sm" onClick={() => void onSaveBehaviorPolicy()} disabled={busy}>
            <Save className="size-4" /> Save Behavior Policy
          </Button>
          <div className="space-y-2">
            <p className="text-sm font-semibold">Runtime Escalations</p>
            <div className="max-h-56 overflow-auto rounded-lg border border-border/60 bg-background p-2 text-sm">
              {escalatedStates.map((state) => (
                <div key={state.chatId} className="flex flex-col gap-1 border-b border-border/40 py-2 last:border-b-0">
                  <p className="font-medium">{state.chatId}</p>
                  <p className="text-xs text-muted-foreground">
                    failures: {state.failureCount} • reason: {state.reason || 'manual_escalated'} • updated: {formatLocalTime(state.updatedAt)}
                  </p>
                </div>
              ))}
              {escalatedStates.length === 0 ? <p className="text-xs text-muted-foreground">No escalated chats right now.</p> : null}
            </div>
          </div>
        </CardContent>
      </Card>
    )
  }

  if (activeSettingsPage === 'variables') {
    return (
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
              onChange={(event) => onVariableFormChange({ ...variableForm, key: event.target.value })}
              className="h-9"
            />
            <select
              className="h-9 rounded-md border border-input bg-background px-3 text-sm"
              value={variableForm.type}
              onChange={(event) => onVariableFormChange({ ...variableForm, type: event.target.value as 'text' | 'secret' })}
            >
              <option value="text">text</option>
              <option value="secret">secret</option>
            </select>
            <Input
              type={variableForm.type === 'secret' ? 'password' : 'text'}
              placeholder="value"
              value={variableForm.value}
              onChange={(event) => onVariableFormChange({ ...variableForm, value: event.target.value })}
              className="h-9"
            />
          </div>
          <Button size="sm" onClick={() => void onSaveVariable()} disabled={busy}>
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
                    onClick={() => onVariableFormChange({ key: v.key, type: v.type, value: '' })}
                  >
                    Edit
                  </Button>
                  <Button
                    size="sm"
                    variant="destructive"
                    className="w-full sm:w-auto"
                    onClick={() => void onDeleteVariable(v.key)}
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
    )
  }

  return (
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
            onChange={(event) => onCurrentAdminPasswordChange(event.target.value)}
            className="h-9 text-sm"
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="account-username">New Username</Label>
          <Input
            id="account-username"
            value={newAdminUsername}
            onChange={(event) => onNewAdminUsernameChange(event.target.value)}
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
            onChange={(event) => onNewAdminPasswordChange(event.target.value)}
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
            onChange={(event) => onConfirmAdminPasswordChange(event.target.value)}
            className="h-9 text-sm"
          />
        </div>
        <div className="md:col-span-2 flex flex-col items-start gap-2 sm:flex-row sm:items-center">
          <Button size="sm" className="w-full sm:w-auto" onClick={() => void onUpdateAdminCredentials()} disabled={busy}>
            <ShieldCheck className="size-4" /> Update Dashboard Account
          </Button>
          <span className="text-xs text-muted-foreground">After update, dashboard login is required again.</span>
        </div>
      </CardContent>
    </Card>
  )
}
