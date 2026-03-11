package logic

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// MlogVar mirrors Mindustry logic LVar with numeric/object dual storage.
type MlogVar struct {
	Name     string
	ID       int
	IsObj    bool
	Constant bool
	ObjVal   any
	NumVal   float64
	SyncTime int64
}

func newMlogVar(name string, id int, constant bool) *MlogVar {
	return &MlogVar{Name: name, ID: id, Constant: constant, IsObj: true}
}

func (v *MlogVar) Obj() any {
	if v == nil || !v.IsObj {
		return nil
	}
	return v.ObjVal
}

func (v *MlogVar) Num() float64 {
	if v == nil {
		return 0
	}
	if v.IsObj {
		if v.ObjVal != nil {
			return 1
		}
		return 0
	}
	if invalidNum(v.NumVal) {
		return 0
	}
	return v.NumVal
}

func (v *MlogVar) NumOrNaN() float64 {
	if v == nil {
		return 0
	}
	if v.IsObj {
		if v.ObjVal != nil {
			return 1
		}
		return math.NaN()
	}
	if invalidNum(v.NumVal) {
		return 0
	}
	return v.NumVal
}

func (v *MlogVar) Bool() bool {
	if v == nil {
		return false
	}
	if v.IsObj {
		return v.ObjVal != nil
	}
	return math.Abs(v.Num()) >= 0.00001
}

func (v *MlogVar) SetNum(val float64) {
	if v == nil || v.Constant {
		return
	}
	if invalidNum(val) {
		v.ObjVal = nil
		v.IsObj = true
		return
	}
	v.NumVal = val
	v.ObjVal = nil
	v.IsObj = false
}

func (v *MlogVar) SetObj(val any) {
	if v == nil || v.Constant {
		return
	}
	v.ObjVal = val
	v.IsObj = true
}

func (v *MlogVar) SetConst(val any) {
	if v == nil {
		return
	}
	switch t := val.(type) {
	case float64:
		v.NumVal = t
		v.ObjVal = nil
		v.IsObj = false
	case float32:
		v.NumVal = float64(t)
		v.ObjVal = nil
		v.IsObj = false
	case int:
		v.NumVal = float64(t)
		v.ObjVal = nil
		v.IsObj = false
	case int8:
		v.NumVal = float64(t)
		v.ObjVal = nil
		v.IsObj = false
	case int16:
		v.NumVal = float64(t)
		v.ObjVal = nil
		v.IsObj = false
	case int32:
		v.NumVal = float64(t)
		v.ObjVal = nil
		v.IsObj = false
	case int64:
		v.NumVal = float64(t)
		v.ObjVal = nil
		v.IsObj = false
	case uint:
		v.NumVal = float64(t)
		v.ObjVal = nil
		v.IsObj = false
	case uint8:
		v.NumVal = float64(t)
		v.ObjVal = nil
		v.IsObj = false
	case uint16:
		v.NumVal = float64(t)
		v.ObjVal = nil
		v.IsObj = false
	case uint32:
		v.NumVal = float64(t)
		v.ObjVal = nil
		v.IsObj = false
	case uint64:
		v.NumVal = float64(t)
		v.ObjVal = nil
		v.IsObj = false
	default:
		v.ObjVal = val
		v.IsObj = true
	}
}

func (v *MlogVar) Set(other *MlogVar) {
	if v == nil || other == nil || v.Constant {
		return
	}
	v.IsObj = other.IsObj
	if v.IsObj {
		v.ObjVal = other.ObjVal
	} else {
		v.NumVal = other.NumVal
	}
}

func invalidNum(v float64) bool {
	return math.IsNaN(v) || math.IsInf(v, 0)
}

// MlogCell is a simple memory cell used by read/write instructions.
type MlogCell struct {
	Data []float64
}

func NewMlogCell(size int) *MlogCell {
	if size <= 0 {
		size = 64
	}
	return &MlogCell{Data: make([]float64, size)}
}

func (c *MlogCell) Get(idx int) float64 {
	if c == nil || idx < 0 || idx >= len(c.Data) {
		return 0
	}
	return c.Data[idx]
}

func (c *MlogCell) Set(idx int, v float64) {
	if c == nil || idx < 0 {
		return
	}
	if idx >= len(c.Data) {
		ndata := make([]float64, idx+1)
		copy(ndata, c.Data)
		c.Data = ndata
	}
	c.Data[idx] = v
}

