package logging

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

type Field struct {
	Key   string
	Value any
}

type Logger struct {
	mu  sync.Mutex
	out io.Writer
}

func New(out io.Writer) *Logger {
	if out == nil {
		out = os.Stdout
	}
	return &Logger{out: out}
}

func (l *Logger) Info(msg string, fields ...Field)  { l.log("info", msg, fields...) }
func (l *Logger) Warn(msg string, fields ...Field)  { l.log("warn", msg, fields...) }
func (l *Logger) Error(msg string, fields ...Field) { l.log("error", msg, fields...) }

func (l *Logger) log(level, msg string, fields ...Field) {
	payload := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"level": level,
		"msg":   msg,
	}
	if len(fields) > 0 {
		extra := map[string]any{}
		for _, f := range fields {
			if f.Key == "" {
				continue
			}
			extra[f.Key] = f.Value
		}
		if len(extra) > 0 {
			payload["fields"] = extra
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	enc := json.NewEncoder(l.out)
	_ = enc.Encode(payload)
}
