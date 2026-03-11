package logic

import (
	"fmt"
	"math"
	"strings"
	"time"

	"mdt-server/internal/protocol"
)

type MlogSetI struct {
	Out   *MlogVar
	Value *MlogVar
}

func (i *MlogSetI) Run(exec *MlogExecutor) {
	if i == nil || exec == nil || i.Out == nil || i.Value == nil {
		return
	}
	i.Out.Set(i.Value)
}

type MlogOpI struct {
	Op  string
	Out *MlogVar
	A   *MlogVar
	B   *MlogVar
}

func (i *MlogOpI) Run(exec *MlogExecutor) {
	if i == nil || i.Out == nil || i.A == nil || i.B == nil {
		return
	}
	op := strings.ToLower(strings.TrimSpace(i.Op))
	a := i.A.NumOrNaN()
	b := i.B.NumOrNaN()
	switch op {
	case "+", "add":
		i.Out.SetNum(a + b)
	case "-", "sub":
		i.Out.SetNum(a - b)
	case "*", "mul":
		i.Out.SetNum(a * b)
	case "/", "div":
		i.Out.SetNum(a / b)
	case "//", "idiv":
		i.Out.SetNum(math.Floor(a / b))
	case "%", "mod":
		i.Out.SetNum(math.Mod(a, b))
	case "%%", "emod":
		i.Out.SetNum(math.Mod(math.Mod(a, b)+b, b))
	case "^", "pow":
		i.Out.SetNum(math.Pow(a, b))
	case "==", "equal":
		i.Out.SetNum(boolToNum(math.Abs(a-b) < 0.000001))
	case "not", "!=":
		i.Out.SetNum(boolToNum(math.Abs(a-b) >= 0.000001))
	case "and", "land":
		i.Out.SetNum(boolToNum(a != 0 && b != 0))
	case "<", "lessthan":
		i.Out.SetNum(boolToNum(a < b))
	case "<=", "lessthaneq":
		i.Out.SetNum(boolToNum(a <= b))
	case ">", "greaterthan":
		i.Out.SetNum(boolToNum(a > b))
	case ">=", "greaterthaneq":
		i.Out.SetNum(boolToNum(a >= b))
	case "===", "strictequal":
		i.Out.SetNum(boolToNum(i.A.IsObj == i.B.IsObj && ((i.A.IsObj && i.A.ObjVal == i.B.ObjVal) || (!i.A.IsObj && i.A.NumVal == i.B.NumVal))))
	case "<<", "shl":
		i.Out.SetNum(float64(int64(a) << int64(b)))
	case ">>", "shr":
		i.Out.SetNum(float64(int64(a) >> int64(b)))
	case ">>>", "ushr":
		i.Out.SetNum(float64(uint64(int64(a)) >> uint64(b)))
	case "or":
		i.Out.SetNum(float64(int64(a) | int64(b)))
	case "b-and", "andb":
		i.Out.SetNum(float64(int64(a) & int64(b)))
	case "xor":
		i.Out.SetNum(float64(int64(a) ^ int64(b)))
	case "flip", "notb":
		i.Out.SetNum(float64(^int64(a)))
	case "max":
		i.Out.SetNum(math.Max(a, b))
	case "min":
		i.Out.SetNum(math.Min(a, b))
	case "angle":
		i.Out.SetNum(math.Atan2(b, a) * 180 / math.Pi)
	case "anglediff":
		diff := math.Abs(a - b)
		for diff > 180 {
			diff -= 360
		}
		i.Out.SetNum(diff)
	case "len":
		i.Out.SetNum(math.Hypot(a, b))
	case "noise":
		i.Out.SetNum(math.Sin(a*12.9898+b*78.233) * 43758.5453)
	case "abs":
		i.Out.SetNum(math.Abs(a))
	case "sign":
		i.Out.SetNum(math.Copysign(1, a))
	case "log":
		i.Out.SetNum(math.Log(a))
	case "logn":
		i.Out.SetNum(math.Log(a) / math.Log(b))
	case "log10":
		i.Out.SetNum(math.Log10(a))
	case "floor":
		i.Out.SetNum(math.Floor(a))
	case "ceil":
		i.Out.SetNum(math.Ceil(a))
	case "round":
		i.Out.SetNum(math.Round(a))
	case "sqrt":
		i.Out.SetNum(math.Sqrt(a))
	case "rand":
		i.Out.SetNum(randUnit() * a)
	case "sin":
		i.Out.SetNum(math.Sin(a * math.Pi / 180))
	case "cos":
		i.Out.SetNum(math.Cos(a * math.Pi / 180))
	case "tan":
		i.Out.SetNum(math.Tan(a * math.Pi / 180))
	case "asin":
		i.Out.SetNum(math.Asin(a) * 180 / math.Pi)
	case "acos":
		i.Out.SetNum(math.Acos(a) * 180 / math.Pi)
	case "atan":
		i.Out.SetNum(math.Atan(a) * 180 / math.Pi)
	default:
		i.Out.SetNum(0)
	}
}

