//go:build !windows

package core

import (
	"fmt"
	"net"
	"time"
)

func normalizeIPCEndpoint(endpoint string) string {
	return endpoint
}

func listenIPC(endpoint string) (net.Listener, error) {
	return nil, fmt.Errorf("named pipe ipc is only supported on windows: %s", endpoint)
}

func dialIPC(endpoint string, _ time.Duration) (net.Conn, error) {
	return nil, fmt.Errorf("named pipe ipc is only supported on windows: %s", endpoint)
}
