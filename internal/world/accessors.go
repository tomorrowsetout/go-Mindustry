package world

// Accessors exposed for the plugin bridge (GameAPI) and status
// endpoints. They read under the world lock and never mutate state.

// Wave returns the current wave index.
func (w *World) Wave() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return int(w.wave)
}

// GameTick returns the simulation tick counter.
func (w *World) GameTick() uint64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.tick
}

// TPS returns the configured ticks-per-second target.
func (w *World) TPS() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return int(w.tps)
}