type MlogJumpI struct {
	Index int
	Cond  string
	A     *MlogVar
	B     *MlogVar
}

func (i *MlogJumpI) Run(exec *MlogExecutor) {
	if i == nil || exec == nil || exec.Counter == nil {
		return
	}
	if i.test() {
		exec.Counter.SetNum(float64(i.Index))
	}
}

func (i *MlogJumpI) test() bool {
	if i == nil || i.A == nil || i.B == nil {
		return false
	}
	op := strings.ToLower(strings.TrimSpace(i.Cond))
	switch op {
	case "always":
		return true
	case "==", "equal":
		if i.A.IsObj && i.B.IsObj {
			return i.A.ObjVal == i.B.ObjVal
		}
		return math.Abs(i.A.Num()-i.B.Num()) < 0.000001
	case "not", "!=":
		if i.A.IsObj && i.B.IsObj {
			return i.A.ObjVal != i.B.ObjVal
		}
		return math.Abs(i.A.Num()-i.B.Num()) >= 0.000001
	case "<":
		return i.A.Num() < i.B.Num()
	case "<=":
		return i.A.Num() <= i.B.Num()
	case ">":
		return i.A.Num() > i.B.Num()
	case ">=":
		return i.A.Num() >= i.B.Num()
	case "===":
		return i.A.IsObj == i.B.IsObj && ((i.A.IsObj && i.A.ObjVal == i.B.ObjVal) || (!i.A.IsObj && i.A.NumVal == i.B.NumVal))
	default:
		return false
	}
}

type MlogReadI struct {
	Cell    *MlogVar
	Address *MlogVar
	Out     *MlogVar
}

func (i *MlogReadI) Run(exec *MlogExecutor) {
	if i == nil || i.Out == nil || i.Cell == nil || i.Address == nil {
		return
	}
	cell := i.Cell.ObjVal
	addr := int(i.Address.Num())
	if exec != nil && exec.Host != nil {
		i.Out.SetNum(exec.Host.ReadCell(cell, addr))
		return
	}
	if c, ok := cell.(*MlogCell); ok {
		i.Out.SetNum(c.Get(addr))
	}
}

type MlogWriteI struct {
	Cell    *MlogVar
	Address *MlogVar
	Value   *MlogVar
}

func (i *MlogWriteI) Run(exec *MlogExecutor) {
	if i == nil || i.Cell == nil || i.Address == nil || i.Value == nil {
		return
	}
	cell := i.Cell.ObjVal
	addr := int(i.Address.Num())
	val := i.Value.Num()
	if exec != nil && exec.Host != nil {
		exec.Host.WriteCell(cell, addr, val)
		return
	}
	if c, ok := cell.(*MlogCell); ok {
		c.Set(addr, val)
	}
}

