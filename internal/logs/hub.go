package logs

import "sync"

type Hub struct {
	mu      sync.RWMutex
	nextID  int64
	clients map[int64]chan Event
}

func NewHub() *Hub {
	return &Hub{clients: map[int64]chan Event{}}
}

func (h *Hub) Subscribe() (int64, <-chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	ch := make(chan Event, 64)
	h.clients[h.nextID] = ch
	return h.nextID, ch
}

func (h *Hub) Unsubscribe(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.clients[id]; ok {
		delete(h.clients, id)
		close(ch)
	}
}

func (h *Hub) Publish(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.clients {
		select {
		case ch <- event:
		default:
		}
	}
}
