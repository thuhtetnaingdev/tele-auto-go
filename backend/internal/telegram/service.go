package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"tele-auto-go/internal/agents"
	"tele-auto-go/internal/ai"
	"tele-auto-go/internal/config"
	"tele-auto-go/internal/contextbuilder"
	"tele-auto-go/internal/orchestrator"
	"tele-auto-go/internal/store"
	"tele-auto-go/internal/util"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	tgpeer "github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

type Service struct {
	cfg        config.Config
	logger     *slog.Logger
	db         *store.Store
	ai         *ai.Client
	soulPrompt string
	orch       *orchestrator.Engine

	dispatcher tg.UpdateDispatcher
	client     *telegram.Client
	raw        *tg.Client
	sender     *message.Sender
	peerMgr    *peers.Manager

	inFlight sync.Map
}

type historyEntry struct {
	MessageID  int
	ChatID     string
	SenderID   string
	SenderName string
	Direction  string
	Text       string
	CreatedAt  time.Time
}

func NewService(cfg config.Config, logger *slog.Logger, db *store.Store, aiClient *ai.Client, soulPrompt string) *Service {
	var orch *orchestrator.Engine
	agentMgr, err := agents.NewManager(cfg.AgentsDir, logger)
	if err != nil {
		logger.Warn("failed to initialize agent manager; fallback reply mode enabled", "error", err.Error())
	} else {
		orch = orchestrator.New(aiClient, agentMgr, db, logger)
	}

	return &Service{
		cfg:        cfg,
		logger:     logger,
		db:         db,
		ai:         aiClient,
		soulPrompt: soulPrompt,
		orch:       orch,
	}
}

func (s *Service) Run(ctx context.Context) error {
	s.dispatcher = tg.NewUpdateDispatcher()
	s.dispatcher.OnNewMessage(func(ctx context.Context, entities tg.Entities, upd *tg.UpdateNewMessage) error {
		if err := s.handleIncoming(ctx, entities, upd); err != nil {
			s.logger.Error(
				"Failed to handle incoming update",
				"error", err.Error(),
				"update_type", "UpdateNewMessage",
			)
		}
		return nil
	})
	s.dispatcher.OnFallback(func(ctx context.Context, entities tg.Entities, update tg.UpdateClass) error {
		s.logger.Debug(
			"Unhandled Telegram update",
			"type", fmt.Sprintf("%T", update),
			"short_entities", entities.Short,
		)
		return nil
	})

	var hook telegram.UpdateHandler
	s.client = telegram.NewClient(s.cfg.Telegram.APIID, s.cfg.Telegram.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: s.cfg.Telegram.SessionFile},
		UpdateHandler: telegram.UpdateHandlerFunc(func(ctx context.Context, u tg.UpdatesClass) error {
			return hook.Handle(ctx, u)
		}),
	})
	s.raw = tg.NewClient(s.client)
	s.sender = message.NewSender(s.raw)

	s.peerMgr = peers.Options{Logger: zap.NewNop()}.Build(s.raw)
	gaps := updates.New(updates.Config{
		Handler:      s.dispatcher,
		AccessHasher: s.peerMgr,
		Logger:       zap.NewNop(),
	})
	hook = s.peerMgr.UpdateHook(gaps)

	return s.client.Run(ctx, func(ctx context.Context) error {
		status, err := s.client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("auth status: %w", err)
		}
		if !status.Authorized {
			return fmt.Errorf("telegram session is not authorized, run: go run ./cmd/login")
		}

		if err := s.peerMgr.Init(ctx); err != nil {
			return fmt.Errorf("peer manager init: %w", err)
		}
		me, err := s.peerMgr.Self(ctx)
		if err != nil {
			return fmt.Errorf("get self: %w", err)
		}
		s.logger.Info("Telegram connected", "user_id", me.ID())

		errCh := make(chan error, 1)
		go func() {
			errCh <- gaps.Run(ctx, s.raw, me.ID(), updates.AuthOptions{IsBot: false})
		}()

		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("updates manager: %w", err)
			}
			return nil
		}
	})
}