type MlogPrintI struct {
	Value *MlogVar
}

func (i *MlogPrintI) Run(exec *MlogExecutor) {
	if i == nil || exec == nil || i.Value == nil {
		return
	}
	if i.Value.IsObj {
		exec.TextBuffer.WriteString(formatMlogObj(i.Value.ObjVal))
		return
	}
	exec.TextBuffer.WriteString(formatMlogNum(i.Value.Num()))
}

type MlogPrintFlushI struct {
	Target *MlogVar
}

func (i *MlogPrintFlushI) Run(exec *MlogExecutor) {
	if exec == nil {
		return
	}
	if exec.Host != nil && i.Target != nil {
		_ = exec.Host.PrintFlush(i.Target.ObjVal, exec.TextBuffer.String())
	}
	exec.TextBuffer.Reset()
}

type MlogDrawI struct {
	Cmd  string
	Args []*MlogVar
}

func (i *MlogDrawI) Run(exec *MlogExecutor) {
	if exec == nil || i == nil {
		return
	}
	cmd := strings.ToLower(strings.TrimSpace(i.Cmd))
	if cmd == "" {
		return
	}
	if len(exec.GraphicsBuffer) >= maxGraphicsBuffer {
		return
	}
	switch cmd {
	case "clear":
		r := clampByte(numOrZeroVar(i.Arg(0)) * 255)
		g := clampByte(numOrZeroVar(i.Arg(1)) * 255)
		b := clampByte(numOrZeroVar(i.Arg(2)) * 255)
		a := clampByte(numOrZeroVar(i.Arg(3)) * 255)
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandClear, pack(r), pack(g), pack(b), pack(a), 0, 0))
	case "color":
		r := clampByte(numOrZeroVar(i.Arg(0)) * 255)
		g := clampByte(numOrZeroVar(i.Arg(1)) * 255)
		b := clampByte(numOrZeroVar(i.Arg(2)) * 255)
		a := clampByte(numOrZeroVar(i.Arg(3)) * 255)
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandColor, pack(r), pack(g), pack(b), pack(a), 0, 0))
	case "stroke":
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandStroke, packSignVar(i.Arg(0)), 0, 0, 0, 0, 0))
	case "colorpack":
		packed := int64(math.Float64bits(numOrZeroVar(i.Arg(0))))
		r := (packed >> 56) & 0xFF
		g := (packed >> 48) & 0xFF
		b := (packed >> 40) & 0xFF
		a := (packed >> 32) & 0xFF
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandColor, pack(int(r)), pack(int(g)), pack(int(b)), pack(int(a)), 0, 0))
	case "line":
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandLine, packSignVar(i.Arg(0)), packSignVar(i.Arg(1)), packSignVar(i.Arg(2)), packSignVar(i.Arg(3)), 0, 0))
	case "rect":
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandRect, packSignVar(i.Arg(0)), packSignVar(i.Arg(1)), packSignVar(i.Arg(2)), packSignVar(i.Arg(3)), 0, 0))
	case "linerect":
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandLineRect, packSignVar(i.Arg(0)), packSignVar(i.Arg(1)), packSignVar(i.Arg(2)), packSignVar(i.Arg(3)), 0, 0))
	case "poly":
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandPoly, packSignVar(i.Arg(0)), packSignVar(i.Arg(1)), packSignVar(i.Arg(2)), packSignVar(i.Arg(3)), packSignVar(i.Arg(4)), 0))
	case "linepoly":
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandLinePoly, packSignVar(i.Arg(0)), packSignVar(i.Arg(1)), packSignVar(i.Arg(2)), packSignVar(i.Arg(3)), packSignVar(i.Arg(4)), 0))
	case "triangle":
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandTriangle, packSignVar(i.Arg(0)), packSignVar(i.Arg(1)), packSignVar(i.Arg(2)), packSignVar(i.Arg(3)), packSignVar(i.Arg(4)), packSignVar(i.Arg(5))))
	case "image":
		p1, p4, ok := packImageContent(i.Arg(0))
		if !ok {
			p1 = packSignVar(i.Arg(0))
			p4 = packSignVar(i.Arg(5))
		}
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandImage, packSignVar(i.Arg(1)), packSignVar(i.Arg(2)), p1, packSignVar(i.Arg(3)), packSignVar(i.Arg(4)), p4))
	case "print":
		i.emitPrint(exec)
	case "translate":
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandTranslate, packSignVar(i.Arg(0)), packSignVar(i.Arg(1)), 0, 0, 0, 0))
	case "scale":
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandScale, packSign(int(numOrZeroVar(i.Arg(0))/displayScaleStep)), packSign(int(numOrZeroVar(i.Arg(1))/displayScaleStep)), 0, 0, 0, 0))
	case "rotate":
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandRotate, packSignVar(i.Arg(0)), 0, 0, 0, 0, 0))
	case "reset":
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandResetTransform, 0, 0, 0, 0, 0, 0))
	}
}

