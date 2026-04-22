package world

import "time"

type TraceRuntimeState struct {
	Tick          uint64
	Wave          int32
	WaveTime      float32
	TimeData      int32
	TPS           int8
	ActiveTiles   int
	Entities      int
	Bullets       int
	PendingBuilds int
	PendingBreaks int
}

func (w *World) TraceRuntimeState() TraceRuntimeState {
	if w == nil {
		return TraceRuntimeState{}
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	tps := w.actualTps
	if tps <= 0 {
		tps = w.tps
	}
	entities := 0
	if w.model != nil {
		entities = len(w.model.Entities)
	}
	return TraceRuntimeState{
		Tick:          w.tick,
		Wave:          w.wave,
		WaveTime:      w.waveTime,
		TimeData:      int32(timeSinceStartSeconds(w.start)),
		TPS:           tps,
		ActiveTiles:   len(w.activeTilePositions),
		Entities:      entities,
		Bullets:       len(w.bullets),
		PendingBuilds: len(w.pendingBuilds),
		PendingBreaks: len(w.pendingBreaks),
	}
}

func timeSinceStartSeconds(startTime time.Time) int {
	if startTime.IsZero() {
		return 0
	}
	return int(time.Since(startTime).Seconds())
}
