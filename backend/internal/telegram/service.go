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
	"tele-auto-go/internal/behavior"
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
	behavior   behavior.LoadedPolicy
	orch       *orchestrator.Engine
	onEvent    func(Event)

	dispatcher tg.UpdateDispatcher
	client     *telegram.Client
	raw        *tg.Client
	sender     *message.Sender
	peerMgr    *peers.Manager

	inFlight sync.Map

	debounceMu     sync.Mutex
	debounceTimers map[string]*debounceState
	runCtx         context.Context
}

type Event struct {
	Type              string
	ChatID            string
	TelegramMessageID string
	Direction         string
	Text              string
	Mode              string
	Reason            string
	CreatedAt         string
}

type autoReplyRequest struct {
	ChatID           string
	ChatName         string
	LatestIncoming   string
	TriggerMessageID string
	TriggerUserID    string
	TriggerUsername  string
}

type debounceState struct {
	seq     uint64
	timer   *time.Timer
	pending autoReplyRequest
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

func NewService(cfg config.Config, logger *slog.Logger, db *store.Store, aiClient *ai.Client, soulPrompt string, onEvent func(Event)) *Service {
	var orch *orchestrator.Engine
	agentMgr, err := agents.NewManager(cfg.AgentsDir, logger)
	if err != nil {
		logger.Warn("failed to initialize agent manager; fallback reply mode enabled", "error", err.Error())
	} else {
		orch = orchestrator.New(aiClient, agentMgr, db, logger)
	}

	return &Service{
		cfg:            cfg,
		logger:         logger,
		db:             db,
		ai:             aiClient,
		soulPrompt:     soulPrompt,
		behavior:       behavior.Load(cfg.BehaviorPath, logger),
		orch:           orch,
		onEvent:        onEvent,
		debounceTimers: make(map[string]*debounceState),
	}
}

func (s *Service) Run(ctx context.Context) error {
	s.runCtx = ctx
	defer s.cancelDebouncedReplies()

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
	if msg.Out {
		return s.persistOutgoing(ctx, msg, peerID)
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
	chatName := s.chatNameFromPeer(ctx, entities, peerID)
	for i := range history {
		if history[i].Direction == "other_person" && strings.TrimSpace(history[i].SenderName) == "other_person" {
			history[i].SenderName = chatName
		}
		if history[i].Direction == "me" {
			history[i].SenderName = "me"
		}
	}
	sort.Slice(history, func(i, j int) bool { return history[i].MessageID < history[j].MessageID })
	if len(history) > s.cfg.ContextLimit {
		history = history[len(history)-s.cfg.ContextLimit:]
	}

	if err := s.db.SaveMessages(ctx, toStoreRows(history)); err != nil {
		s.logger.Warn("failed to save message history", "error", err.Error())
	}
	s.publishEvent(Event{
		Type:              "message_created",
		ChatID:            chatID,
		TelegramMessageID: triggerID,
		Direction:         "other_person",
		Text:              latestIncomingText,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	})

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
	req := autoReplyRequest{
		ChatID:           chatID,
		ChatName:         chatName,
		LatestIncoming:   latestIncomingText,
		TriggerMessageID: triggerID,
		TriggerUserID:    senderIDFromMessagePeer(msg.GetFromID),
		TriggerUsername:  s.usernameFromMessage(ctx, entities, msg),
	}
	if req.TriggerUserID == "" {
		if peerUser, ok := msg.GetPeerID().(*tg.PeerUser); ok {
			req.TriggerUserID = strconv.FormatInt(peerUser.UserID, 10)
		}
	}
	decision := s.evaluateBehavior(ctx, chatID, latestIncomingText)
	if !s.shouldAutoReply(ctx, chatID) {
		if decision.ForceManual {
			s.publishEvent(Event{
				Type:      "behavior_escalated",
				ChatID:    chatID,
				Mode:      "manual",
				Reason:    decision.SkipReason,
				CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			})
		}
		return nil
	}
	if !decision.AllowReply {
		if decision.ForceManual {
			if err := s.db.SetBehaviorEscalation(ctx, chatID, decision.SkipReason); err != nil {
				s.logger.Warn("failed to persist behavior escalation", "chat_id", chatID, "error", err.Error())
			}
			s.publishEvent(Event{
				Type:      "behavior_escalated",
				ChatID:    chatID,
				Mode:      "manual",
				Reason:    decision.SkipReason,
				CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			})
		}
		if err := s.db.ClearBehaviorPending(ctx, chatID, time.Now().UTC()); err != nil {
			s.logger.Warn("failed to clear behavior pending state", "chat_id", chatID, "error", err.Error())
		}
		return nil
	}
	if decision.DebounceSeconds > 0 {
		s.scheduleDebouncedAutoReply(ctx, req, decision)
		return nil
	}
	return s.executeAutoReply(ctx, req)
}

func truncatePreview(text string, max int) string {
	v := util.NormalizeSpace(text)
	if max <= 0 || len(v) <= max {
		return v
	}
	return v[:max] + "...(truncated)"
}

func (s *Service) scheduleDebouncedAutoReply(ctx context.Context, req autoReplyRequest, decision behavior.Decision) {
	window := time.Duration(decision.DebounceSeconds) * time.Second
	if window <= 0 {
		return
	}

	if err := s.db.UpdateBehaviorPending(ctx, req.ChatID, time.Now().UTC(), time.Now().UTC().Add(window), req.TriggerMessageID, truncatePreview(req.LatestIncoming, 120)); err != nil {
		s.logger.Warn("failed to persist behavior debounce state", "chat_id", req.ChatID, "error", err.Error())
	}

	s.debounceMu.Lock()
	state, ok := s.debounceTimers[req.ChatID]
	if !ok {
		state = &debounceState{}
		s.debounceTimers[req.ChatID] = state
	}
	state.seq++
	seq := state.seq
	state.pending = req
	if state.timer != nil {
		state.timer.Stop()
	}
	state.timer = time.AfterFunc(window, func() {
		s.flushDebouncedAutoReply(req.ChatID, seq)
	})
	s.debounceMu.Unlock()

	s.logger.Info(
		"Auto-reply debounced",
		"chat_id", req.ChatID,
		"message_id", req.TriggerMessageID,
		"debounce_seconds", decision.DebounceSeconds,
	)
}

func (s *Service) flushDebouncedAutoReply(chatID string, seq uint64) {
	s.debounceMu.Lock()
	state, ok := s.debounceTimers[chatID]
	if !ok || state.seq != seq {
		s.debounceMu.Unlock()
		return
	}
	req := state.pending
	delete(s.debounceTimers, chatID)
	s.debounceMu.Unlock()

	if s.runCtx == nil || s.runCtx.Err() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.runCtx, 2*time.Minute)
	defer cancel()
	if err := s.executeAutoReply(ctx, req); err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Error(
			"Debounced auto-reply failed",
			"chat_id", req.ChatID,
			"message_id", req.TriggerMessageID,
			"error", err.Error(),
		)
	}
}