type MlogDrawFlushI struct {
	Target *MlogVar
}

func (i *MlogDrawFlushI) Run(exec *MlogExecutor) {
	if exec == nil {
		return
	}
	if exec.Host != nil && i.Target != nil {
		_ = exec.Host.DrawFlush(i.Target.ObjVal, exec.GraphicsBuffer)
	}
	exec.GraphicsBuffer = exec.GraphicsBuffer[:0]
}

type MlogWaitI struct {
	Duration *MlogVar
}

func (i *MlogWaitI) Run(exec *MlogExecutor) {
	if exec == nil || i == nil || i.Duration == nil {
		return
	}
	sec := i.Duration.Num()
	if sec <= 0 {
		return
	}
	now := time.Now()
	if exec.Host != nil {
		now = exec.Host.Now()
	}
	exec.waitUntil = now.Add(time.Duration(sec * float64(time.Second)))
	exec.Yield = true
}

type MlogStopI struct{}

func (i *MlogStopI) Run(exec *MlogExecutor) {
	if exec != nil {
		exec.Yield = true
	}
}

type MlogEndI struct{}

func (i *MlogEndI) Run(exec *MlogExecutor) {
	if exec != nil && exec.Counter != nil {
		exec.Counter.SetNum(float64(len(exec.Program.Instructions)))
	}
}

type MlogNoopI struct{}

func (i *MlogNoopI) Run(exec *MlogExecutor) {}

type MlogGetLinkI struct {
	Out   *MlogVar
	Index *MlogVar
}

func (i *MlogGetLinkI) Run(exec *MlogExecutor) {
	if i == nil || exec == nil || i.Out == nil || i.Index == nil {
		return
	}
	idx := int(i.Index.Num())
	if exec.Host != nil {
		if v, ok := exec.Host.GetLink(idx); ok {
			i.Out.SetObj(v)
			return
		}
	}
	if idx >= 0 && idx < len(exec.GraphicsBuffer) {
		i.Out.SetObj(exec.GraphicsBuffer[idx])
		return
	}
	i.Out.SetObj(nil)
}

type MlogPackColorI struct {
	Out       *MlogVar
	R, G, B, A *MlogVar
}

func (i *MlogPackColorI) Run(exec *MlogExecutor) {
	if i == nil || i.Out == nil || i.R == nil || i.G == nil || i.B == nil || i.A == nil {
		return
	}
	r := clampByte(i.R.Num())
	g := clampByte(i.G.Num())
	b := clampByte(i.B.Num())
	a := clampByte(i.A.Num())
	packed := uint32(r) | uint32(g)<<8 | uint32(b)<<16 | uint32(a)<<24
	i.Out.SetNum(float64(math.Float32frombits(packed)))
}

type MlogSetRateI struct {
	Rate *MlogVar
}

