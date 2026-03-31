package main

import (
  "fmt"
  "mdt-server/internal/worldstream"
)

func main(){
  model, err := worldstream.LoadWorldModelFromMSAV(`assets\\worlds\\maps\\serpulo\\hidden\\103.msav`, nil)
  if err != nil { panic(err) }
  cx, cy := 239, 343
  for y := cy-8; y <= cy+8; y++ {
    for x := cx-8; x <= cx+8; x++ {
      t, err := model.TileAt(x, y)
      if err != nil || t == nil || t.Build == nil || t.Build.Health <= 0 {
        continue
      }
      fmt.Printf("tile=(%d,%d) block=%d team=%d rot=%d\n", x, y, t.Block, t.Build.Team, t.Build.Rotation)
    }
  }
}
