// Package realtime provides SSE fan-out of entity change events.
package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	lensmetrics "github.com/ncdlabs/gitea-lens/internal/metrics"
)

type Event struct {
	Type   string `json:"type"`
	ID     int64  `json:"id"`
	RepoID int64  `json:"repo_id"`
}

// AllowFunc decides whether a subscriber may receive an event.
type AllowFunc func(ev Event) bool

type Hub struct {
	mu      sync.RWMutex
	clients map[chan Event]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan Event]struct{})}
}

func (h *Hub) Run(ctx context.Context) {
	<-ctx.Done()
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		close(ch)
		delete(h.clients, ch)
	}
}

func (h *Hub) Subscribe() chan Event {
	ch := make(chan Event, 64)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *Hub) Publish(ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- ev:
		default:
			lensmetrics.SSEEventsDroppedTotal.Inc()
		}
	}
}

// ServeFiltered streams SSE events allowed by allow (nil = allow all).
func (h *Hub) ServeFiltered(w http.ResponseWriter, r *http.Request, allow AllowFunc) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	if allow == nil {
		allow = func(Event) bool { return true }
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := h.Subscribe()
	defer h.Unsubscribe(ch)

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if !allow(ev) {
				continue
			}
			b, _ := json.Marshal(ev)
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(b)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}
