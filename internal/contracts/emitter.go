// Package contracts — emitter.go defines the Emitter interface and provides
// concrete implementations for publishing sync lifecycle events.
//
// The Emitter is the central event bus for the Bridgit sync engine. Phases
// emit events, and subscribers react to them (writing to reports, log files,
// or future webhook triggers). The design is intentionally synchronous —
// matching the codebase's "boring and predictable" philosophy.

package contracts

import (
	"sync"
	"time"
)

// EventHandler is a callback function invoked synchronously when an event
// is emitted. Handlers should complete quickly to avoid blocking the
// engine's execution. Long-running side effects should be deferred.
type EventHandler func(event Event)

// Emitter defines the contract for publishing and subscribing to sync
// lifecycle events. All engine phases receive an Emitter to announce
// meaningful state transitions without coupling to specific consumers.
type Emitter interface {
	// Emit publishes an event to all registered subscribers synchronously.
	// The Timestamp field is set automatically if not already populated.
	Emit(event Event)

	// Subscribe registers a handler that will be called for every future event.
	// Handlers are invoked in the order they were registered.
	Subscribe(handler EventHandler)
}

// InMemoryEmitter is a synchronous, in-process event bus.
//
// Events are dispatched to subscribers immediately during the Emit call.
// No goroutines, no channels, no buffering — the simplest possible
// implementation that satisfies the Emitter contract. Thread-safe via mutex
// to support potential future concurrent phase execution.
type InMemoryEmitter struct {
	// subscribers holds all registered event handlers in registration order.
	subscribers []EventHandler

	// subscriberMutex protects the subscribers slice from concurrent access.
	// Currently the engine runs single-threaded, but this futureproofs
	// against parallel phase execution without requiring a redesign.
	subscriberMutex sync.RWMutex
}

// NewEmitter creates a ready-to-use InMemoryEmitter with no subscribers.
//
// Subscribers are attached via Subscribe() after construction. The emitter
// is safe to use immediately — emitting before any subscribers are attached
// is a silent no-op per event.
func NewEmitter() *InMemoryEmitter {
	return &InMemoryEmitter{
		subscribers: make([]EventHandler, 0),
	}
}

// Emit publishes an event to all registered subscribers in order.
//
// Sets the Timestamp field to the current time if the caller left it empty.
// Each subscriber is called synchronously — the engine blocks until all
// handlers complete. This guarantees events are fully processed before
// the emitting phase continues.
func (emitter *InMemoryEmitter) Emit(event Event) {
	// Auto-populate the timestamp so callers don't need to track time.
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Read-lock the subscriber list to allow concurrent Emit calls
	// while preventing modification during iteration.
	emitter.subscriberMutex.RLock()
	defer emitter.subscriberMutex.RUnlock()

	// Dispatch to each subscriber in registration order.
	for _, handler := range emitter.subscribers {
		handler(event)
	}
}

// Subscribe registers a new event handler that will receive all future events.
//
// Handlers are called in the order they are registered. There is no way to
// unsubscribe — handlers live for the duration of the engine run. This is
// intentional: Bridgit runs are short-lived single-execution processes.
func (emitter *InMemoryEmitter) Subscribe(handler EventHandler) {
	// Write-lock to safely append while other goroutines may be emitting.
	emitter.subscriberMutex.Lock()
	defer emitter.subscriberMutex.Unlock()

	emitter.subscribers = append(emitter.subscribers, handler)
}

// NopEmitter is a silent no-op implementation of the Emitter interface.
//
// Use this when events are not needed (e.g., in tests or CLI tools that
// only care about the sync result, not the lifecycle events).
type NopEmitter struct{}

// Emit does nothing. Events are silently discarded.
func (nop *NopEmitter) Emit(event Event) {}

// Subscribe does nothing. Handlers are silently ignored.
func (nop *NopEmitter) Subscribe(handler EventHandler) {}
