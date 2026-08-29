package application

import (
	"context"
	"sync"
)

// RefreshSink accepts a minimal, body-free inbox refresh hint for one connection.
type RefreshSink interface {
	Send(context.Context, RefreshEvent) error
}

// RefreshHub owns live gateway connections within one application process.
type RefreshHub struct {
	mu     sync.Mutex
	nextID uint64
	byUser map[uint]map[uint64]RefreshSink
}

func NewRefreshHub() *RefreshHub {
	return &RefreshHub{byUser: make(map[uint]map[uint64]RefreshSink)}
}

// Register adds a connection for userID and returns an idempotent unregister function.
func (hub *RefreshHub) Register(userID uint, sink RefreshSink) func() {
	if hub == nil || userID == 0 || sink == nil {
		return func() {}
	}
	hub.mu.Lock()
	hub.nextID++
	connectionID := hub.nextID
	connections := hub.byUser[userID]
	if connections == nil {
		connections = make(map[uint64]RefreshSink)
		hub.byUser[userID] = connections
	}
	connections[connectionID] = sink
	hub.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() { hub.unregister(userID, connectionID) })
	}
}

// Publish delivers an event only to live connections for its target user. Failed
// connections are removed so a stale WebSocket cannot retain server memory.
func (hub *RefreshHub) Publish(ctx context.Context, event RefreshEvent) {
	if hub == nil || event.UserID == 0 {
		return
	}
	hub.mu.Lock()
	connections := make([]struct {
		id   uint64
		sink RefreshSink
	}, 0, len(hub.byUser[event.UserID]))
	for id, sink := range hub.byUser[event.UserID] {
		connections = append(connections, struct {
			id   uint64
			sink RefreshSink
		}{id: id, sink: sink})
	}
	hub.mu.Unlock()
	for _, connection := range connections {
		if connection.sink.Send(ctx, event) != nil {
			hub.unregister(event.UserID, connection.id)
		}
	}
}

func (hub *RefreshHub) unregister(userID uint, connectionID uint64) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	connections := hub.byUser[userID]
	if connections == nil {
		return
	}
	delete(connections, connectionID)
	if len(connections) == 0 {
		delete(hub.byUser, userID)
	}
}
