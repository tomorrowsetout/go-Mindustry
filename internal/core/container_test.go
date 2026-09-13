package core

import (
	"context"
	"testing"
)

type fakeModule struct {
	name string
	deps []string
}

func (m *fakeModule) Name() string               { return m.name }
func (m *fakeModule) Dependencies() []string     { return m.deps }
func (m *fakeModule) Init(_ Deps) error           { return nil }
func (m *fakeModule) Start() error                { return nil }
func (m *fakeModule) Stop(_ context.Context) error { return nil }

func TestContainerStartsInDependencyOrder(t *testing.T) {
	c := NewContainer()
	c.Register(&fakeModule{name: "b", deps: []string{"a"}})
	c.Register(&fakeModule{name: "a"})
	c.Register(&fakeModule{name: "c", deps: []string{"b"}})

	if err := c.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// Dependency order must be a, b, c.
	want := []string{"a", "b", "c"}
	got := c.order
	if len(got) != len(want) {
		t.Fatalf("expected order %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected order %v, got %v", want, got)
		}
	}
	for _, n := range []string{"a", "b", "c"} {
		if c.State(n) != StateStopped {
			t.Fatalf("module %s expected stopped, got %v", n, c.State(n))
		}
	}
}

func TestContainerDetectsCycle(t *testing.T) {
	c := NewContainer()
	c.Register(&fakeModule{name: "x", deps: []string{"y"}})
	c.Register(&fakeModule{name: "y", deps: []string{"x"}})
	if err := c.Init(); err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}
