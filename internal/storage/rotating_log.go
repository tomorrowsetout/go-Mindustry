package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type rotatingLog struct {
	mu       sync.Mutex
	dir      string
	prefix   string
	ext      string
	maxSize  int64
	maxFiles int
	curFile  *os.File
	curSize  int64
}

func newRotatingLog(dir, prefix, ext string, maxSize int64, maxFiles int) (*rotatingLog, error) {
	if maxSize <= 0 {
		maxSize = 5 * 1024 * 1024
	}
	if maxFiles <= 0 {
		maxFiles = 50
	}
	if strings.TrimSpace(ext) == "" {
		ext = ".log"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := &rotatingLog{
		dir:      dir,
		prefix:   sanitizeFilename(prefix),
		ext:      ext,
		maxSize:  maxSize,
		maxFiles: maxFiles,
	}
	if err := l.rotateLocked(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *rotatingLog) WriteLine(p []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.curFile == nil {
		if err := l.rotateLocked(); err != nil {
			return err
		}
	}
	if l.curSize+int64(len(p)) > l.maxSize {
		if err := l.rotateLocked(); err != nil {
			return err
		}
	}
	n, err := l.curFile.Write(p)
	l.curSize += int64(n)
	return err
}

func (l *rotatingLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.curFile != nil {
		err := l.curFile.Close()
		l.curFile = nil
		l.curSize = 0
		return err
	}
	return nil
}

func (l *rotatingLog) rotateLocked() error {
	if l.curFile != nil {
		_ = l.curFile.Close()
		l.curFile = nil
	}
	name := fmt.Sprintf("%s-%s%s", l.prefix, time.Now().Format("20060102-150405.000000000"), l.ext)
	path := filepath.Join(l.dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	l.curFile = f
	l.curSize = fi.Size()
	l.cleanupLocked()
	return nil
}

func (l *rotatingLog) cleanupLocked() {
	pattern := filepath.Join(l.dir, l.prefix+"-*"+l.ext)
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) <= l.maxFiles {
		return
	}
	type fileInfo struct {
		path string
		time time.Time
	}
	items := make([]fileInfo, 0, len(matches))
	for _, m := range matches {
		st, err := os.Stat(m)
		if err != nil {
			continue
		}
		items = append(items, fileInfo{path: m, time: st.ModTime()})
	}
	if len(items) <= l.maxFiles {
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].time.Before(items[j].time) })
	excess := len(items) - l.maxFiles
	for i := 0; i < excess; i++ {
		_ = os.Remove(items[i].path)
	}
}