// MlogHost provides external world interaction for logic instructions.
type MlogHost interface {
	Now() time.Time
	Sensor(target any, sensor string) (any, bool)
	Control(target any, action string, a, b, c, d *MlogVar) bool
	UControl(target any, action string, a, b, c, d *MlogVar) bool
	Radar(target any, filter string, sort string, count int) (any, bool)
	URadar(target any, filter string, sort string, count int) (any, bool)
	Locate(target any, locate string, flag string, enemy bool, ore any) (found bool, x, y float64, obj any)
	GetLink(index int) (any, bool)
	ReadCell(cell any, addr int) float64
	WriteCell(cell any, addr int, value float64)
	Lookup(kind string, id int) (any, bool)
	GetBlock(x, y int, layer string) (any, bool)
	SetBlock(x, y int, block any, team int, rotation int) bool
	Spawn(unit any, x, y float64, team int, rot float64) (any, bool)
	Apply(status any, target any, duration float64) bool
	Explosion(x, y float64, damage, radius float64, team int) bool
	Status(target any) (health, maxHealth float64, team int, ok bool)
	Fetch(kind string, team any, extra any, idx int) (any, bool)
	SyncVar(id int, value any) bool
	PrintFlush(target any, text string) bool
	DrawFlush(target any, buffer []uint64) bool
	ClientData(channel string, value any, reliable bool) bool
}

// MlogInstruction executes a single instruction.
type MlogInstruction interface {
	Run(exec *MlogExecutor)
}

// MlogProgram holds assembled instructions/vars.
type MlogProgram struct {
	Instructions []MlogInstruction
	Vars         []*MlogVar
	VarByName    map[string]*MlogVar
	Counter      *MlogVar
	Unit         *MlogVar
	This         *MlogVar
	Ipt          *MlogVar
}

// MlogExecutor executes Mindustry logic instructions.
type MlogExecutor struct {
	Program *MlogProgram
	Host    MlogHost

	Vars    []*MlogVar
	Counter *MlogVar
	Unit    *MlogVar
	This    *MlogVar
	Ipt     *MlogVar

	Yield bool

	TextBuffer     strings.Builder
	GraphicsBuffer []uint64

	waitUntil time.Time
	maxSteps  int
	rate      float64
}

func NewMlogExecutor(program *MlogProgram, host MlogHost) *MlogExecutor {
	exec := &MlogExecutor{
		Program: program,
		Host:    host,
		rate:    1,
	}
	if program != nil {
		exec.Vars = program.Vars
		exec.Counter = program.Counter
		exec.Unit = program.Unit
		exec.This = program.This
		exec.Ipt = program.Ipt
	}
	return exec
}

func (e *MlogExecutor) RunOnce() {
	if e == nil || e.Program == nil || len(e.Program.Instructions) == 0 || e.Counter == nil {
		return
	}
	if !e.waitUntil.IsZero() {
		now := time.Now()
		if e.Host != nil {
			now = e.Host.Now()
		}
		if now.Before(e.waitUntil) {
			return
		}
		e.waitUntil = time.Time{}
	}
	idx := int(e.Counter.Num())
	if idx < 0 || idx >= len(e.Program.Instructions) {
		e.Counter.SetNum(0)
		idx = 0
	}
	e.Counter.IsObj = false
	e.Counter.NumVal = float64(idx + 1)
	e.Program.Instructions[idx].Run(e)
}

func (e *MlogExecutor) RunSteps(maxSteps int) {
	if maxSteps <= 0 {
		maxSteps = 1000
	}
	e.maxSteps = maxSteps
	e.Yield = false
	for i := 0; i < maxSteps; i++ {
		if e.Yield {
			return
		}
		e.RunOnce()
	}
}

// MlogAssembler parses Mindustry logic into instructions.
type MlogAssembler struct {
	Vars      []*MlogVar
	VarByName map[string]*MlogVar
	Privileged bool
}

func NewMlogAssembler() *MlogAssembler {
	a := &MlogAssembler{
		VarByName: map[string]*MlogVar{},
	}
	a.PutVar("@counter").IsObj = false
	a.PutConst("@unit", nil)
	a.PutConst("@this", nil)
	return a
}

func (a *MlogAssembler) PutVar(name string) *MlogVar {
	if v, ok := a.VarByName[name]; ok {
		return v
	}
	v := newMlogVar(name, len(a.Vars), false)
	a.Vars = append(a.Vars, v)
	a.VarByName[name] = v
	return v
}

