package conversationstream

import (
	"sync"
	"time"
)

type Event struct {
	Type       string `json:"type"`
	ChatID     string `json:"chatId,omitempty"`
	MessageID  string `json:"telegramMessageId,omitempty"`
	Direction  string `json:"direction,omitempty"`
	Text       string `json:"text,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Reason     string `json:"reason,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
	OccurredAt string `json:"occurredAt"`
}

type Hub struct {
	mu       sync.RWMutex
	nextID   int
	ringSize int
	ring     []Event
	subs     map[int]chan Event
}

func NewHub(ringSize int) *Hub {
	if ringSize < 1 {
		ringSize = 1
	}
	return &Hub{
		ringSize: ringSize,
		ring:     make([]Event, 0, ringSize),
		subs:     make(map[int]chan Event),
	}
}

func (h *Hub) Publish(e Event) {
	if e.OccurredAt == "" {
		e.OccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	h.mu.Lock()
	if len(h.ring) == h.ringSize {
		copy(h.ring, h.ring[1:])
		h.ring[len(h.ring)-1] = e
	} else {
		h.ring = append(h.ring, e)
	}
	subs := make([]chan Event, 0, len(h.subs))
	for _, ch := range h.subs {
		subs = append(subs, ch)
	}
	h.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func (h *Hub) Snapshot(limit int) []Event {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if limit < 1 || limit > len(h.ring) {
		limit = len(h.ring)
	}
	start := len(h.ring) - limit
	out := make([]Event, 0, limit)
	out = append(out, h.ring[start:]...)
	return out
}

func (h *Hub) Subscribe() (<-chan Event, func()) {
	h.mu.Lock()
	id := h.nextID
	h.nextID++
	ch := make(chan Event, 128)
	h.subs[id] = ch
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		if sub, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(sub)
		}
		h.mu.Unlock()
	}

	return ch, unsub
}
