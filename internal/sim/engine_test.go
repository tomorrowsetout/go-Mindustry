package sim

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatchCoversAllWorkExactlyOnce(t *testing.T) {
	e := NewEngine(Config{TPS: 60, Cores: 4, Partitions: 4})
	defer e.Stop()

	const totalWork = 37
	seen := make([]int32, totalWork)
	stats := e.Dispatch(totalWork, func(p Partition) {
		for i := p.Start; i < p.End; i++ {
			atomic.AddInt32(&seen[i], 1)
		}
	})

	if stats.TotalWork != totalWork {
		t.Fatalf("expected totalWork=%d, got %d", totalWork, stats.TotalWork)
	}
	if stats.Partitions != 4 {
		t.Fatalf("expected 4 partitions, got %d", stats.Partitions)
	}
	if stats.WorkerCount != 4 {
		t.Fatalf("expected 4 workers, got %d", stats.WorkerCount)
	}
	for i, count := range seen {
		if count != 1 {
			t.Fatalf("expected work index %d to run once, got %d", i, count)
		}
	}
}

func TestRunTickRecordsDispatchStats(t *testing.T) {
	e := NewEngine(Config{TPS: 60, Cores: 4, Partitions: 3})
	defer e.Stop()

	e.RunTick(time.Second/60, func() {
		e.Dispatch(128, func(_ Partition) {})
	})

	stats := e.Stats()
	if stats.Tick != 1 {
		t.Fatalf("expected tick=1, got %d", stats.Tick)
	}
	if stats.LastTickTime.IsZero() {
		t.Fatal("expected LastTickTime to be recorded")
	}
	if stats.Partitions != 3 {
		t.Fatalf("expected partitions=3, got %d", stats.Partitions)
	}
	if stats.TotalWork != 128 {
		t.Fatalf("expected totalWork=128, got %d", stats.TotalWork)
	}
	if stats.WorkerCount != 3 {
		t.Fatalf("expected workerCount=3, got %d", stats.WorkerCount)
	}
	if stats.LastDispatch.TotalWork != 128 {
		t.Fatalf("expected last dispatch totalWork=128, got %d", stats.LastDispatch.TotalWork)
	}
	if stats.LastDispatch.Partitions != 3 {
		t.Fatalf("expected last dispatch partitions=3, got %d", stats.LastDispatch.Partitions)
	}
}
