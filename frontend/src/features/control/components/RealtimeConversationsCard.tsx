import type { RefObject } from 'react'

import { BellRing } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import type { ConversationMessage, ConversationSummary } from '@/features/control/types'

type RealtimeConversationsCardProps = {
  globalAutoReplyEnabled: boolean
  notificationPermission: NotificationPermission | 'unsupported'
  onEnableNotifications: () => Promise<void>
  conversations: ConversationSummary[]
  selectedChatId: string
  onSelectChatId: (chatId: string) => void
  selectedConversation?: ConversationSummary
  busy: boolean
  onSetConversationMode: (chatId: string, mode: 'auto' | 'manual') => Promise<void>
  activeMessages: ConversationMessage[]
  conversationThreadRef: RefObject<HTMLDivElement | null>
  formatLocalTime: (value?: string) => string
  manualReplyText: string
  onManualReplyTextChange: (value: string) => void
  onSendManualReply: () => Promise<void>
}

export function RealtimeConversationsCard({
  globalAutoReplyEnabled,
  notificationPermission,
  onEnableNotifications,
  conversations,
  selectedChatId,
  onSelectChatId,
  selectedConversation,
  busy,
  onSetConversationMode,
  activeMessages,
  conversationThreadRef,
  formatLocalTime,
  manualReplyText,
  onManualReplyTextChange,
  onSendManualReply,
}: RealtimeConversationsCardProps) {
  return (
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
            <Button size="sm" variant="outline" onClick={() => void onEnableNotifications()}>
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
                onClick={() => onSelectChatId(item.chatId)}
              >
                <p className="truncate text-sm font-medium">{item.chatName || item.chatId}</p>
                <p className="truncate text-xs text-muted-foreground">{item.lastMessage}</p>
                <div className="mt-1 flex items-center gap-2 text-[11px] text-muted-foreground">
                  <span>{formatLocalTime(item.lastMessageAt)}</span>
                  <span>•</span>
                  <span>{item.effectiveMode.toUpperCase()}</span>
                  {item.escalatedManual ? <Badge variant="secondary">Behavior</Badge> : null}
                  {item.unreadIncoming > 0 ? <Badge variant="outline">{item.unreadIncoming}</Badge> : null}
                </div>
                {item.escalatedManual && item.manualReason ? (
                  <p className="mt-1 truncate text-[11px] text-amber-700">{item.manualReason}</p>
                ) : null}
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
                      onClick={() => void onSetConversationMode(selectedConversation.chatId, 'manual')}
                    >
                      Manual
                    </Button>
                    <Button
                      size="sm"
                      variant={selectedConversation.mode !== 'manual' ? 'default' : 'outline'}
                      disabled={busy || !globalAutoReplyEnabled}
                      onClick={() => void onSetConversationMode(selectedConversation.chatId, 'auto')}
                    >
                      Auto
                    </Button>
                    <Badge variant={selectedConversation.effectiveMode === 'manual' ? 'secondary' : 'default'}>
                      Effective: {selectedConversation.effectiveMode}
                    </Badge>
                  </div>
                </div>
                {selectedConversation.escalatedManual ? (
                  <div className="rounded-md border border-amber-300/60 bg-amber-50 px-3 py-2 text-xs text-amber-800">
                    Behavior escalation active: {selectedConversation.manualReason || 'manual_escalated'}
                  </div>
                ) : null}

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
                    onChange={(event) => onManualReplyTextChange(event.target.value)}
                    className="min-h-[90px]"
                  />
                  <div className="flex items-center justify-end">
                    <Button size="sm" onClick={() => void onSendManualReply()} disabled={busy || !manualReplyText.trim()}>
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
  )
}
