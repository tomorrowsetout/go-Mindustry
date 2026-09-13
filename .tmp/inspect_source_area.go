package main

import (
  "fmt"
  "path/filepath"
  "reflect"
  "time"
  "mdt-server/internal/protocol"
  "mdt-server/internal/vanilla"
  "mdt-server/internal/world"
  "mdt-server/internal/worldstream"
)
func inspectRuntime(w *world.World, label string) {
 v:=reflect.ValueOf(w).Elem()
 width:=w.Model().Width
 srcPos:=int32(492*width+318)
 projPos:=int32(490*width+317)
 corePos:=int32(494*width+317)
 sources:=v.FieldByName("sandboxItemSourceTiles")
 accum:=v.FieldByName("itemSourceAccum")
 dumpIdx:=v.FieldByName("blockDumpIndex")
 boosts:=v.FieldByName("buildingBoostStates")
 overdrives:=v.FieldByName("overdriveProjectorStates")
 count:=0
 for i:=0;i<sources.Len();i++{if int32(sources.Index(i).Int())==srcPos{count++}}
 fmt.Printf("%s runtime srcPos=%d sourceIndexCount=%d accumValid=%v", label, srcPos, count, accum.MapIndex(reflect.ValueOf(srcPos)).IsValid())
 if a:=accum.MapIndex(reflect.ValueOf(srcPos)); a.IsValid(){fmt.Printf(" accum=%.3f", float32(a.Float()))}
 if d:=dumpIdx.MapIndex(reflect.ValueOf(srcPos)); d.IsValid(){fmt.Printf(" dumpIdx=%d", d.Int())}
 for _,p:=range []int32{srcPos,projPos,corePos}{if b:=boosts.MapIndex(reflect.ValueOf(p)); b.IsValid(){fmt.Printf(" boost[%d]={scale=%.3f dur=%.3f}",p,float32(b.FieldByName("TimeScale").Float()),float32(b.FieldByName("Duration").Float()))}}
 if st:=overdrives.MapIndex(reflect.ValueOf(projPos)); st.IsValid() && !st.IsNil(){e:=st.Elem(); fmt.Printf(" overdriveCharge=%.3f heat=%.3f phase=%.3f use=%.3f",float32(e.FieldByName("Charge").Float()),float32(e.FieldByName("Heat").Float()),float32(e.FieldByName("PhaseHeat").Float()),float32(e.FieldByName("UseProgress").Float()))}
 fmt.Println()
}
func main(){
 reg:=protocol.NewContentRegistry(); ids,_:=vanilla.LoadContentIDs(filepath.Join("data","vanilla","content_ids.json")); vanilla.ApplyContentIDs(reg,ids)
 m,_:=worldstream.LoadWorldModelFromMSAV(filepath.Join("assets","worlds","file.msav"),reg)
 w:=world.New(world.Config{TPS:60}); w.SetModel(m); _=w.LoadVanillaProfiles(filepath.Join("data","vanilla","profiles.json"))
 for y:=0; y<m.Height; y++ { for x:=0; x<m.Width; x++ { t,_:=m.TileAt(x,y); if t!=nil && t.Build!=nil && m.BlockNames[int16(t.Block)]=="item-source" && len(t.Build.MapSyncTail)>=2 && t.Build.MapSyncTail[len(t.Build.MapSyncTail)-1]==0x0b { fmt.Printf("phase-source %d,%d center=(%d,%d) rot=%d tail=%x\n",x,y,t.Build.X,t.Build.Y,t.Rotation,t.Build.MapSyncTail) } } }
 pts:=[][2]int{}
 for y:=490; y<=497; y++ { for x:=314; x<=321; x++ { pts=append(pts,[2]int{x,y}) } }
 dump:=func(label string){fmt.Println(label); clone:=w.CloneModelForWorldStream(); for _,pt:=range pts{t,_:=clone.TileAt(pt[0],pt[1]); if t.Block==0 && t.Build==nil {continue}; fmt.Printf("%d,%d block=%d name=%s team=%d rot=%d",pt[0],pt[1],t.Block,clone.BlockNames[int16(t.Block)],t.Team,t.Rotation); if t.Build!=nil{fmt.Printf(" center=(%d,%d) buildName=%s items=%v liquids=%v config=%x tail=%x",t.Build.X,t.Build.Y,clone.BlockNames[int16(t.Build.Block)],t.Build.Items,t.Build.Liquids,t.Build.Config,t.Build.MapSyncTail)}; fmt.Println()}}
 inspectRuntime(w,"initial"); dump("initial"); w.Step(time.Second/60); inspectRuntime(w,"after"); dump("after")
}
