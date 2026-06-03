// Package eventbus provides a simple callback-based publish/subscribe event bus.
package eventbus

import (
	"log"
	"sync"
)

// Handler processes an event. Errors are logged but do not affect other handlers.
type Handler func(data any)

// Bus is a publish/subscribe event bus.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]*handlerEntry
}

// New creates a new EventBus.
func New() *Bus {
	return &Bus{
		handlers: make(map[string][]*handlerEntry),
	}
}

	// Emit publishes data to all handlers registered on the given channel.
func (b *Bus) Emit(channel string, data any) {
	b.mu.RLock()
	handlers := b.handlers[channel]
	b.mu.RUnlock()

	for _, entry := range handlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("event handler error (%s): %v", channel, r)
				}
			}()
			entry.fn(data)
		}()
	}
}

type handlerEntry struct {
	fn Handler
}

// On registers a handler for the given channel. Returns an unsubscribe function.
func (b *Bus) On(channel string, handler Handler) (unsub func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry := &handlerEntry{fn: handler}
	b.handlers[channel] = append(b.handlers[channel], entry)

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		slice := b.handlers[channel]
		for i, candidate := range slice {
			if candidate == entry {
				b.handlers[channel] = append(slice[:i], slice[i+1:]...)
				return
			}
		}
	}
}

// Clear removes all handlers from all channels.
func (b *Bus) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = make(map[string][]*handlerEntry)
}
