package world

// ModelSnapshot returns a deep-copied world model for read-only export paths
// (e.g. world stream handshake) without holding world locks for a long time.
func (w *World) ModelSnapshot() *WorldModel {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return cloneWorldModel(w.model)
}

func cloneWorldModel(src *WorldModel) *WorldModel {
	if src == nil {
		return nil
	}
	dst := &WorldModel{
		Width:        src.Width,
		Height:       src.Height,
		NextEntityID: src.NextEntityID,
		MSAVVersion:  src.MSAVVersion,
		EntitiesRev:  src.EntitiesRev,
	}

	if len(src.Tiles) > 0 {
		dst.Tiles = make([]Tile, len(src.Tiles))
		for i := range src.Tiles {
			t := src.Tiles[i]
			dst.Tiles[i] = t
			if t.Build != nil {
				b := *t.Build
				if len(t.Build.Items) > 0 {
					b.Items = append([]ItemStack(nil), t.Build.Items...)
				}
				if len(t.Build.Liquids) > 0 {
					b.Liquids = append([]LiquidStack(nil), t.Build.Liquids...)
				}
				if len(t.Build.Config) > 0 {
					b.Config = append([]byte(nil), t.Build.Config...)
				}
				if len(t.Build.Payload) > 0 {
					b.Payload = append([]byte(nil), t.Build.Payload...)
				}
				dst.Tiles[i].Build = &b
			}
		}
	}

	if len(src.Units) > 0 {
		dst.Units = make(map[int32]*Unit, len(src.Units))
		for id, u := range src.Units {
			if u == nil {
				continue
			}
			uc := *u
			dst.Units[id] = &uc
		}
	} else {
		dst.Units = make(map[int32]*Unit)
	}

	if len(src.Entities) > 0 {
		dst.Entities = make([]RawEntity, len(src.Entities))
		for i := range src.Entities {
			e := src.Entities[i]
			if len(e.Payload) > 0 {
				e.Payload = append([]byte(nil), e.Payload...)
			}
			dst.Entities[i] = e
		}
	} else {
		dst.Entities = make([]RawEntity, 0)
	}

	if len(src.Tags) > 0 {
		dst.Tags = make(map[string]string, len(src.Tags))
		for k, v := range src.Tags {
			dst.Tags[k] = v
		}
	} else {
		dst.Tags = make(map[string]string)
	}
	if len(src.Content) > 0 {
		dst.Content = append([]byte(nil), src.Content...)
	}
	if len(src.Patches) > 0 {
		dst.Patches = append([]byte(nil), src.Patches...)
	}
	if len(src.RawMap) > 0 {
		dst.RawMap = append([]byte(nil), src.RawMap...)
	}
	if len(src.RawEntities) > 0 {
		dst.RawEntities = append([]byte(nil), src.RawEntities...)
	}
	if len(src.Markers) > 0 {
		dst.Markers = append([]byte(nil), src.Markers...)
	}
	if len(src.Custom) > 0 {
		dst.Custom = append([]byte(nil), src.Custom...)
	}
	if len(src.BlockNames) > 0 {
		dst.BlockNames = make(map[int16]string, len(src.BlockNames))
		for k, v := range src.BlockNames {
			dst.BlockNames[k] = v
		}
	} else {
		dst.BlockNames = make(map[int16]string)
	}
	if len(src.UnitNames) > 0 {
		dst.UnitNames = make(map[int16]string, len(src.UnitNames))
		for k, v := range src.UnitNames {
			dst.UnitNames[k] = v
		}
	} else {
		dst.UnitNames = make(map[int16]string)
	}
	return dst
}