func (s *Service) cancelDebouncedReplies() {
	s.debounceMu.Lock()
	defer s.debounceMu.Unlock()
	for chatID, state := range s.debounceTimers {
		if state.timer != nil {
			state.timer.Stop()
		}
		delete(s.debounceTimers, chatID)
	}
}

func (s *Service) executeAutoReply(ctx context.Context, req autoReplyRequest) error {
	inputPeer, err := s.inputPeerFromChatID(ctx, req.ChatID)
	if err != nil {
		return fmt.Errorf("resolve input peer: %w", err)
	}

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
	for i := range history {
		if history[i].Direction == "other_person" && strings.TrimSpace(history[i].SenderName) == "other_person" {
			history[i].SenderName = req.ChatName
		}
		if history[i].Direction == "me" {
			history[i].SenderName = "me"
		}
	}
	sort.Slice(history, func(i, j int) bool { return history[i].MessageID < history[j].MessageID })
	if len(history) > s.cfg.ContextLimit {
		history = history[len(history)-s.cfg.ContextLimit:]
	}
	if err := s.db.SaveMessages(ctx, toStoreRows(history)); err != nil {
		s.logger.Warn("failed to save message history before auto-reply", "error", err.Error())
	}

	autoReplyID, err := s.db.CreateAutoReply(ctx, req.ChatID, req.TriggerMessageID, len(history), s.cfg.OpenAI.Model)
	if err != nil {
		return fmt.Errorf("create auto reply row: %w", err)
	}

	decision := s.evaluateBehavior(ctx, req.ChatID, req.LatestIncoming)
	if !s.shouldAutoReply(ctx, req.ChatID) {
		_ = s.db.ClearBehaviorPending(ctx, req.ChatID, time.Now().UTC())
		mode, ok, _ := s.db.GetChatMode(ctx, req.ChatID)
		status := "skipped_disabled"
		errMsg := "AUTO_REPLY_ENABLED=false"
		if s.cfg.AutoReply.Enabled && decision.ForceManual {
			status = "skipped_behavior_manual"
			errMsg = decision.SkipReason
		} else if s.cfg.AutoReply.Enabled && ok && mode == "manual" {
			status = "skipped_manual_mode"
			errMsg = "chat mode is manual"
		}
		_ = s.db.MarkAutoReplyFailed(ctx, autoReplyID, status, errMsg)
		return nil
	}
	if !decision.AllowReply {
		_ = s.db.ClearBehaviorPending(ctx, req.ChatID, time.Now().UTC())
		if decision.ForceManual {
			if err := s.db.SetBehaviorEscalation(ctx, req.ChatID, decision.SkipReason); err != nil {
				s.logger.Warn("failed to persist behavior escalation", "chat_id", req.ChatID, "error", err.Error())
			}
			s.publishEvent(Event{
				Type:      "behavior_escalated",
				ChatID:    req.ChatID,
				Mode:      "manual",
				Reason:    decision.SkipReason,
				CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			})
		}
		_ = s.db.MarkAutoReplyFailed(ctx, autoReplyID, "skipped_behavior_policy", decision.SkipReason)
		return nil
	}

	constraints := behavior.SampleConstraints(s.currentBehaviorPolicy())

	reply := ""
	replySource := "none"
	if s.orch != nil {
		reply, err = s.orch.Handle(ctx, orchestrator.MessageContext{
			ChatID:          req.ChatID,
			ChatName:        req.ChatName,
			LatestIncoming:  req.LatestIncoming,
			RecentMessages:  toContextLines(history),
			TriggerMessage:  req.TriggerMessageID,
			TriggerUserID:   req.TriggerUserID,
			TriggerUsername: req.TriggerUsername,
		}, s.soulPrompt, constraints)
		if err != nil {
			s.logger.Warn("orchestrator failed; fallback to legacy reply", "error", err.Error())
		} else if strings.TrimSpace(reply) != "" {
			replySource = "orchestrator"
		}
	} else {
		s.logger.Warn("orchestrator unavailable; fallback to legacy reply", "chat_id", req.ChatID, "message_id", req.TriggerMessageID)
	}
	if strings.TrimSpace(reply) == "" {
		s.logger.Info(
			"using legacy ai fallback",
			"chat_id", req.ChatID,
			"message_id", req.TriggerMessageID,
			"reason", "orchestrator_empty_or_unavailable",
		)
		systemPrompt, userPrompt := contextbuilder.Build(
			req.ChatName,
			toContextLines(history),
			req.LatestIncoming,
			s.soulPrompt,
			constraints,
		)
		reply, err = s.ai.GenerateReply(ctx, systemPrompt, userPrompt)
		if err != nil {
			_ = s.db.MarkAutoReplyFailed(ctx, autoReplyID, "ai_error", err.Error())
			s.markBehaviorFailure(ctx, req.ChatID, "ai_error")
			return fmt.Errorf("ai generate: %w", err)
		}
		replySource = "legacy_ai"
	}
	reply = behavior.ApplyOutputConstraints(reply, constraints, 40)
	if strings.TrimSpace(reply) == "" {
		_ = s.db.MarkAutoReplyFailed(ctx, autoReplyID, "skipped_empty", "AI returned blank text")
		s.markBehaviorFailure(ctx, req.ChatID, "skipped_empty")
		s.logger.Info("No reply sent", "chat_id", req.ChatID, "message_id", req.TriggerMessageID, "reason", "blank_ai_output")
		return nil
	}

	d, err := util.WaitRandomDelayContext(ctx, s.cfg.AutoReply.DelayMinMS, s.cfg.AutoReply.DelayMaxMS)
	if err != nil {
		_ = s.db.MarkAutoReplyFailed(ctx, autoReplyID, "canceled_before_send", err.Error())
		s.markBehaviorFailure(ctx, req.ChatID, "canceled_before_send")
		return nil
	}
	s.logger.Debug("Human-like delay completed", "delay_ms", d.Milliseconds(), "chat_id", req.ChatID, "message_id", req.TriggerMessageID)

	sentRecords, err := s.sendReplyParts(ctx, inputPeer, req.ChatID, "me", reply, constraints)
	if err != nil {
		_ = s.db.MarkAutoReplyFailed(ctx, autoReplyID, "send_failed", err.Error())
		s.markBehaviorFailure(ctx, req.ChatID, "send_failed")
		return fmt.Errorf("send reply: %w", err)
	}
	if err := s.db.MarkAutoReplySent(ctx, autoReplyID, reply); err != nil {
		s.logger.Warn("failed to mark auto reply sent", "error", err.Error())
	}
	lastSent := sentRecords[len(sentRecords)-1]
	if err := s.db.MarkBehaviorReplySent(ctx, req.ChatID, lastSent.CreatedAt); err != nil {
		s.logger.Warn("failed to persist behavior success state", "chat_id", req.ChatID, "error", err.Error())
	}
	for _, sentRecord := range sentRecords {
		s.publishEvent(Event{
			Type:              "message_created",
			ChatID:            sentRecord.ChatID,
			TelegramMessageID: sentRecord.TelegramMessageID,
			Direction:         sentRecord.Direction,
			Text:              sentRecord.Text,
			CreatedAt:         sentRecord.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}

	s.logger.Info(
		"Auto-reply sent",
		"chat_id", req.ChatID,
		"message_id", req.TriggerMessageID,
		"context_count", len(history),
		"source", replySource,
		"reply_preview", truncatePreview(reply, 220),
	)
	return nil
}

func (s *Service) publishEvent(ev Event) {
	if s.onEvent == nil {
		return
	}
	s.onEvent(ev)
}

func (s *Service) currentBehaviorPolicy() behavior.Policy {
	return s.behavior.Policy
}

func (s *Service) currentBehaviorMeta() behavior.LoadedPolicy {
	return s.behavior
}

func (s *Service) behaviorState(ctx context.Context, chatID string) behavior.RuntimeState {
	state, ok, err := s.db.GetBehaviorRuntimeState(ctx, chatID)
	if err != nil {
		s.logger.Warn("failed to load behavior runtime state", "chat_id", chatID, "error", err.Error())
		return behavior.RuntimeState{ChatID: chatID}
	}
	if !ok {
		return behavior.RuntimeState{ChatID: chatID}
	}
	return behavior.RuntimeState{
		ChatID:                  state.ChatID,
		LastIncomingAt:          state.LastIncomingAt,
		LastAutoReplyAt:         state.LastAutoReplyAt,
		DebounceUntil:           state.DebounceUntil,
		PendingTriggerMessageID: state.PendingTriggerMessageID,
		PendingPreview:          state.PendingPreview,
		ConsecutiveFailures:     state.ConsecutiveFailures,
		EscalatedManual:         state.EscalatedManual,
		EscalationReason:        state.EscalationReason,
		UpdatedAt:               state.UpdatedAt,
	}
}

func (s *Service) evaluateBehavior(ctx context.Context, chatID, latestIncoming string) behavior.Decision {
	return behavior.Evaluate(time.Now(), s.currentBehaviorPolicy(), s.behaviorState(ctx, chatID), latestIncoming)
}

func (s *Service) markBehaviorFailure(ctx context.Context, chatID, reason string) {
	policy := s.currentBehaviorPolicy()
	current := s.behaviorState(ctx, chatID)
	nextFailures := current.ConsecutiveFailures + 1
	escalate := behavior.ShouldEscalateAfterFailure(policy, nextFailures)
	state, err := s.db.MarkBehaviorFailure(ctx, chatID, reason, escalate)
	if err != nil {
		s.logger.Warn("failed to persist behavior failure", "chat_id", chatID, "error", err.Error())
		return
	}
	if state.EscalatedManual {
		s.publishEvent(Event{
			Type:      "behavior_escalated",
			ChatID:    chatID,
			Mode:      "manual",
			Reason:    state.EscalationReason,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
}

func (s *Service) shouldAutoReply(ctx context.Context, chatID string) bool {
	if !s.cfg.AutoReply.Enabled {
		return false
	}
	state := s.behaviorState(ctx, chatID)
	if state.EscalatedManual {
		return false
	}
	mode, ok, err := s.db.GetChatMode(ctx, chatID)
	if err != nil {
		s.logger.Warn("failed to load chat mode; using default auto mode", "chat_id", chatID, "error", err.Error())
		return true
	}
	if ok && mode == "manual" {
		return false
	}
	return true
}

func (s *Service) persistOutgoing(ctx context.Context, msg *tg.Message, peerID tg.PeerClass) error {
	text := util.NormalizeSpace(msg.Message)
	if text == "" {
		text = "(non-text message)"
	}
	row := store.MessageRecord{
		ChatID:            chatIDFromPeer(peerID),
		TelegramMessageID: strconv.Itoa(msg.ID),
		SenderID:          senderIDFromMessagePeer(msg.GetFromID),
		SenderName:        "me",
		Direction:         "me",
		Text:              text,
		CreatedAt:         time.Unix(int64(msg.GetDate()), 0).UTC(),
	}
	if err := s.db.SaveMessage(ctx, row); err != nil {
		return err
	}
	s.publishEvent(Event{
		Type:              "message_created",
		ChatID:            row.ChatID,
		TelegramMessageID: row.TelegramMessageID,
		Direction:         row.Direction,
		Text:              row.Text,
		CreatedAt:         row.CreatedAt.UTC().Format(time.RFC3339Nano),
	})
	return nil
}

func (s *Service) SendText(ctx context.Context, chatID, text string) (store.MessageRecord, error) {
	chatID = strings.TrimSpace(chatID)
	text = util.NormalizeSpace(text)
	if chatID == "" {
		return store.MessageRecord{}, fmt.Errorf("chat_id is required")
	}
	if text == "" {
		return store.MessageRecord{}, fmt.Errorf("text is required")
	}
	inputPeer, err := s.inputPeerFromChatID(ctx, chatID)
	if err != nil {
		return store.MessageRecord{}, err
	}
	rec, err := s.sendAndPersist(ctx, inputPeer, chatID, "me", text)
	if err != nil {
		return store.MessageRecord{}, err
	}
	s.publishEvent(Event{
		Type:              "message_created",
		ChatID:            rec.ChatID,
		TelegramMessageID: rec.TelegramMessageID,
		Direction:         rec.Direction,
		Text:              rec.Text,
		CreatedAt:         rec.CreatedAt.UTC().Format(time.RFC3339Nano),
	})
	return rec, nil
}

func (s *Service) ResolveChatDisplayName(ctx context.Context, chatID string) (string, error) {
	chatID = strings.TrimSpace(chatID)
	if !strings.HasPrefix(chatID, "user:") {
		return chatID, nil
	}
	rawUserID := strings.TrimSpace(strings.TrimPrefix(chatID, "user:"))
	userID, err := strconv.ParseInt(rawUserID, 10, 64)
	if err != nil || userID <= 0 {
		return chatID, nil
	}
	if name := s.displayNameFromPeerCache(ctx, userID); name != "" {
		return name, nil
	}
	if name := s.displayNameFromDialogs(ctx, userID); name != "" {
		return name, nil
	}
	return chatID, nil
}

func (s *Service) displayNameFromPeerCache(ctx context.Context, userID int64) string {
	resolved, err := s.peerMgr.ResolveUserID(ctx, userID)
	if err != nil {
		return ""
	}
	if name := strings.TrimSpace(resolved.VisibleName()); name != "" {
		return name
	}
	if username, ok := resolved.Username(); ok {
		if username = strings.TrimSpace(username); username != "" {
			return "@" + username
		}
	}
	return ""
}

func (s *Service) displayNameFromDialogs(ctx context.Context, userID int64) string {
	resp, err := s.raw.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetDate: 0,
		OffsetID:   0,
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      100,
		Hash:       0,
	})
	if err != nil {
		return ""
	}
	modified, ok := resp.AsModified()
	if !ok {
		return ""
	}
	for _, u := range modified.GetUsers() {
		usr, ok := u.AsNotEmpty()
		if !ok || usr.ID != userID {
			continue
		}
		first, _ := usr.GetFirstName()
		last, _ := usr.GetLastName()
		if full := strings.TrimSpace(strings.TrimSpace(first + " " + last)); full != "" {
			return full
		}
		if username := strings.TrimSpace(usr.Username); username != "" {
			return "@" + username
		}
	}
	return ""
}

func (s *Service) sendAndPersist(ctx context.Context, inputPeer tg.InputPeerClass, chatID, senderName, text string) (store.MessageRecord, error) {
	if _, err := s.sender.To(inputPeer).Text(ctx, text); err != nil {
		return store.MessageRecord{}, err
	}
	msgID := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	createdAt := time.Now().UTC()

	historyResp, err := s.raw.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  inputPeer,
		Limit: 1,
	})
	if err == nil {
		if mod, ok := historyResp.AsModified(); ok {
			for _, cls := range mod.GetMessages() {
				if m, ok := cls.(*tg.Message); ok && m.Out {
					msgID = strconv.Itoa(m.ID)
					createdAt = time.Unix(int64(m.GetDate()), 0).UTC()
					break
				}
			}
		}
	}

	row := store.MessageRecord{
		ChatID:            chatID,
		TelegramMessageID: msgID,
		SenderName:        senderName,
		Direction:         "me",
		Text:              text,
		CreatedAt:         createdAt,
	}
	if err := s.db.SaveMessage(ctx, row); err != nil {
		return store.MessageRecord{}, err
	}
	return row, nil
}

func (s *Service) sendReplyParts(ctx context.Context, inputPeer tg.InputPeerClass, chatID, senderName, text string, constraints behavior.Constraints) ([]store.MessageRecord, error) {
	parts := []string{text}
	if constraints.PreferSplitMessages && !constraints.PreferOneWordReply {
		if split := behavior.SplitOutgoingMessages(text); len(split) > 0 {
			parts = split
		}
	}

	records := make([]store.MessageRecord, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		rec, err := s.sendAndPersist(ctx, inputPeer, chatID, senderName, part)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no reply parts to send")
	}
	return records, nil
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

func (s *Service) inputPeerFromChatID(ctx context.Context, chatID string) (tg.InputPeerClass, error) {
	chatID = strings.TrimSpace(chatID)
	if !strings.HasPrefix(chatID, "user:") {
		return nil, fmt.Errorf("only private user chats are supported")
	}
	rawUserID := strings.TrimSpace(strings.TrimPrefix(chatID, "user:"))
	userID, err := strconv.ParseInt(rawUserID, 10, 64)
	if err != nil || userID <= 0 {
		return nil, fmt.Errorf("invalid chat_id: %s", chatID)
	}
	resolved, err := s.peerMgr.ResolveUserID(ctx, userID)
	if err == nil {
		return resolved.InputPeer(), nil
	}
	return s.resolveInputPeerFromDialogs(ctx, userID)
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

func (s *Service) usernameFromMessage(ctx context.Context, entities tg.Entities, msg *tg.Message) string {
	from, ok := msg.GetFromID()
	if ok && from != nil {
		if p, ok := from.(*tg.PeerUser); ok {
			return s.usernameFromUserID(ctx, entities, p.UserID)
		}
	}
	if peerUser, ok := msg.GetPeerID().(*tg.PeerUser); ok {
		return s.usernameFromUserID(ctx, entities, peerUser.UserID)
	}
	return ""
}

func (s *Service) usernameFromUserID(ctx context.Context, entities tg.Entities, userID int64) string {
	if userID == 0 {
		return ""
	}
	u, ok := entities.Users[userID]
	if ok && u != nil {
		if username := strings.TrimSpace(u.Username); username != "" {
			return username
		}
	}
	resolved, err := s.peerMgr.ResolveUserID(ctx, userID)
	if err != nil {
		return ""
	}
	if username, ok := resolved.Username(); ok {
		return strings.TrimSpace(username)
	}
	return ""
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
