package main

import (
	"fmt"

	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/worldstream"
)

func main() {
	ids, err := vanilla.LoadContentIDs("data/vanilla/content_ids.json")
	if err != nil {
		panic(err)
	}
	reg := protocol.NewContentRegistry()
	vanilla.ApplyContentIDs(reg, ids)
	model, err := worldstream.LoadWorldModelFromMSAV("assets/worlds/file.msav", reg)
	if err != nil {
		panic(err)
	}
	points := [][2]int{{383, 482}, {387, 482}, {383, 487}, {387, 487}}
	for _, pt := range points {
		pos := pt[1]*model.Width + pt[0]
		t := model.Tiles[pos]
		fmt.Printf("%d,%d block=%d buildCenter=%d,%d rev=%d tail=%x", pt[0], pt[1], t.Block, t.Build.X, t.Build.Y, t.Build.MapSyncRevision, t.Build.MapSyncTail)
		if len(t.Build.MapSyncTail) >= 8 {
			r := protocol.NewReader(t.Build.MapSyncTail)
			reload, _ := r.ReadFloat32()
			rot, _ := r.ReadFloat32()
			fmt.Printf(" reload=%f rot=%f", reload, rot)
		}
		fmt.Println()
	}
}
