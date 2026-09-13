package oracle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	PositionTolerance = 0.01
	AngleTolerance    = 0.1
	ScalarTolerance   = 1e-4
)

type Tolerances struct {
	Position float64 `json:"position"`
	Angle    float64 `json:"angle"`
	Scalar   float64 `json:"scalar"`
}

type Scenario struct {
	Name                string   `json:"name"`
	MapPath             string   `json:"mapPath"`
	Seed                int64    `json:"seed"`
	Ticks               int      `json:"ticks"`
	DeltaMS             int      `json:"deltaMs,omitempty"`
	Mode                string   `json:"mode,omitempty"`
	HotReloadTick       int      `json:"hotReloadTick,omitempty"`
	HotReloadMapPath    string   `json:"hotReloadMapPath,omitempty"`
	VanillaProfilesPath string   `json:"vanillaProfilesPath,omitempty"`
	CaptureInitial      bool     `json:"captureInitial,omitempty"`
	Tags                []string `json:"tags,omitempty"`
}

type SlotState struct {
	ID     int     `json:"id"`
	Amount float64 `json:"amount"`
}

type LogicState struct {
	Enabled      bool               `json:"enabled"`
	ControlledBy string             `json:"controlledBy,omitempty"`
	Flags        map[string]bool    `json:"flags,omitempty"`
	Numbers      map[string]float64 `json:"numbers,omitempty"`
}

type TileState struct {
	X                  int         `json:"x"`
	Y                  int         `json:"y"`
	FloorID            int         `json:"floorId"`
	OverlayID          int         `json:"overlayId"`
	BlockID            int         `json:"blockId"`
	TeamID             int         `json:"teamId"`
	Rotation           int         `json:"rotation"`
	BuildHealth        float64     `json:"buildHealth,omitempty"`
	ConstructBlockID   int         `json:"constructBlockId,omitempty"`
	ConstructProgress  float64     `json:"constructProgress,omitempty"`
	ControllerType     string      `json:"controllerType,omitempty"`
	PowerStored        float64     `json:"powerStored,omitempty"`
	PowerBalance       float64     `json:"powerBalance,omitempty"`
	Heat               float64     `json:"heat,omitempty"`
	Reload             float64     `json:"reload,omitempty"`
	Items              []SlotState `json:"items,omitempty"`
	Liquids            []SlotState `json:"liquids,omitempty"`
	PayloadUnitTypeIDs []int       `json:"payloadUnitTypeIds,omitempty"`
	Logic              LogicState  `json:"logic"`
}

type UnitState struct {
	ID             int                `json:"id"`
	TypeID         int                `json:"typeId"`
	TeamID         int                `json:"teamId"`
	X              float64            `json:"x"`
	Y              float64            `json:"y"`
	Rotation       float64            `json:"rotation"`
	VelocityX      float64            `json:"velocityX"`
	VelocityY      float64            `json:"velocityY"`
	Health         float64            `json:"health"`
	Shield         float64            `json:"shield,omitempty"`
	ControllerType string             `json:"controllerType"`
	AIState        map[string]string  `json:"aiState,omitempty"`
	MountReloads   map[string]float64 `json:"mountReloads,omitempty"`
}

type TraceEvent struct {
	Tick      int               `json:"tick"`
	Kind      string            `json:"kind"`
	SubjectID int               `json:"subjectId,omitempty"`
	X         int               `json:"x,omitempty"`
	Y         int               `json:"y,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

type TickTrace struct {
	Index  int          `json:"index"`
	Tiles  []TileState  `json:"tiles,omitempty"`
	Units  []UnitState  `json:"units,omitempty"`
	Events []TraceEvent `json:"events,omitempty"`
}

type Trace struct {
	Producer   string            `json:"producer"`
	Scenario   Scenario          `json:"scenario"`
	Tolerances Tolerances        `json:"tolerances"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Ticks      []TickTrace       `json:"ticks"`
}

func NewTrace(producer string, scenario Scenario) Trace {
	return Trace{
		Producer: producer,
		Scenario: scenario,
		Tolerances: Tolerances{
			Position: PositionTolerance,
			Angle:    AngleTolerance,
			Scalar:   ScalarTolerance,
		},
		Metadata: map[string]string{},
		Ticks:    []TickTrace{},
	}
}

func (s Scenario) Normalized() Scenario {
	if s.Ticks < 0 {
		s.Ticks = 0
	}
	if s.DeltaMS <= 0 {
		s.DeltaMS = int((time.Second / 60) / time.Millisecond)
	}
	return s
}

func (s Scenario) DeltaDuration() time.Duration {
	s = s.Normalized()
	return time.Duration(s.DeltaMS) * time.Millisecond
}

func WriteTrace(path string, trace Trace) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func ReadTrace(path string) (Trace, error) {
	var trace Trace
	raw, err := os.ReadFile(path)
	if err != nil {
		return trace, err
	}
	err = json.Unmarshal(raw, &trace)
	return trace, err
}
