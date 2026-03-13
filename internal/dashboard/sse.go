package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

// Event represents an SSE event with a named type and pre-rendered HTML data.
type Event struct {
	Type string
	Data string
}

// Broker manages SSE client connections and broadcasts events via fan-out.
type Broker struct {
	clients    map[chan Event]struct{}
	register   chan chan Event
	unregister chan chan Event
	broadcast  chan Event
	mu         sync.RWMutex
}

// NewBroker creates a new SSE broker with buffered channels.
func NewBroker() *Broker {
	return &Broker{
		clients:    make(map[chan Event]struct{}),
		register:   make(chan chan Event),
		unregister: make(chan chan Event),
		broadcast:  make(chan Event, 256),
	}
}

// Run starts the broker event loop. It blocks until ctx is cancelled.
func (b *Broker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			b.mu.Lock()
			for ch := range b.clients {
				delete(b.clients, ch)
				close(ch)
			}
			b.mu.Unlock()
			return

		case ch := <-b.register:
			b.mu.Lock()
			b.clients[ch] = struct{}{}
			b.mu.Unlock()

		case ch := <-b.unregister:
			b.mu.Lock()
			if _, ok := b.clients[ch]; ok {
				delete(b.clients, ch)
				close(ch)
			}
			b.mu.Unlock()

		case event := <-b.broadcast:
			b.mu.RLock()
			for ch := range b.clients {
				// Non-blocking send; skip slow clients.
				select {
				case ch <- event:
				default:
				}
			}
			b.mu.RUnlock()
		}
	}
}

// ServeHTTP handles SSE connections. It sets appropriate headers, registers the
// client, and streams events until the client disconnects.
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan Event, 32)
	b.register <- ch
	defer func() {
		b.unregister <- ch
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data)
			flusher.Flush()
		}
	}
}

// Broadcast sends an event to all connected clients. If the broadcast channel
// is full, the event is dropped (non-blocking).
func (b *Broker) Broadcast(event Event) {
	select {
	case b.broadcast <- event:
	default:
	}
}
