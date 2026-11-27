package daemon

import (
	"encoding/json"
	"sync"
)

// Event is one Server-Sent Event.
type Event struct {
	Name string
	Data []byte
}

func newEvent(name string, v any) Event {
	b, err := json.Marshal(v)
	if err != nil {
		b = []byte("{}")
	}
	return Event{Name: name, Data: b}
}

// Sub is one open browser page. id is the page's own id, used to find it
// when it reports that it is hidden or visible.
type Sub struct {
	ch     chan Event
	id     string
	hidden bool
}

// indexKey is the hub key for the index page.
const indexKey = ""

// Hub fans events out to the pages of each workspace.
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[*Sub]struct{}
}

func newHub() *Hub { return &Hub{subs: map[string]map[*Sub]struct{}{}} }

// Subscribe registers a page for a workspace.
func (h *Hub) Subscribe(ws string) *Sub { return h.SubscribeAs(ws, "", false) }

// SubscribeAs registers a page with its id and whether it starts hidden.
func (h *Hub) SubscribeAs(ws, id string, hidden bool) *Sub {
	s := &Sub{ch: make(chan Event, 256), id: id, hidden: hidden}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[ws] == nil {
		h.subs[ws] = map[*Sub]struct{}{}
	}
	h.subs[ws][s] = struct{}{}
	return s
}

// Unsubscribe removes a page.
func (h *Hub) Unsubscribe(ws string, s *Sub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[ws], s)
	if len(h.subs[ws]) == 0 {
		delete(h.subs, ws)
	}
}

// SetHidden records whether the page with the given id is hidden.
func (h *Hub) SetHidden(ws, id string, hidden bool) {
	if id == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs[ws] {
		if s.id == id {
			s.hidden = hidden
		}
	}
}

// Counts returns the open pages of a workspace and how many of them are
// hidden.
func (h *Hub) Counts(ws string) (total, hidden int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs[ws] {
		total++
		if s.hidden {
			hidden++
		}
	}
	return total, hidden
}

// Viewers counts the open pages of a workspace.
func (h *Hub) Viewers(ws string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[ws])
}

// Total counts all open pages.
func (h *Hub) Total() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, s := range h.subs {
		n += len(s)
	}
	return n
}

// Publish sends events to every page of a workspace. A page that is too slow
// misses events; it gets the full state again when it reconnects.
func (h *Hub) Publish(ws string, evs ...Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs[ws] {
		for _, ev := range evs {
			select {
			case s.ch <- ev:
			default:
			}
		}
	}
}