func (i *MlogSetRateI) Run(exec *MlogExecutor) {
	if exec == nil || i == nil || i.Rate == nil {
		return
	}
	val := i.Rate.Num()
	if val <= 0 {
		val = 1
	}
	exec.rate = val
}

type MlogLookupI struct {
	Out   *MlogVar
	Kind  string
	Index *MlogVar
}

func (i *MlogLookupI) Run(exec *MlogExecutor) {
	if i == nil || i.Out == nil || i.Index == nil {
		return
	}
	if exec != nil && exec.Host != nil {
		if v, ok := exec.Host.Lookup(i.Kind, int(i.Index.Num())); ok {
			i.Out.SetObj(v)
			return
		}
	}
	i.Out.SetObj(nil)
}

type MlogGetBlockI struct {
	Out   *MlogVar
	X, Y  *MlogVar
	Layer string
}

func (i *MlogGetBlockI) Run(exec *MlogExecutor) {
	if i == nil || i.Out == nil || i.X == nil || i.Y == nil {
		return
	}
	if exec != nil && exec.Host != nil {
		if v, ok := exec.Host.GetBlock(int(i.X.Num()), int(i.Y.Num()), i.Layer); ok {
			i.Out.SetObj(v)
			return
		}
	}
	i.Out.SetObj(nil)
}

type MlogSetBlockI struct {
	Block    *MlogVar
	X, Y     *MlogVar
	Team     *MlogVar
	Rotation *MlogVar
}

func (i *MlogSetBlockI) Run(exec *MlogExecutor) {
	if exec == nil || i == nil || i.Block == nil || i.X == nil || i.Y == nil || i.Team == nil || i.Rotation == nil {
		return
	}
	if exec.Host != nil {
		_ = exec.Host.SetBlock(int(i.X.Num()), int(i.Y.Num()), i.Block.ObjVal, int(i.Team.Num()), int(i.Rotation.Num()))
	}
}

type MlogSensorI struct {
	Out    *MlogVar
	Target *MlogVar
	Sensor string
}

func (i *MlogSensorI) Run(exec *MlogExecutor) {
	if i == nil || i.Out == nil || i.Target == nil {
		return
	}
	if exec != nil && exec.Host != nil {
		if v, ok := exec.Host.Sensor(i.Target.ObjVal, i.Sensor); ok {
			if num, ok2 := v.(float64); ok2 {
				i.Out.SetNum(num)
			} else {
				i.Out.SetObj(v)
			}
			return
		}
	}
	i.Out.SetNum(0)
}

type MlogControlI struct {
	Target *MlogVar
	Action string
	A, B, C, D *MlogVar
}

func (i *MlogControlI) Run(exec *MlogExecutor) {
	if exec == nil || i == nil || i.Target == nil {
		return
	}
	if exec.Host != nil {
		_ = exec.Host.Control(i.Target.ObjVal, i.Action, i.A, i.B, i.C, i.D)
	}
}

type MlogUControlI struct {
	Action string
	A, B, C, D *MlogVar
}

func (i *MlogUControlI) Run(exec *MlogExecutor) {
	if exec == nil || i == nil {
		return
	}
	if exec.Host != nil {
		_ = exec.Host.UControl(exec.Unit.ObjVal, i.Action, i.A, i.B, i.C, i.D)
	}
}

type MlogUnitBindI struct {
	Type *MlogVar
}

func (i *MlogUnitBindI) Run(exec *MlogExecutor) {
	if exec == nil || i == nil || i.Type == nil || exec.Host == nil {
		return
	}
	// delegate binding to host; store in @unit
	if v, ok := exec.Host.Fetch("unitbind", nil, nil, int(i.Type.Num())); ok {
		exec.Unit.SetObj(v)
		return
	}
	exec.Unit.SetObj(nil)
}