func (s *Service) handleIncoming(ctx context.Context, entities tg.Entities, upd *tg.UpdateNewMessage) error {
	msg, ok := upd.Message.(*tg.Message)
	if !ok {
		s.logger.Debug("Ignoring non-user message update", "type", fmt.Sprintf("%T", upd.Message))
		return nil // ignore service/system updates
	}
	s.logger.Info(
		"New message update received",
		"message_id", msg.ID,
		"outgoing", msg.Out,
		"peer_type", fmt.Sprintf("%T", msg.GetPeerID()),
		"text_len", len(strings.TrimSpace(msg.Message)),
	)
	if msg.Out {
		s.logger.Debug("Ignoring outgoing message update", "message_id", msg.ID)
		return nil
	}

	peerID := msg.GetPeerID()
	if peerID == nil {
		s.logger.Debug("Ignoring message without peer", "message_id", msg.ID)
		return nil
	}
	if s.cfg.AutoReply.PrivateOnly && !isPrivatePeer(peerID) {
		s.logger.Debug("Ignoring non-private chat message", "peer_type", fmt.Sprintf("%T", peerID), "message_id", msg.ID)
		return nil
	}
	if s.cfg.AutoReply.IgnoreGroups && (isGroupPeer(peerID) || isChannelPeer(peerID)) {
		s.logger.Debug("Ignoring group/channel message", "peer_type", fmt.Sprintf("%T", peerID), "message_id", msg.ID)
		return nil
	}

	if s.cfg.AutoReply.IgnoreBots && s.isPeerBot(ctx, entities, peerID) {
		s.logger.Debug("Ignoring bot message", "message_id", msg.ID)
		return nil
	}

	latestIncomingText := util.NormalizeSpace(msg.Message)
	if latestIncomingText == "" {
		if s.cfg.AutoReply.IgnoreMediaOnly {
			s.logger.Debug("Ignoring empty/media-only incoming message", "message_id", msg.ID)
			return nil
		}
		return nil
	}

	chatID := chatIDFromPeer(peerID)
	triggerID := strconv.Itoa(msg.ID)
	eventKey := chatID + ":" + triggerID

	if _, loaded := s.inFlight.LoadOrStore(eventKey, struct{}{}); loaded {
		s.logger.Info("event already in-flight, skipping", "chat_id", chatID, "message_id", triggerID)
		return nil
	}
	defer s.inFlight.Delete(eventKey)

	reserved, err := s.db.ReserveProcessedEvent(ctx, chatID, triggerID)
	if err != nil {
		return fmt.Errorf("reserve processed event: %w", err)
	}
	if !reserved {
		s.logger.Info("duplicate incoming event, skipping", "chat_id", chatID, "message_id", triggerID)
		return nil
	}

	inputPeer, err := s.resolveInputPeer(ctx, entities, peerID)
	if err != nil {
		return fmt.Errorf("resolve input peer: %w", err)
	}
	s.logger.Info("Incoming message trigger accepted", "chat_id", chatID, "message_id", triggerID)

	historyResult, err := s.raw.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  inputPeer,
		Limit: s.cfg.ContextLimit,
	})
	if err != nil {
		return fmt.Errorf("messages.getHistory: %w", err)
	}
	modifiedHistory, ok := historyResult.AsModified()
	if !ok {
		return fmt.Errorf("messages.getHistory returned unmodified result")
	}

	history := s.normalizeHistory(modifiedHistory.GetMessages())
	if !containsMessageID(history, msg.ID) {
		history = append(history, s.entryFromTrigger(msg))
	}
	sort.Slice(history, func(i, j int) bool { return history[i].MessageID < history[j].MessageID })
	if len(history) > s.cfg.ContextLimit {
		history = history[len(history)-s.cfg.ContextLimit:]
	}

	if err := s.db.SaveMessages(ctx, toStoreRows(history)); err != nil {
		s.logger.Warn("failed to save message history", "error", err.Error())
	}

	if s.cfg.AutoReply.LogContext {
		lines := formatContextLines(history)
		s.logger.Info(
			"Loaded recent conversation context for AI",
			"chat_id", chatID,
			"message_id", triggerID,
			"context_count", len(history),
			"context_limit", s.cfg.ContextLimit,
			"context_messages", lines,
		)
	} else {
		s.logger.Info(
			"Loaded recent conversation context for AI",
			"chat_id", chatID,
			"message_id", triggerID,
			"context_count", len(history),
			"context_limit", s.cfg.ContextLimit,
		)
	}

	autoReplyID, err := s.db.CreateAutoReply(ctx, chatID, triggerID, len(history), s.cfg.OpenAI.Model)
	if err != nil {
		return fmt.Errorf("create auto reply row: %w", err)
	}

	if !s.cfg.AutoReply.Enabled {
		_ = s.db.MarkAutoReplyFailed(ctx, autoReplyID, "skipped_disabled", "AUTO_REPLY_ENABLED=false")
		return nil
	}

	reply := ""
	if s.orch != nil {
		reply, err = s.orch.Handle(ctx, orchestrator.MessageContext{
			ChatID:         chatID,
			ChatName:       s.chatNameFromPeer(ctx, entities, peerID),
			LatestIncoming: latestIncomingText,
			RecentMessages: toContextLines(history),
			TriggerMessage: triggerID,
		}, s.soulPrompt)
		if err != nil {
			s.logger.Warn("orchestrator failed; fallback to legacy reply", "error", err.Error())
		}
	}
	if strings.TrimSpace(reply) == "" {
		systemPrompt, userPrompt := contextbuilder.Build(
			s.chatNameFromPeer(ctx, entities, peerID),
			toContextLines(history),
			latestIncomingText,
			s.soulPrompt,
		)
		reply, err = s.ai.GenerateReply(ctx, systemPrompt, userPrompt)
		if err != nil {
			_ = s.db.MarkAutoReplyFailed(ctx, autoReplyID, "ai_error", err.Error())
			return fmt.Errorf("ai generate: %w", err)
		}
	}
	if strings.TrimSpace(reply) == "" {
		_ = s.db.MarkAutoReplyFailed(ctx, autoReplyID, "skipped_empty", "AI returned blank text")
		s.logger.Info("No reply sent", "chat_id", chatID, "message_id", triggerID, "reason", "blank_ai_output")
		return nil
	}

	d, err := util.WaitRandomDelayContext(ctx, s.cfg.AutoReply.DelayMinMS, s.cfg.AutoReply.DelayMaxMS)
	if err != nil {
		_ = s.db.MarkAutoReplyFailed(ctx, autoReplyID, "canceled_before_send", err.Error())
		return nil
	}
	s.logger.Debug("Human-like delay completed", "delay_ms", d.Milliseconds(), "chat_id", chatID, "message_id", triggerID)

	if _, err := s.sender.To(inputPeer).Text(ctx, reply); err != nil {
		_ = s.db.MarkAutoReplyFailed(ctx, autoReplyID, "send_failed", err.Error())
		return fmt.Errorf("send reply: %w", err)
	}
	if err := s.db.MarkAutoReplySent(ctx, autoReplyID, reply); err != nil {
		s.logger.Warn("failed to mark auto reply sent", "error", err.Error())
	}

	s.logger.Info("Auto-reply sent", "chat_id", chatID, "message_id", triggerID, "context_count", len(history))
	return nil
}

