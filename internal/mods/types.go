package mods

import (
	"fmt"
)

// ErrInvalidModType error for invalid mod type
var ErrInvalidModType = fmt.Errorf("mods: invalid mod type")

// ErrModNotFound error for mod not found
var ErrModNotFound = fmt.Errorf("mods: mod not found")

// ErrModAlreadyExists error for mod already exists
var ErrModAlreadyExists = fmt.Errorf("mods: mod already exists")

// Log represents logging interface
type Log interface {
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// simpleLogger simple logger
type simpleLogger struct{}

var modLog Log = &simpleLogger{}

func (l *simpleLogger) Info(msg string, args ...interface{}) {
	fmt.Printf("[MOD] "+msg+"\n", args...)
}

func (l *simpleLogger) Warn(msg string, args ...interface{}) {
	fmt.Printf("[MOD WARN] "+msg+"\n", args...)
}

func (l *simpleLogger) Error(msg string, args ...interface{}) {
	fmt.Printf("[MOD ERROR] "+msg+"\n", args...)
}