func (a *MlogAssembler) PutConst(name string, val any) *MlogVar {
	v := a.PutVar(name)
	v.Constant = true
	v.SetConst(val)
	return v
}

func (a *MlogAssembler) Var(symbol string) *MlogVar {
	if v, ok := a.VarByName[symbol]; ok {
		return v
	}
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return a.PutVar(symbol)
	}
	if strings.HasPrefix(symbol, "\"") && strings.HasSuffix(symbol, "\"") && len(symbol) >= 2 {
		raw := strings.ReplaceAll(symbol[1:len(symbol)-1], "\\n", "\n")
		return a.PutConst("___"+symbol, raw)
	}
	symbol = strings.ReplaceAll(symbol, " ", "_")
	value, ok := parseMlogNumber(symbol)
	if ok {
		return a.PutConst("___"+symbol, value)
	}
	return a.PutVar(symbol)
}

func (a *MlogAssembler) Assemble(code string) (*MlogProgram, error) {
	lines := splitMlogLines(code)
	instructions := make([]MlogInstruction, 0, len(lines))
	for _, line := range lines {
		tokens := tokenizeMlogLine(line)
		if len(tokens) == 0 {
			continue
		}
		inst, err := a.parseInstruction(tokens)
		if err != nil {
			return nil, err
		}
		if inst != nil {
			instructions = append(instructions, inst)
		}
	}
	prog := &MlogProgram{
		Instructions: instructions,
		Vars:         a.Vars,
		VarByName:    a.VarByName,
		Counter:      a.VarByName["@counter"],
		Unit:         a.VarByName["@unit"],
		This:         a.VarByName["@this"],
		Ipt:          a.PutConst("@ipt", 0),
	}
	return prog, nil
}

func (p *MlogProgram) EnsureVar(name string) *MlogVar {
	if p == nil {
		return nil
	}
	if p.VarByName == nil {
		p.VarByName = map[string]*MlogVar{}
	}
	if v, ok := p.VarByName[name]; ok {
		return v
	}
	v := newMlogVar(name, len(p.Vars), false)
	p.Vars = append(p.Vars, v)
	p.VarByName[name] = v
	return v
}