func (s *Service) normalizeHistory(messages []tg.MessageClass) []historyEntry {
	out := make([]historyEntry, 0, len(messages))
	for _, m := range messages {
		switch v := m.(type) {
		case *tg.Message:
			peerID := v.GetPeerID()
			chatID := chatIDFromPeer(peerID)
			text := util.NormalizeSpace(v.Message)
			if text == "" {
				text = "(non-text message)"
			}
			out = append(out, historyEntry{
				MessageID:  v.ID,
				ChatID:     chatID,
				SenderID:   senderIDFromMessagePeer(v.GetFromID),
				SenderName: directionName(v.Out),
				Direction:  directionName(v.Out),
				Text:       text,
				CreatedAt:  time.Unix(int64(v.GetDate()), 0).UTC(),
			})
		case *tg.MessageService:
			peerID := v.GetPeerID()
			chatID := chatIDFromPeer(peerID)
			out = append(out, historyEntry{
				MessageID:  v.ID,
				ChatID:     chatID,
				SenderID:   senderIDFromMessagePeer(v.GetFromID),
				SenderName: directionName(v.Out),
				Direction:  directionName(v.Out),
				Text:       "(service message)",
				CreatedAt:  time.Unix(int64(v.GetDate()), 0).UTC(),
			})
		}
	}
	return out
}

