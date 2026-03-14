import { useRef, useState } from 'react'

import { logNotificationDebug, trimNotificationBody } from '@/features/control/utils'
import type { ConversationStreamEvent, ConversationSummary } from '@/features/control/types'

type UseManualConversationNotificationsOptions = {
  getConversations: () => ConversationSummary[]
  getSelectedChatId: () => string
  getGlobalAutoReplyEnabled: () => boolean
  navigateToDashboard: () => void
  onSelectChat: (chatId: string) => void
  onSetMessage: (message: string) => void
}

export function useManualConversationNotifications({
  getConversations,
  getSelectedChatId,
  getGlobalAutoReplyEnabled,
  navigateToDashboard,
  onSelectChat,
  onSetMessage,
}: UseManualConversationNotificationsOptions) {
  const notifiedEventKeysRef = useRef<Set<string>>(new Set())
  const notificationAudioContextRef = useRef<AudioContext | null>(null)
  const [notificationPermission, setNotificationPermission] = useState<NotificationPermission | 'unsupported'>(() => {
    if (typeof window === 'undefined' || !('Notification' in window)) {
      return 'unsupported'
    }
    return window.Notification.permission
  })

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
      onSetMessage('This browser does not support desktop notifications.')
      return
    }
    try {
      await ensureNotificationAudioContext()
      const permission = await window.Notification.requestPermission()
      logNotificationDebug('permission-request-result', { permission })
      setNotificationPermission(permission)
      if (permission === 'granted') {
        onSetMessage('Browser notifications enabled for manual chats.')
        return
      }
      if (permission === 'denied') {
        onSetMessage('Browser notifications are blocked. Allow them in browser site settings.')
        return
      }
      onSetMessage('Browser notification permission was dismissed.')
    } catch {
      logNotificationDebug('permission-request-failed')
      onSetMessage('Unable to request browser notification permission.')
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
    const conversation = getConversations().find((item) => item.chatId === chatID)
    const isManual = !getGlobalAutoReplyEnabled() || conversation?.effectiveMode === 'manual'
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
      getSelectedChatId() === chatID
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
      navigateToDashboard()
      onSelectChat(chatID)
      notification.close()
    }
  }

  return {
    notificationPermission,
    syncNotificationPermission,
    ensureNotificationAudioContext,
    requestBrowserNotifications,
    notifyForManualConversation,
  }
}
