package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

type profilesFile struct {
	Turrets []turretProfile `json:"turrets"`
}

type turretProfile struct {
	Name      string  `json:"name"`
	Range     float32 `json:"range"`
	Damage    float32 `json:"damage"`
	Interval  float32 `json:"interval"`
	TargetAir bool    `json:"target_air"`
	FireMode  string  `json:"fire_mode"`
}

func main() {
	reg := protocol.NewContentRegistry()
	ids, err := vanilla.LoadContentIDs(filepath.Join("data", "vanilla", "content_ids.json"))
	if err != nil {
		panic(err)
	}
	vanilla.ApplyContentIDs(reg, ids)
	model, err := worldstream.LoadWorldModelFromMSAV(filepath.Join("assets", "worlds", "file.msav"), reg)
	if err != nil {
		panic(err)
	}
	raw, err := os.ReadFile(filepath.Join("data", "vanilla", "profiles.json"))
	if err != nil {
		panic(err)
	}
	var pf profilesFile
	if err := json.Unmarshal(raw, &pf); err != nil {
		panic(err)
	}
	turrets := map[string]turretProfile{}
	for _, p := range pf.Turrets {
		if p.TargetAir && p.Range > 0 {
			turrets[p.Name] = p
		}
	}
	targets := map[int32]world.RawEntity{}
	for _, e := range model.Entities {
		if e.ID == 1197519 || e.ID == 1197523 {
			targets[e.ID] = e
		}
	}
	for id, e := range targets {
		fmt.Printf("target id=%d team=%d pos=(%.3f,%.3f) shield=%.6f\n", id, e.Team, e.X, e.Y, e.Shield)
		for i := range model.Tiles {
			tile := &model.Tiles[i]
			if tile.Build == nil || tile.Team == e.Team {
				continue
			}
			name := model.BlockNames[int16(tile.Block)]
			prof, ok := turrets[name]
			if !ok {
				continue
			}
			tx, ty := float32(tile.X*8+4), float32(tile.Y*8+4)
			dx, dy := e.X-tx, e.Y-ty
			dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
			if dist > prof.Range+40 {
				continue
			}
			fmt.Printf("  turret %s team=%d tile=(%d,%d) dist=%.3f range=%.1f damage=%.3f interval=%.6f mode=%s hp=%.1f tail=%x\n",
				name, tile.Team, tile.X, tile.Y, dist, prof.Range, prof.Damage, prof.Interval, prof.FireMode, tile.Build.Health, tile.Build.MapSyncTail)
		}
	}
}