func (s *Service) entryFromTrigger(msg *tg.Message) historyEntry {
	return historyEntry{
		MessageID:  msg.ID,
		ChatID:     chatIDFromPeer(msg.GetPeerID()),
		SenderID:   senderIDFromMessagePeer(msg.GetFromID),
		SenderName: directionName(msg.Out),
		Direction:  directionName(msg.Out),
		Text:       util.NormalizeSpace(msg.Message),
		CreatedAt:  time.Unix(int64(msg.GetDate()), 0).UTC(),
	}
}

func toStoreRows(in []historyEntry) []store.MessageRecord {
	out := make([]store.MessageRecord, 0, len(in))
	for _, e := range in {
		out = append(out, store.MessageRecord{
			ChatID:            e.ChatID,
			TelegramMessageID: strconv.Itoa(e.MessageID),
			SenderID:          e.SenderID,
			SenderName:        e.SenderName,
			Direction:         e.Direction,
			Text:              e.Text,
			CreatedAt:         e.CreatedAt,
		})
	}
	return out
}

func toContextLines(in []historyEntry) []contextbuilder.MessageLine {
	out := make([]contextbuilder.MessageLine, 0, len(in))
	for _, e := range in {
		out = append(out, contextbuilder.MessageLine{
			Direction: e.Direction,
			Text:      e.Text,
		})
	}
	return out
}

func formatContextLines(in []historyEntry) []string {
	out := make([]string, 0, len(in))
	for i, e := range in {
		role := "other_person"
		if e.Direction == "me" {
			role = "me"
		}
		out = append(out, fmt.Sprintf("%d. [%s] %s", i+1, role, util.NormalizeSpace(e.Text)))
	}
	return out
}

func containsMessageID(in []historyEntry, id int) bool {
	for _, e := range in {
		if e.MessageID == id {
			return true
		}
	}
	return false
}

func directionName(out bool) string {
	if out {
		return "me"
	}
	return "other_person"
}

func senderIDFromMessagePeer(fromFunc func() (tg.PeerClass, bool)) string {
	from, ok := fromFunc()
	if !ok || from == nil {
		return ""
	}
	switch v := from.(type) {
	case *tg.PeerUser:
		return strconv.FormatInt(v.UserID, 10)
	case *tg.PeerChat:
		return strconv.FormatInt(v.ChatID, 10)
	case *tg.PeerChannel:
		return strconv.FormatInt(v.ChannelID, 10)
	default:
		return ""
	}
}

func chatIDFromPeer(peer tg.PeerClass) string {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return "user:" + strconv.FormatInt(p.UserID, 10)
	case *tg.PeerChat:
		return "chat:" + strconv.FormatInt(p.ChatID, 10)
	case *tg.PeerChannel:
		return "channel:" + strconv.FormatInt(p.ChannelID, 10)
	default:
		return "unknown:0"
	}
}

