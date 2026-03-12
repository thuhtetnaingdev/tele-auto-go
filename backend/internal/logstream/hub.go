package logstream

import (
	"sync"
	"time"
)

type Entry struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

type Hub struct {
	mu       sync.RWMutex
	nextID   int
	ringSize int
	ring     []Entry
	subs     map[int]chan Entry
}

func NewHub(ringSize int) *Hub {
	if ringSize < 1 {
		ringSize = 1
	}
	return &Hub{
		ringSize: ringSize,
		ring:     make([]Entry, 0, ringSize),
		subs:     make(map[int]chan Entry),
	}
}

func (h *Hub) Publish(e Entry) {
	if e.Time == "" {
		e.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}

	h.mu.Lock()
	if len(h.ring) == h.ringSize {
		copy(h.ring, h.ring[1:])
		h.ring[len(h.ring)-1] = e
	} else {
		h.ring = append(h.ring, e)
	}
	subs := make([]chan Entry, 0, len(h.subs))
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

func (h *Hub) Snapshot(limit int) []Entry {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if limit < 1 || limit > len(h.ring) {
		limit = len(h.ring)
	}
	start := len(h.ring) - limit
	out := make([]Entry, 0, limit)
	out = append(out, h.ring[start:]...)
	return out
}

func (h *Hub) Subscribe() (<-chan Entry, func()) {
	h.mu.Lock()
	id := h.nextID
	h.nextID++
	ch := make(chan Entry, 128)
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
