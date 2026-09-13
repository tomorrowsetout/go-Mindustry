package net

import (
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func snapshotPacketEntityIDs(t *testing.T, srv *Server, packets []*protocol.Remote_NetClient_entitySnapshot_32) []int32 {
	t.Helper()
	ids := make([]int32, 0, len(packets)*4)
	for _, packet := range packets {
		for _, entry := range decodeEntitySnapshotPacket(t, srv, packet) {
			ids = append(ids, entry.ID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func TestBuildEntitySnapshotPacketsForConnCachedReusesBaseAndPreservesHiddenFiltering(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.entitySnapshotIntervalNs.Store(int64(time.Hour))
	srv.stateSnapshotIntervalNs.Store(int64(time.Hour))

	var buildCalls atomic.Int32
	srv.ExtraEntitySnapshotEntitiesFn = func() ([]protocol.UnitSyncEntity, error) {
		buildCalls.Add(1)
		return []protocol.UnitSyncEntity{
			&protocol.UnitEntitySync{
				IDValue:      7001,
				ClassIDValue: 30,
				ClassIDSet:   true,
				TypeID:       35,
				TeamID:       2,
				Health:       100,
				X:            24,
				Y:            40,
			},
			&protocol.UnitEntitySync{
				IDValue:      7002,
				ClassIDValue: 30,
				ClassIDSet:   true,
				TypeID:       35,
				TeamID:       2,
				Health:       100,
				X:            32,
				Y:            40,
			},
		}, nil
	}

	hiddenViewer := &Conn{id: 1}
	visibleViewer := &Conn{id: 2}
	srv.EntitySnapshotHiddenFn = func(viewer *Conn, entity protocol.UnitSyncEntity) bool {
		return viewer == hiddenViewer && entity != nil && entity.ID() == 7001
	}

	freshPackets, freshHidden, err := srv.buildEntitySnapshotPacketsForConn(hiddenViewer)
	if err != nil {
		t.Fatalf("fresh buildEntitySnapshotPacketsForConn failed: %v", err)
	}
	cachedPackets, cachedHidden, err := srv.buildEntitySnapshotPacketsForConnCached(hiddenViewer)
	if err != nil {
		t.Fatalf("cached buildEntitySnapshotPacketsForConnCached failed: %v", err)
	}
	visiblePackets, visibleHidden, err := srv.buildEntitySnapshotPacketsForConnCached(visibleViewer)
	if err != nil {
		t.Fatalf("visible buildEntitySnapshotPacketsForConnCached failed: %v", err)
	}

	if got := buildCalls.Load(); got != 2 {
		t.Fatalf("expected 2 base builds (fresh + first cache miss), got %d", got)
	}
	if !reflect.DeepEqual(freshHidden, cachedHidden) {
		t.Fatalf("expected cached hidden ids %v to match fresh %v", cachedHidden, freshHidden)
	}
	freshIDs := snapshotPacketEntityIDs(t, srv, freshPackets)
	cachedIDs := snapshotPacketEntityIDs(t, srv, cachedPackets)
	if !reflect.DeepEqual(freshIDs, cachedIDs) {
		t.Fatalf("expected cached ids %v to match fresh %v", cachedIDs, freshIDs)
	}
	if len(visibleHidden) != 0 {
		t.Fatalf("expected visible viewer hidden ids to be empty, got %v", visibleHidden)
	}
	if got := snapshotPacketEntityIDs(t, srv, visiblePackets); !reflect.DeepEqual(got, []int32{7001, 7002}) {
		t.Fatalf("expected visible viewer ids [7001 7002], got %v", got)
	}

	stats := srv.EntitySnapshotCacheStats()
	if stats.Misses != 1 {
		t.Fatalf("expected cache misses=1, got %d", stats.Misses)
	}
	if stats.Hits != 1 {
		t.Fatalf("expected cache hits=1, got %d", stats.Hits)
	}
}

func BenchmarkBuildEntitySnapshotPacketsForConn(b *testing.B) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.entitySnapshotIntervalNs.Store(int64(time.Hour))
	srv.stateSnapshotIntervalNs.Store(int64(time.Hour))

	entities := make([]protocol.UnitSyncEntity, 0, 1024)
	for i := 0; i < 1024; i++ {
		entities = append(entities, &protocol.UnitEntitySync{
			IDValue:      int32(8000 + i),
			ClassIDValue: 30,
			ClassIDSet:   true,
			TypeID:       35,
			TeamID:       byte(i%4 + 1),
			Health:       100,
			X:            float32((i % 64) * 8),
			Y:            float32((i / 64) * 8),
		})
	}
	srv.ExtraEntitySnapshotEntitiesFn = func() ([]protocol.UnitSyncEntity, error) {
		out := make([]protocol.UnitSyncEntity, len(entities))
		copy(out, entities)
		return out, nil
	}

	viewerA := &Conn{id: 1}
	viewerB := &Conn{id: 2}
	srv.EntitySnapshotHiddenFn = func(viewer *Conn, entity protocol.UnitSyncEntity) bool {
		return viewer == viewerA && entity != nil && entity.ID()%5 == 0
	}

	b.Run("fresh", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			viewer := viewerA
			if i&1 == 1 {
				viewer = viewerB
			}
			if _, _, err := srv.buildEntitySnapshotPacketsForConn(viewer); err != nil {
				b.Fatalf("fresh buildEntitySnapshotPacketsForConn failed: %v", err)
			}
		}
	})

	b.Run("cached", func(b *testing.B) {
		b.ReportAllocs()
		srv.clearEntitySnapshotCache()
		if _, _, err := srv.buildEntitySnapshotPacketsForConnCached(viewerA); err != nil {
			b.Fatalf("cached warmup failed: %v", err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			viewer := viewerA
			if i&1 == 1 {
				viewer = viewerB
			}
			if _, _, err := srv.buildEntitySnapshotPacketsForConnCached(viewer); err != nil {
				b.Fatalf("cached buildEntitySnapshotPacketsForConnCached failed: %v", err)
			}
		}
	})
}