func (a *MlogAssembler) parseInstruction(tokens []string) (MlogInstruction, error) {
	switch strings.ToLower(tokens[0]) {
	case "set":
		if len(tokens) < 3 {
			return nil, errors.New("set requires 2 args")
		}
		return &MlogSetI{Out: a.Var(tokens[1]), Value: a.Var(tokens[2])}, nil
	case "op":
		if len(tokens) < 5 {
			return nil, errors.New("op requires 4 args")
		}
		return &MlogOpI{Op: tokens[1], Out: a.Var(tokens[2]), A: a.Var(tokens[3]), B: a.Var(tokens[4])}, nil
	case "jump":
		if len(tokens) < 5 {
			return nil, errors.New("jump requires 4 args")
		}
		index, err := strconv.Atoi(tokens[1])
		if err != nil {
			return nil, fmt.Errorf("jump index: %w", err)
		}
		return &MlogJumpI{Index: index, Cond: tokens[2], A: a.Var(tokens[3]), B: a.Var(tokens[4])}, nil
	case "read":
		if len(tokens) < 4 {
			return nil, errors.New("read requires 3 args")
		}
		return &MlogReadI{Cell: a.Var(tokens[2]), Address: a.Var(tokens[3]), Out: a.Var(tokens[1])}, nil
	case "write":
		if len(tokens) < 4 {
			return nil, errors.New("write requires 3 args")
		}
		return &MlogWriteI{Cell: a.Var(tokens[2]), Address: a.Var(tokens[3]), Value: a.Var(tokens[1])}, nil
	case "print":
		if len(tokens) < 2 {
			return nil, errors.New("print requires 1 arg")
		}
		return &MlogPrintI{Value: a.Var(tokens[1])}, nil
	case "printflush":
		if len(tokens) < 2 {
			return nil, errors.New("printflush requires 1 arg")
		}
		return &MlogPrintFlushI{Target: a.Var(tokens[1])}, nil
	case "draw":
		if len(tokens) < 2 {
			return nil, errors.New("draw requires args")
		}
		args := make([]*MlogVar, 0, 6)
		for i := 2; i < len(tokens) && len(args) < 6; i++ {
			args = append(args, a.Var(tokens[i]))
		}
		return &MlogDrawI{Cmd: tokens[1], Args: args}, nil
	case "drawflush":
		if len(tokens) < 2 {
			return nil, errors.New("drawflush requires 1 arg")
		}
		return &MlogDrawFlushI{Target: a.Var(tokens[1])}, nil
	case "clientdata":
		if len(tokens) < 4 {
			return nil, errors.New("clientdata requires 3 args")
		}
		return &MlogClientDataI{Channel: a.Var(tokens[1]), Value: a.Var(tokens[2]), Reliable: a.Var(tokens[3])}, nil
	case "wait":
		if len(tokens) < 2 {
			return nil, errors.New("wait requires 1 arg")
		}
		return &MlogWaitI{Duration: a.Var(tokens[1])}, nil
	case "stop":
		return &MlogStopI{}, nil
	case "end":
		return &MlogEndI{}, nil
	case "noop":
		return &MlogNoopI{}, nil
	case "getlink":
		if len(tokens) < 3 {
			return nil, errors.New("getlink requires 2 args")
		}
		return &MlogGetLinkI{Out: a.Var(tokens[1]), Index: a.Var(tokens[2])}, nil
	case "packcolor":
		if len(tokens) < 6 {
			return nil, errors.New("packcolor requires 5 args")
		}
		return &MlogPackColorI{Out: a.Var(tokens[1]), R: a.Var(tokens[2]), G: a.Var(tokens[3]), B: a.Var(tokens[4]), A: a.Var(tokens[5])}, nil
	case "setrate":
		if len(tokens) < 2 {
			return nil, errors.New("setrate requires 1 arg")
		}
		return &MlogSetRateI{Rate: a.Var(tokens[1])}, nil
	case "lookup":
		if len(tokens) < 4 {
			return nil, errors.New("lookup requires 3 args")
		}
		return &MlogLookupI{Out: a.Var(tokens[1]), Kind: tokens[2], Index: a.Var(tokens[3])}, nil
	case "getblock":
		if len(tokens) < 5 {
			return nil, errors.New("getblock requires 4 args")
		}
		return &MlogGetBlockI{Out: a.Var(tokens[1]), X: a.Var(tokens[2]), Y: a.Var(tokens[3]), Layer: tokens[4]}, nil
	case "setblock":
		if len(tokens) < 6 {
			return nil, errors.New("setblock requires 5 args")
		}
		return &MlogSetBlockI{Block: a.Var(tokens[1]), X: a.Var(tokens[2]), Y: a.Var(tokens[3]), Team: a.Var(tokens[4]), Rotation: a.Var(tokens[5])}, nil
	case "sensor":
		if len(tokens) < 4 {
			return nil, errors.New("sensor requires 3 args")
		}
		return &MlogSensorI{Out: a.Var(tokens[1]), Target: a.Var(tokens[2]), Sensor: tokens[3]}, nil
	case "control":
		if len(tokens) < 6 {
			return nil, errors.New("control requires 5 args")
		}
		return &MlogControlI{Target: a.Var(tokens[1]), Action: tokens[2], A: a.Var(tokens[3]), B: a.Var(tokens[4]), C: a.Var(tokens[5]), D: a.optionalVar(tokens, 6)}, nil
	case "ucontrol":
		if len(tokens) < 5 {
			return nil, errors.New("ucontrol requires 4 args")
		}
		return &MlogUControlI{Action: tokens[1], A: a.Var(tokens[2]), B: a.Var(tokens[3]), C: a.Var(tokens[4]), D: a.optionalVar(tokens, 5)}, nil
	case "unitbind":
		if len(tokens) < 2 {
			return nil, errors.New("unitbind requires 1 arg")
		}
		return &MlogUnitBindI{Type: a.Var(tokens[1])}, nil
	case "uradar":
		if len(tokens) < 7 {
			return nil, errors.New("uradar requires 6 args")
		}
		return &MlogURadarI{Filter: tokens[1], Sort: tokens[2], Team: a.Var(tokens[3]), Unit: a.Var(tokens[4]), SortMode: tokens[5], Out: a.Var(tokens[6])}, nil
	case "radar":
		if len(tokens) < 7 {
			return nil, errors.New("radar requires 6 args")
		}
		return &MlogRadarI{Filter: tokens[1], Sort: tokens[2], Team: a.Var(tokens[3]), Block: a.Var(tokens[4]), SortMode: tokens[5], Out: a.Var(tokens[6])}, nil
	case "ulocate":
		if len(tokens) < 8 {
			return nil, errors.New("ulocate requires 7 args")
		}
		return &MlogULocateI{Locate: tokens[1], Flag: tokens[2], Enemy: a.Var(tokens[3]), Ore: a.Var(tokens[4]), OutX: a.Var(tokens[5]), OutY: a.Var(tokens[6]), OutFound: a.Var(tokens[7])}, nil
	case "fetch":
		if len(tokens) < 4 {
			return nil, errors.New("fetch requires 3+ args")
		}
		if len(tokens) >= 6 {
			return &MlogFetchI{Out: a.Var(tokens[1]), Kind: tokens[2], Team: a.Var(tokens[3]), Extra: a.Var(tokens[4]), Index: a.Var(tokens[5])}, nil
		}
		return &MlogFetchI{Out: a.Var(tokens[1]), Kind: tokens[2], Index: a.Var(tokens[3])}, nil
	case "sync":
		if len(tokens) < 3 {
			return nil, errors.New("sync requires 2 args")
		}
		return &MlogSyncI{Name: tokens[1], Value: a.Var(tokens[2])}, nil
	default:
		return &MlogNoopI{}, nil
	}
}

