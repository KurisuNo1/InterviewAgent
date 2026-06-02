package ws

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 8192
)

// Client represents a single WebSocket connection.
type Client struct {
	hub       *Hub
	conn      *websocket.Conn
	send      chan *WSEvent
	sessionID string
	ctx       context.Context
	cancel    context.CancelFunc // cancels the event subscription on disconnect
}

// NewClient creates a new WebSocket client.
func NewClient(hub *Hub, conn *websocket.Conn, sessionID string) *Client {
	return &Client{
		hub:       hub,
		conn:      conn,
		send:      make(chan *WSEvent, 256),
		sessionID: sessionID,
		ctx:       context.Background(),
	}
}

// ReadPump reads messages from the WebSocket connection and dispatches them.
func (c *Client) ReadPump() {
	defer func() {
		if c.cancel != nil {
			c.cancel()
		}
		c.hub.Unregister(c.sessionID, c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws read error: %v", err)
			}
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.sendError("failed to parse message: " + err.Error())
			continue
		}

		c.dispatch(msg)
	}
}

// WritePump writes events to the WebSocket connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case event, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteJSON(event); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// dispatch routes an incoming message based on its type.
func (c *Client) dispatch(msg WSMessage) {
	switch msg.Type {
	case TypeStart:
		event, err := c.hub.svc.StartInterview(c.ctx, c.sessionID)
		if err != nil {
			c.sendError(err.Error())
			return
		}
		c.hub.Broadcast(c.sessionID, &WSEvent{
			Type:      event.Type,
			SessionID: event.SessionID,
			Data:      event.Data,
			Streaming: event.Streaming,
		})
	case TypeAnswer:
		var payload struct {
			Answer string `json:"answer"`
		}
		json.Unmarshal(msg.Payload, &payload)
		event, err := c.hub.svc.SubmitAnswer(c.ctx, c.sessionID, payload.Answer)
		if err != nil {
			c.sendError(err.Error())
			return
		}
		c.hub.Broadcast(c.sessionID, &WSEvent{
			Type:      event.Type,
			SessionID: event.SessionID,
			Data:      event.Data,
			Streaming: event.Streaming,
		})
	case TypeSkip:
		event, err := c.hub.svc.SkipQuestion(c.ctx, c.sessionID)
		if err != nil {
			c.sendError(err.Error())
			return
		}
		c.hub.Broadcast(c.sessionID, &WSEvent{
			Type:      event.Type,
			SessionID: event.SessionID,
			Data:      event.Data,
		})
	case TypeResume:
		session, err := c.hub.svc.ResumeSession(c.ctx, c.sessionID)
		if err != nil {
			c.sendError(err.Error())
			return
		}
		c.hub.Broadcast(c.sessionID, &WSEvent{
			Type:      "resumed",
			SessionID: c.sessionID,
			Data:      session,
		})
	case TypePing:
		c.send <- &WSEvent{Type: TypePong, SessionID: c.sessionID}
	}
}

func (c *Client) sendError(msg string) {
	c.send <- &WSEvent{
		Type:      TypeError,
		SessionID: c.sessionID,
		Data:      msg,
	}
}