func isPrivatePeer(peer tg.PeerClass) bool {
	_, ok := peer.(*tg.PeerUser)
	return ok
}

func isGroupPeer(peer tg.PeerClass) bool {
	_, ok := peer.(*tg.PeerChat)
	return ok
}

func isChannelPeer(peer tg.PeerClass) bool {
	_, ok := peer.(*tg.PeerChannel)
	return ok
}

func (s *Service) resolveInputPeer(ctx context.Context, entities tg.Entities, peerID tg.PeerClass) (tg.InputPeerClass, error) {
	if p, err := tgpeer.EntitiesFromUpdate(entities).ExtractPeer(peerID); err == nil {
		return p, nil
	}
	resolved, err := s.peerMgr.ResolvePeer(ctx, peerID)
	if err == nil {
		return resolved.InputPeer(), nil
	}

	// Fallback: short updates may not include user access hash.
	// Pulling recent dialogs usually contains the target user with access hash.
	switch p := peerID.(type) {
	case *tg.PeerUser:
		dialogPeer, derr := s.resolveInputPeerFromDialogs(ctx, p.UserID)
		if derr == nil {
			return dialogPeer, nil
		}
		return nil, fmt.Errorf("%w; dialogs fallback: %v", err, derr)
	default:
		return nil, err
	}
}

func (s *Service) resolveInputPeerFromDialogs(ctx context.Context, userID int64) (tg.InputPeerClass, error) {
	resp, err := s.raw.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetDate: 0,
		OffsetID:   0,
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      100,
		Hash:       0,
	})
	if err != nil {
		return nil, fmt.Errorf("messages.getDialogs: %w", err)
	}
	modified, ok := resp.AsModified()
	if !ok {
		return nil, fmt.Errorf("messages.getDialogs returned unmodified result")
	}

	for _, u := range modified.GetUsers() {
		usr, ok := u.AsNotEmpty()
		if !ok {
			continue
		}
		if usr.ID != userID {
			continue
		}
		if err := s.peerMgr.Apply(ctx, modified.GetUsers(), modified.GetChats()); err != nil {
			s.logger.Debug("Failed to apply dialog peers cache", "error", err.Error())
		}
		return &tg.InputPeerUser{
			UserID:     usr.ID,
			AccessHash: usr.AccessHash,
		}, nil
	}
	return nil, fmt.Errorf("user_id=%d not found in recent dialogs", userID)
}

func (s *Service) isPeerBot(ctx context.Context, entities tg.Entities, peer tg.PeerClass) bool {
	p, ok := peer.(*tg.PeerUser)
	if !ok {
		return false
	}
	u, ok := entities.Users[p.UserID]
	if ok && u != nil {
		return u.Bot
	}

	resolved, err := s.peerMgr.ResolveUserID(ctx, p.UserID)
	if err != nil {
		return false
	}
	raw := resolved.Raw()
	if raw == nil {
		return false
	}
	return raw.Bot
}

func (s *Service) chatNameFromPeer(ctx context.Context, entities tg.Entities, peer tg.PeerClass) string {
	p, ok := peer.(*tg.PeerUser)
	if !ok {
		return "unknown"
	}
	u, ok := entities.Users[p.UserID]
	if ok && u != nil {
		first, _ := u.GetFirstName()
		last, _ := u.GetLastName()
		name := strings.TrimSpace(strings.TrimSpace(first + " " + last))
		if name != "" {
			return name
		}
		if u.Username != "" {
			return u.Username
		}
	}

	resolved, err := s.peerMgr.ResolveUserID(ctx, p.UserID)
	if err != nil {
		return "unknown"
	}
	if name := strings.TrimSpace(resolved.VisibleName()); name != "" {
		return name
	}
	if username, ok := resolved.Username(); ok && strings.TrimSpace(username) != "" {
		return username
	}
	return "unknown"
}
