package js

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
)

// ErrRuntimeNotInit means runtime not initialized
var ErrRuntimeNotInit = errors.New("runtime not initialized")

// ErrFunctionNotFound means function not found
var ErrFunctionNotFound = errors.New("function not found")

// ErrCallbackExists means callback already exists
var ErrCallbackExists = errors.New("callback already exists")

// JSRuntime represents JavaScript runtime environment
// Note: This is a mock implementation, actual implementation needs to integrate Duktape, Goja or other JS engines
type JSRuntime struct {
	mu              sync.RWMutex
	initialized     bool
	globalVariables map[string]interface{}
	callbacks       map[string]func(...interface{}) interface{}
}

// NewJSRuntime create new JavaScript runtime
func NewJSRuntime() *JSRuntime {
	return &JSRuntime{
		globalVariables: make(map[string]interface{}),
		callbacks:       make(map[string]func(...interface{}) interface{}),
	}
}

// Init initialize runtime
// Note: Actual implementation needs to call JavaScript engine initialization function
func (r *JSRuntime) Init() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		return nil
	}

	// Initialize global variables
	r.globalVariables["console"] = map[string]interface{}{
		"log":    r.consoleLog,
		"warn":   r.consoleWarn,
		"error":  r.consoleError,
		"info":   r.consoleInfo,
	}

	r.globalVariables["Math"] = map[string]interface{}{
		"PI":        3.141592653589793,
		"E":         2.718281828459045,
		"sqrt":      mathSqrt,
		"random":    mathRandom,
		"sin":       mathSin,
		"cos":       mathCos,
		"tan":       mathTan,
	}

	r.initialized = true
	return nil
}

// IsInitialized check if runtime is initialized
func (r *JSRuntime) IsInitialized() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.initialized
}

// Run runs JavaScript code
func (r *JSRuntime) Run(code string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return ErrRuntimeNotInit
	}

	// actual implementation would execute the code
	_ = code
	return nil
}

// SetVariable sets a global variable
func (r *JSRuntime) SetVariable(name string, value interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return ErrRuntimeNotInit
	}

	r.globalVariables[name] = value
	return nil
}

// GetVariable gets a global variable
func (r *JSRuntime) GetVariable(name string) (interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.initialized {
		return nil, ErrRuntimeNotInit
	}

	value, exists := r.globalVariables[name]
	if !exists {
		return nil, ErrFunctionNotFound
	}
	return value, nil
}

// SetGlobal sets a global variable (alias for SetVariable)
func (r *JSRuntime) SetGlobal(name string, value interface{}) error {
	return r.SetVariable(name, value)
}

// CallFunction calls a function by name
func (r *JSRuntime) CallFunction(name string, args ...interface{}) (interface{}, error) {
	return r.CallCallback(name, args...)
}

// Close closes the runtime and releases resources
func (r *JSRuntime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.globalVariables = nil
	r.callbacks = nil
	r.initialized = false

	return nil
}

// SetCallback sets a callback function
func (r *JSRuntime) SetCallback(name string, fn func(...interface{}) interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return ErrRuntimeNotInit
	}

	if _, exists := r.callbacks[name]; exists {
		return ErrCallbackExists
	}

	r.callbacks[name] = fn
	return nil
}

// CallCallback calls a callback function
func (r *JSRuntime) CallCallback(name string, args ...interface{}) (interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.initialized {
		return nil, ErrRuntimeNotInit
	}

	fn, exists := r.callbacks[name]
	if !exists {
		return nil, ErrFunctionNotFound
	}
	return fn(args...), nil
}

// consoleLog console.log implementation
func (r *JSRuntime) consoleLog(args ...interface{}) {
	fmt.Println(args...)
}

// consoleWarn console.warn implementation
func (r *JSRuntime) consoleWarn(args ...interface{}) {
	fmt.Println(append([]interface{}{"WARN:"}, args...)...)
}

// consoleError console.error implementation
func (r *JSRuntime) consoleError(args ...interface{}) {
	fmt.Println(append([]interface{}{"ERROR:"}, args...)...)
}

// consoleInfo console.info implementation
func (r *JSRuntime) consoleInfo(args ...interface{}) {
	fmt.Println(append([]interface{}{"INFO:"}, args...)...)
}

// mathSqrt Math.sqrt implementation
func mathSqrt(args ...interface{}) interface{} {
	if len(args) == 0 {
		return float64(0)
	}
	x, _ := args[0].(float64)
	return math.Sqrt(x)
}

// mathRandom Math.random implementation
func mathRandom(args ...interface{}) interface{} {
	return rand.Float64()
}

// mathSin Math.sin implementation
func mathSin(args ...interface{}) interface{} {
	if len(args) == 0 {
		return float64(0)
	}
	x, _ := args[0].(float64)
	return math.Sin(x)
}

// mathCos Math.cos implementation
func mathCos(args ...interface{}) interface{} {
	if len(args) == 0 {
		return float64(0)
	}
	x, _ := args[0].(float64)
	return math.Cos(x)
}

// mathTan Math.tan implementation
func mathTan(args ...interface{}) interface{} {
	if len(args) == 0 {
		return float64(0)
	}
	x, _ := args[0].(float64)
	return math.Tan(x)
}
