package ws

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/KurisuNo1/InterviewAgent/internal/interaction"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Hub maintains the set of active WebSocket clients and broadcasts events to them.
type Hub struct {
	mu       sync.RWMutex
	sessions map[string]map[*Client]bool // sessionID -> clients
	svc      interaction.InterviewService
}

// NewHub creates a new WebSocket hub.
func NewHub(svc interaction.InterviewService) *Hub {
	return &Hub{
		sessions: make(map[string]map[*Client]bool),
		svc:      svc,
	}
}

// Register adds a client to a session.
func (h *Hub) Register(sessionID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sessions[sessionID] == nil {
		h.sessions[sessionID] = make(map[*Client]bool)
	}
	h.sessions[sessionID][client] = true
}

// Unregister removes a client from a session.
func (h *Hub) Unregister(sessionID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients, ok := h.sessions[sessionID]; ok {
		if _, ok := clients[client]; ok {
			delete(clients, client)
			close(client.send)
		}
		if len(clients) == 0 {
			delete(h.sessions, sessionID)
		}
	}
}

// Broadcast sends an event to all clients in a session.
func (h *Hub) Broadcast(sessionID string, event *WSEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients, ok := h.sessions[sessionID]; ok {
		for client := range clients {
			select {
			case client.send <- event:
			default:
				// Channel full — client is too slow; drop it.
				// Unregister will be called by ReadPump when it notices the
				// connection is broken, so just remove the client from the set.
				delete(clients, client)
			}
		}
		if len(clients) == 0 {
			delete(h.sessions, sessionID)
		}
	}
}

// ServeWS handles WebSocket upgrade requests.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		wrappedErr := "ws upgrade failed: " + err.Error()
		http.Error(w, wrappedErr, http.StatusInternalServerError)
		return
	}

	client := NewClient(h, conn, sessionID)
	// Use a cancellable background context so service calls outlive the HTTP
	// request, but are cleaned up when the WebSocket client disconnects.
	ctx, cancel := context.WithCancel(context.Background())
	client.ctx = ctx
	client.cancel = cancel

	h.Register(sessionID, client)

	// Subscribe to orchestrator events for real-time push.
	eventCh, err := h.svc.Subscribe(ctx, sessionID)
	if err == nil {
		go h.forwardEvents(eventCh, client, sessionID)
	} else {
		log.Printf("[WS] Failed to subscribe to events for session %s: %v", sessionID, err)
	}

	go client.WritePump()
	go client.ReadPump()
}

// forwardEvents forwards orchestrator events to the WebSocket client.
func (h *Hub) forwardEvents(eventCh <-chan *interaction.InterviewEvent, client *Client, sessionID string) {
	log.Printf("[WS] Event forwarder started for session %s", sessionID)
	for event := range eventCh {
		wsEvent := &WSEvent{
			Type:      event.Type,
			SessionID: event.SessionID,
			Data:      event.Data,
		}
		h.Broadcast(sessionID, wsEvent)
	}
	log.Printf("[WS] Event forwarder stopped for session %s", sessionID)
}

// Service returns the underlying InterviewService.
func (h *Hub) Service() interaction.InterviewService {
	return h.svc
}

// Close disconnects all clients and cleans up.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sessionID, clients := range h.sessions {
		for client := range clients {
			close(client.send)
			client.conn.Close()
		}
		delete(h.sessions, sessionID)
	}
}
