package plugin

import (
	"sort"
	"sync"
)

// EventPayload carries event data to handlers. Handlers receive a
// mutable map; returning false from a handler marks the event as
// consumed (mirrors YF's cancellable events such as PlayerChatEvent).
type EventPayload map[string]any

// EventHandler processes one fired event. Returning false cancels
// propagation to lower-priority handlers.
type EventHandler func(payload EventPayload) bool

// EventBinding mirrors YZFEventBinding: a handle that removes one
// registration when the owning module is unloaded.
type EventBinding struct {
	Event  string
	Module string
	id     uint64
}

// EventBus mirrors YZFEventRegistry + Events.fire semantics: handlers
// run in descending priority order, a handler returning false stops
// the chain, and every registration is owned by a module so reloads
// can detach cleanly.
type EventBus struct {
	mu       sync.Mutex
	nextID   uint64
	handlers map[string][]eventEntry
}

type eventEntry struct {
	id       uint64
	module   string
	priority int
	handler  EventHandler
}

// NewEventBus creates an empty bus.
func NewEventBus() *EventBus {
	return &EventBus{handlers: make(map[string][]eventEntry)}
}

// Subscribe registers a handler for an event name. Higher priority
// handlers run first. The module ID owns the registration.
func (b *EventBus) Subscribe(event, module string, priority int, handler EventHandler) *EventBinding {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	entry := eventEntry{id: b.nextID, module: module, priority: priority, handler: handler}
	b.handlers[event] = append(b.handlers[event], entry)
	sort.SliceStable(b.handlers[event], func(i, j int) bool {
		return b.handlers[event][i].priority > b.handlers[event][j].priority
	})
	return &EventBinding{Event: event, Module: module, id: b.nextID}
}

// Unsubscribe removes one binding.
func (b *EventBus) Unsubscribe(binding *EventBinding) {
	if binding == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.removeLocked(binding.Event, binding.id)
}

// UnsubscribeModule removes every handler registered by a module.
func (b *EventBus) UnsubscribeModule(module string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for event, entries := range b.handlers {
		kept := entries[:0]
		for _, entry := range entries {
			if entry.module != module {
				kept = append(kept, entry)
			}
		}
		if len(kept) == 0 {
			delete(b.handlers, event)
		} else {
			b.handlers[event] = kept
		}
	}
}

// Fire dispatches an event. It returns false when any handler consumed
// the event (handler returned false).
func (b *EventBus) Fire(event string, payload EventPayload) bool {
	b.mu.Lock()
	entries := append([]eventEntry(nil), b.handlers[event]...)
	b.mu.Unlock()
	consumed := true
	for _, entry := range entries {
		if !safeHandle(entry.handler, payload) {
			return false
		}
	}
	return consumed
}

func (b *EventBus) removeLocked(event string, id uint64) {
	entries, ok := b.handlers[event]
	if !ok {
		return
	}
	kept := entries[:0]
	for _, entry := range entries {
		if entry.id != id {
			kept = append(kept, entry)
		}
	}
	if len(kept) == 0 {
		delete(b.handlers, event)
	} else {
		b.handlers[event] = kept
	}
}

func safeHandle(handler EventHandler, payload EventPayload) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = true // a panicking handler must not break the chain
		}
	}()
	return handler(payload)
}

// KnownEvents lists the YZF event names this server fires. Runtimes
// expose these to scripts; firing an unknown event is a no-op.
var KnownEvents = []string{
	"PlayerJoin",
	"PlayerLeave",
	"PlayEvent",
	"ResetEvent",
	"ServerLoadEvent",
	"WorldLoadEvent",
	"GameOverEvent",
	"PlayerChatEvent",
	"SendPacketEvent",
	"ReceivePacketEvent",
	"PlayerTeamChangedEvent",
	"HealthChangedEvent",
	"LogicAssembledEvent",
}
