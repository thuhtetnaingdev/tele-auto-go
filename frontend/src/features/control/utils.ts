import type { BehaviorPolicy, BehaviorQuietHoursWindow, SettingsResponse } from '@/features/control/types'

export function normalizeSettings(payload: SettingsResponse | undefined): SettingsResponse {
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

export function formatUptime(seconds?: number) {
  if (!seconds || seconds < 1) return '0s'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (h > 0) return `${h}h ${m}m ${s}s`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

export function formatLocalTime(ts?: string) {
  if (!ts) return 'N/A'
  const date = new Date(ts)
  if (Number.isNaN(date.getTime())) return 'N/A'
  return date.toLocaleString()
}

export function normalizePhoneWithPlus(raw: string) {
  const value = raw.trim()
  if (!value) return ''
  if (value.startsWith('+')) return value
  return `+${value}`
}

export function readInitialPhone() {
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

export function normalizeAllowUsersList(values: string[]) {
  const next = values
    .map((value) => normalizeAllowUserToken(value))
    .filter(Boolean)
  return Array.from(new Set(next))
}

export function parseAllowUsersInput(raw: string) {
  return normalizeAllowUsersList(raw.split(/[\n,]/g))
}

export function trimNotificationBody(text?: string) {
  const normalized = (text || '').trim()
  if (normalized.length <= 140) {
    return normalized || 'New message received'
  }
  return `${normalized.slice(0, 137)}...`
}

export function isTrueLike(value: string | undefined) {
  return (value || '').trim().toLowerCase() === 'true'
}

export function logNotificationDebug(stage: string, details?: Record<string, unknown>) {
  const payload = details ? { stage, ...details } : { stage }
  console.info('[tele-auto][notification-sound]', payload)
}

export function defaultBehaviorPolicy(): BehaviorPolicy {
  return {
    debounceSeconds: 10,
    cooldownSeconds: 0,
    quietHours: [],
    maxReplyWords: 60,
    shortReplyProbability: 0,
    oneWordReplyProbability: 0,
    followUpProbability: 0,
    splitMessageProbability: 0,
    toneRules: {
      allow: ['natural', 'brief', 'human'],
      deny: ['assistant-like', 'promotional', 'flirty'],
    },
    escalation: {
      failureThreshold: 3,
      triggerKeywords: [],
    },
  }
}

export function normalizeBehaviorPolicy(policy?: BehaviorPolicy): BehaviorPolicy {
  const fallback = defaultBehaviorPolicy()
  if (!policy) {
    return fallback
  }
  return {
    debounceSeconds: Number.isFinite(policy.debounceSeconds) ? Math.max(0, policy.debounceSeconds) : fallback.debounceSeconds,
    cooldownSeconds: Number.isFinite(policy.cooldownSeconds) ? Math.max(0, policy.cooldownSeconds) : fallback.cooldownSeconds,
    quietHours: Array.isArray(policy.quietHours)
      ? policy.quietHours
        .map((window) => ({ start: (window?.start || '').trim(), end: (window?.end || '').trim() }))
        .filter((window) => window.start && window.end)
      : [],
    maxReplyWords: Number.isFinite(policy.maxReplyWords) && policy.maxReplyWords > 0 ? policy.maxReplyWords : fallback.maxReplyWords,
    shortReplyProbability: normalizeProbability(policy.shortReplyProbability, fallback.shortReplyProbability),
    oneWordReplyProbability: normalizeProbability(policy.oneWordReplyProbability, fallback.oneWordReplyProbability),
    followUpProbability: normalizeProbability(policy.followUpProbability, fallback.followUpProbability),
    splitMessageProbability: normalizeProbability(policy.splitMessageProbability, fallback.splitMessageProbability),
    toneRules: {
      allow: normalizeTextList(policy.toneRules?.allow || fallback.toneRules.allow),
      deny: normalizeTextList(policy.toneRules?.deny || fallback.toneRules.deny),
    },
    escalation: {
      failureThreshold: Number.isFinite(policy.escalation?.failureThreshold) && policy.escalation.failureThreshold > 0
        ? policy.escalation.failureThreshold
        : fallback.escalation.failureThreshold,
      triggerKeywords: normalizeTextList(policy.escalation?.triggerKeywords || []),
    },
  }
}

export function normalizeTextList(values: string[]) {
  return values
    .map((value) => value.trim())
    .filter(Boolean)
}

export function parseTextList(raw: string) {
  return normalizeTextList(raw.split(/[\n,]/g))
}

export function formatTextList(values: string[]) {
  return normalizeTextList(values).join('\n')
}

export function parseQuietHoursInput(raw: string): BehaviorQuietHoursWindow[] {
  return raw
    .split(/\n/g)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const [start, end] = line.split('-').map((part) => part.trim())
      return { start: start || '', end: end || '' }
    })
    .filter((window) => window.start && window.end)
}

export function formatQuietHoursInput(windows: BehaviorQuietHoursWindow[]) {
  return (windows || [])
    .map((window) => `${window.start}-${window.end}`)
    .join('\n')
}

function normalizeProbability(value: number | undefined, fallback: number) {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return fallback
  }
  return Math.min(1, Math.max(0, value))
}