type MlogRadarI struct {
	Filter   string
	Sort     string
	Team     *MlogVar
	Block    *MlogVar
	SortMode string
	Out      *MlogVar
}

func (i *MlogRadarI) Run(exec *MlogExecutor) {
	if exec == nil || exec.Host == nil || i == nil || i.Out == nil {
		return
	}
	if v, ok := exec.Host.Radar(i.Block.ObjVal, i.Filter, i.SortMode, int(numOrZero(i.Team))); ok {
		i.Out.SetObj(v)
		return
	}
	i.Out.SetObj(nil)
}

type MlogURadarI struct {
	Filter   string
	Sort     string
	Team     *MlogVar
	Unit     *MlogVar
	SortMode string
	Out      *MlogVar
}

func (i *MlogURadarI) Run(exec *MlogExecutor) {
	if exec == nil || exec.Host == nil || i == nil || i.Out == nil {
		return
	}
	if v, ok := exec.Host.URadar(i.Unit.ObjVal, i.Filter, i.SortMode, int(numOrZero(i.Team))); ok {
		i.Out.SetObj(v)
		return
	}
	i.Out.SetObj(nil)
}

type MlogULocateI struct {
	Locate   string
	Flag     string
	Enemy    *MlogVar
	Ore      *MlogVar
	OutX     *MlogVar
	OutY     *MlogVar
	OutFound *MlogVar
}

func (i *MlogULocateI) Run(exec *MlogExecutor) {
	if exec == nil || exec.Host == nil || i == nil || i.OutX == nil || i.OutY == nil || i.OutFound == nil {
		return
	}
	found, x, y, obj := exec.Host.Locate(exec.Unit.ObjVal, i.Locate, i.Flag, numOrZero(i.Enemy) != 0, objOrNil(i.Ore))
	i.OutFound.SetNum(boolToNum(found))
	if found {
		i.OutX.SetNum(x)
		i.OutY.SetNum(y)
		if obj != nil {
			exec.Unit.SetObj(obj)
		}
	}
}

type MlogFetchI struct {
	Out   *MlogVar
	Kind  string
	Team  *MlogVar
	Extra *MlogVar
	Index *MlogVar
}

func (i *MlogFetchI) Run(exec *MlogExecutor) {
	if exec == nil || exec.Host == nil || i == nil || i.Out == nil || i.Index == nil {
		return
	}
	if v, ok := exec.Host.Fetch(i.Kind, objOrNil(i.Team), objOrNil(i.Extra), int(i.Index.Num())); ok {
		i.Out.SetObj(v)
		return
	}
	i.Out.SetObj(nil)
}

type MlogSyncI struct {
	Name  string
	Value *MlogVar
}

func (i *MlogSyncI) Run(exec *MlogExecutor) {
	if exec == nil || exec.Host == nil || i == nil || i.Value == nil || exec.Program == nil {
		return
	}
	if exec.Program.VarByName == nil {
		return
	}
	v := exec.Program.VarByName[i.Name]
	if v == nil {
		return
	}
	val := i.Value.ObjVal
	if !i.Value.IsObj {
		val = i.Value.Num()
	}
	_ = exec.Host.SyncVar(v.ID, val)
}

type MlogClientDataI struct {
	Channel  *MlogVar
	Value    *MlogVar
	Reliable *MlogVar
}

func (i *MlogClientDataI) Run(exec *MlogExecutor) {
	if exec == nil || exec.Host == nil || i == nil || i.Channel == nil || i.Value == nil || i.Reliable == nil {
		return
	}
	if !i.Channel.IsObj {
		return
	}
	ch, ok := i.Channel.ObjVal.(string)
	if !ok || ch == "" {
		return
	}
	val := i.Value.ObjVal
	if !i.Value.IsObj {
		val = i.Value.Num()
	}
	_ = exec.Host.ClientData(ch, val, i.Reliable.Num() != 0)
}

func boolToNum(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func clampByte(v float64) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return int(v)
}

