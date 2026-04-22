//go:build windows

package core

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
)

func normalizeIPCEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if strings.HasPrefix(endpoint, `\\.\pipe\`) {
		return endpoint
	}
	return `\\.\pipe\` + endpoint
}

func listenIPC(endpoint string) (net.Listener, error) {
	endpoint = normalizeIPCEndpoint(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("ipc endpoint is empty")
	}
	return winio.ListenPipe(endpoint, nil)
}

func dialIPC(endpoint string, timeout time.Duration) (net.Conn, error) {
	endpoint = normalizeIPCEndpoint(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("ipc endpoint is empty")
	}
	return winio.DialPipe(endpoint, &timeout)
}