func (a *MlogAssembler) optionalVar(tokens []string, idx int) *MlogVar {
	if idx < 0 || idx >= len(tokens) {
		return a.Var("0")
	}
	return a.Var(tokens[idx])
}

func splitMlogLines(code string) []string {
	lines := strings.Split(code, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func tokenizeMlogLine(line string) []string {
	if line == "" {
		return nil
	}
	var out []string
	var sb strings.Builder
	inQuote := false
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if escaped {
			sb.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			inQuote = !inQuote
			sb.WriteByte(ch)
			continue
		}
		if !inQuote && (ch == ' ' || ch == '\t') {
			if sb.Len() > 0 {
				out = append(out, sb.String())
				sb.Reset()
			}
			continue
		}
		sb.WriteByte(ch)
	}
	if sb.Len() > 0 {
		out = append(out, sb.String())
	}
	return out
}

func parseMlogNumber(symbol string) (float64, bool) {
	if symbol == "" {
		return 0, false
	}
	if strings.HasPrefix(symbol, "0b") {
		if v, err := strconv.ParseInt(symbol[2:], 2, 64); err == nil {
			return float64(v), true
		}
		return 0, false
	}
	if strings.HasPrefix(symbol, "+0b") {
		if v, err := strconv.ParseInt(symbol[3:], 2, 64); err == nil {
			return float64(v), true
		}
		return 0, false
	}
	if strings.HasPrefix(symbol, "-0b") {
		if v, err := strconv.ParseInt(symbol[3:], 2, 64); err == nil {
			return -float64(v), true
		}
		return 0, false
	}
	if strings.HasPrefix(symbol, "0x") {
		if v, err := strconv.ParseInt(symbol[2:], 16, 64); err == nil {
			return float64(v), true
		}
		return 0, false
	}
	if strings.HasPrefix(symbol, "+0x") {
		if v, err := strconv.ParseInt(symbol[3:], 16, 64); err == nil {
			return float64(v), true
		}
		return 0, false
	}
	if strings.HasPrefix(symbol, "-0x") {
		if v, err := strconv.ParseInt(symbol[3:], 16, 64); err == nil {
			return -float64(v), true
		}
		return 0, false
	}
	if strings.HasPrefix(symbol, "%") {
		if v, ok := parseMlogColor(symbol); ok {
			return v, true
		}
	}
	if v, err := strconv.ParseFloat(symbol, 64); err == nil {
		if math.IsInf(v, 0) {
			return 0, true
		}
		return v, true
	}
	return 0, false
}

func parseMlogColor(symbol string) (float64, bool) {
	s := strings.TrimPrefix(symbol, "%")
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		// named colors are not resolved here
		return 0, false
	}
	if len(s) != 6 && len(s) != 8 {
		return 0, false
	}
	if len(s) == 6 {
		s += "ff"
	}
	if v, err := strconv.ParseUint(s, 16, 32); err == nil {
		r := (v >> 24) & 0xFF
		g := (v >> 16) & 0xFF
		b := (v >> 8) & 0xFF
		a := v & 0xFF
		packed := uint32(r) | uint32(g)<<8 | uint32(b)<<16 | uint32(a)<<24
		return float64(math.Float32frombits(packed)), true
	}
	return 0, false
}
