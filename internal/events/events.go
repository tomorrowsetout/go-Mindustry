package events

// Event interface
type Event interface {
	GetTrigger() Trigger
	GetTimestamp() interface{}
	GetType() string
}

// Trigger trigger type
type Trigger uint32

// BaseEvent base event
type BaseEvent struct{}

// GetTrigger get trigger
func (e *BaseEvent) GetTrigger() Trigger { return 0 }

// GetTimestamp get timestamp
func (e *BaseEvent) GetTimestamp() interface{} { return nil }

// GetType get type
func (e *BaseEvent) GetType() string { return "BaseEvent" }

// GlobalEventManager global event manager
var GlobalEventManager *EventManager

// EventManager event manager
type EventManager struct{}

// NewEventManager create event manager
func NewEventManager() *EventManager { return &EventManager{} }

// Dispatch dispatch event
func Dispatch(ev Event) {}

// AddHook add hook
func AddHook(trigger Trigger, hook interface{}) {}

// AddAllHook add all hook
func AddAllHook(hook interface{}) {}

// DispatchWithTrigger dispatch with trigger
func DispatchWithTrigger(trigger Trigger) {}
