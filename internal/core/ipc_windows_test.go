//go:build windows

package core

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitForIPCConn(t *testing.T, endpoint string) net.Conn {
	t.Helper()
	var lastErr error
	for attempt := 0; attempt < 40; attempt++ {
		conn, err := dialIPC(endpoint, 250*time.Millisecond)
		if err == nil {
			return conn
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("dial %s failed: %v", endpoint, lastErr)
	return nil
}

func TestRemoteCore3WorldCacheRoundTrip(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "worlds", "file.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("file.msav not present in workspace")
		}
		t.Fatalf("stat file.msav: %v", err)
	}

	endpoint := normalizeIPCEndpoint(fmt.Sprintf("mdt-server-test-core3-%d", time.Now().UnixNano()))
	done := make(chan error, 1)
	go func() {
		done <- RunChildCore("core3", endpoint, 0)
	}()

	conn := waitForIPCConn(t, endpoint)
	defer conn.Close()

	c3 := NewCore3(Config{Name: "remote-core3"})
	c3.AttachRemote(newIPCClient(conn))

	res, err := c3.GetWorldCache(path)
	if err != nil {
		t.Fatalf("remote GetWorldCache: %v", err)
	}
	if len(res.Data) == 0 {
		t.Fatal("expected remote core3 to return worldstream payload")
	}
	if err := c3.InvalidateWorldCache(path); err != nil {
		t.Fatalf("remote InvalidateWorldCache: %v", err)
	}
	if err := c3.remote.client.Call("shutdown", nil, nil); err != nil {
		t.Fatalf("shutdown core3 child: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("core3 child exited with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for core3 child shutdown")
	}
}

func TestRemoteCore4PolicyRoundTrip(t *testing.T) {
	endpoint := normalizeIPCEndpoint(fmt.Sprintf("mdt-server-test-core4-%d", time.Now().UnixNano()))
	done := make(chan error, 1)
	go func() {
		done <- RunChildCore("core4", endpoint, 0)
	}()

	conn := waitForIPCConn(t, endpoint)
	defer conn.Close()

	c4 := NewCore4(Config{Name: "remote-core4"})
	c4.AttachRemote(newIPCClient(conn))

	res, err := c4.AllowConnection("127.0.0.1", "uuid-remote")
	if err != nil {
		t.Fatalf("remote AllowConnection: %v", err)
	}
	if !res.Allowed {
		t.Fatalf("expected remote core4 to allow first connection, got %+v", res)
	}
	shard, err := c4.PlayerShard("uuid-remote", "127.0.0.1")
	if err != nil {
		t.Fatalf("remote PlayerShard: %v", err)
	}
	if shard.PlayerShard <= 0 {
		t.Fatalf("expected positive remote player shard, got %+v", shard)
	}
	if err := c4.remote.client.Call("shutdown", nil, nil); err != nil {
		t.Fatalf("shutdown core4 child: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("core4 child exited with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for core4 child shutdown")
	}
}
