package main
import (
  "fmt"
  "mdt-server/internal/worldstream"
)
func main(){
 m,err:=worldstream.LoadWorldModelFromMSAV("assets/worlds/23315.msav", nil)
 if err!=nil { panic(err)}
 fmt.Println(m.Tags["rules"])
}