func numOrZero(v *MlogVar) float64 {
	if v == nil {
		return 0
	}
	return v.Num()
}

func numOrZeroVar(v *MlogVar) float64 {
	if v == nil {
		return 0
	}
	return v.Num()
}

func (i *MlogDrawI) Arg(idx int) *MlogVar {
	if i == nil || idx < 0 || idx >= len(i.Args) {
		return nil
	}
	return i.Args[idx]
}

const (
	commandClear          = 0
	commandColor          = 1
	commandColorPack      = 2
	commandStroke         = 3
	commandLine           = 4
	commandRect           = 5
	commandLineRect       = 6
	commandPoly           = 7
	commandLinePoly       = 8
	commandTriangle       = 9
	commandImage          = 10
	commandPrint          = 11
	commandTranslate      = 12
	commandScale          = 13
	commandRotate         = 14
	commandResetTransform = 15
)

const (
	maxGraphicsBuffer = 4096
	displayScaleStep  = 0.05
)

func pack(v int) int {
	return v & 0x3FF
}

func packSign(v int) int {
	if v < 0 {
		return (int(-v) & 0x1FF) | 0x200
	}
	return v & 0x1FF
}

func packSignVar(v *MlogVar) int {
	return packSign(int(numOrZeroVar(v)))
}

func packDisplayCmd(typ int, x, y, p1, p2, p3, p4 int) uint64 {
	var out uint64
	out |= uint64(typ&0xF) << 60
	out |= uint64(x&0x3FF) << 50
	out |= uint64(y&0x3FF) << 40
	out |= uint64(p1&0x3FF) << 30
	out |= uint64(p2&0x3FF) << 20
	out |= uint64(p3&0x3FF) << 10
	out |= uint64(p4 & 0x3FF)
	return out
}

func packImageContent(v *MlogVar) (int, int, bool) {
	if v == nil || !v.IsObj || v.ObjVal == nil {
		return 0, 0, false
	}
	switch t := v.ObjVal.(type) {
	case interface {
		ID() int16
		ContentType() protocol.ContentType
	}:
		packed := (int(t.ID()) << 5) | (int(t.ContentType()) & 31)
		return packed & 0x3FF, (packed >> 10) & 0x3FF, true
	default:
		return 0, 0, false
	}
}

func (i *MlogDrawI) emitPrint(exec *MlogExecutor) {
	if exec == nil {
		return
	}
	str := exec.TextBuffer.String()
	if str == "" {
		return
	}
	x := int(numOrZeroVar(i.Arg(0)))
	y := int(numOrZeroVar(i.Arg(1)))
	for idx := 0; idx < len(str); idx++ {
		exec.GraphicsBuffer = append(exec.GraphicsBuffer, packDisplayCmd(commandPrint, packSign(x), packSign(y), int(str[idx]), 0, 0, 0))
		if len(exec.GraphicsBuffer) >= maxGraphicsBuffer {
			break
		}
		x++
	}
	exec.TextBuffer.Reset()
}

func formatMlogObj(obj any) string {
	switch t := obj.(type) {
	case nil:
		return "null"
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	case protocol.Content:
		return t.Name()
	case protocol.Team:
		return t.Name
	case interface{ Name() string }:
		return t.Name()
	case interface{ ID() int16 }:
		return fmt.Sprintf("[%d]", t.ID())
	case interface{ Pos() int32 }:
		return fmt.Sprintf("[%d]", t.Pos())
	default:
		return fmt.Sprint(obj)
	}
}

func objOrNil(v *MlogVar) any {
	if v == nil {
		return nil
	}
	return v.ObjVal
}

func formatMlogNum(v float64) string {
	if invalidNum(v) {
		return "0"
	}
	if math.Abs(v-math.Round(v)) < 0.000001 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.3f", v)
}

func randUnit() float64 {
	return float64(time.Now().UnixNano()%1000000) / 1000000.0
}
