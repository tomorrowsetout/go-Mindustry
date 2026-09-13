package video

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"mdt-server/internal/world"
)

const worldUnitsPerTile = 8.0

type Config struct {
	Enabled     bool
	OutputDir   string
	FPS         int
	Width       int
	Height      int
	TileSize    int
	JPEGQuality int
	QueueSize   int
}

type PlayerState struct {
	Name      string  `json:"name,omitempty"`
	UUID      string  `json:"uuid,omitempty"`
	UnitID    int32   `json:"unit_id,omitempty"`
	TeamID    byte    `json:"team_id,omitempty"`
	X         float32 `json:"x,omitempty"`
	Y         float32 `json:"y,omitempty"`
	Connected bool    `json:"connected"`
	Dead      bool    `json:"dead,omitempty"`
}

type Recorder struct {
	cfg Config

	sessionDir string
	manifest   string
	metadata   string
	videoPath  string

	cloneModel func() *world.WorldModel
	snapshot   func() world.Snapshot
	currentMap func() string
	players    func() []PlayerState

	writer   *AVIWriter
	fontFace font.Face

	stopOnce sync.Once
	stopCh   chan struct{}
	frameCh  chan captureTask
	wg       sync.WaitGroup

	mu sync.Mutex

	startedAt time.Time
	endedAt   time.Time

	layoutReady bool
	layout      renderLayout
	mapWidth    int
	mapHeight   int

	framesWritten int
	droppedFrames int

	firstErr error
}

type captureTask struct {
	CapturedAt time.Time
	MapPath    string
	Snapshot   world.Snapshot
	Model      *world.WorldModel
	Players    []PlayerState
}

type metadataFile struct {
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at"`
	MapPath       string    `json:"map_path,omitempty"`
	FPS           int       `json:"fps"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	PixelsPerTile float64   `json:"pixels_per_tile"`
	FramesWritten int       `json:"frames_written"`
	DroppedFrames int       `json:"dropped_frames"`
	EncodingMode  string    `json:"encoding_mode"`
	VideoPath     string    `json:"video_path,omitempty"`
}

type frameManifestEntry struct {
	Frame     int           `json:"frame"`
	Timestamp time.Time     `json:"timestamp"`
	MapPath   string        `json:"map_path,omitempty"`
	Tick      uint64        `json:"tick"`
	Wave      int32         `json:"wave"`
	Units     int           `json:"units"`
	Entities  int           `json:"entities"`
	Players   []PlayerState `json:"players,omitempty"`
}

type renderLayout struct {
	canvasWidth  int
	canvasHeight int
	scale        float64
	offsetX      float64
	offsetY      float64
}

func Start(cfg Config, cloneModel func() *world.WorldModel, snapshot func() world.Snapshot, currentMap func() string, players func() []PlayerState) (*Recorder, error) {
	cfg = normalizeConfig(cfg)
	if !cfg.Enabled {
		return nil, nil
	}
	if cloneModel == nil {
		return nil, fmt.Errorf("video recorder requires cloneModel callback")
	}

	startedAt := time.Now()
	sessionDir := filepath.Join(cfg.OutputDir, buildSessionName(startedAt, currentMap))
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, err
	}

	fontFace, err := loadRecorderFont(cfg.Height)
	if err != nil {
		return nil, err
	}
	writer, err := NewAVIWriter(filepath.Join(sessionDir, "match.avi"), cfg.Width, cfg.Height, cfg.FPS, cfg.JPEGQuality)
	if err != nil {
		if c, ok := fontFace.(io.Closer); ok {
			_ = c.Close()
		}
		return nil, err
	}

	r := &Recorder{
		cfg:        cfg,
		sessionDir: sessionDir,
		manifest:   filepath.Join(sessionDir, "timeline.jsonl"),
		metadata:   filepath.Join(sessionDir, "metadata.json"),
		videoPath:  filepath.Join(sessionDir, "match.avi"),
		cloneModel: cloneModel,
		snapshot:   snapshot,
		currentMap: currentMap,
		players:    players,
		writer:     writer,
		fontFace:   fontFace,
		stopCh:     make(chan struct{}),
		frameCh:    make(chan captureTask, cfg.QueueSize),
		startedAt:  startedAt,
	}

	r.wg.Add(2)
	go r.captureLoop()
	go r.renderLoop()
	r.captureNow()
	return r, nil
}

func (r *Recorder) Close() error {
	if r == nil {
		return nil
	}
	r.stopOnce.Do(func() {
		close(r.stopCh)
		r.wg.Wait()

		r.mu.Lock()
		r.endedAt = time.Now()
		r.mu.Unlock()

		if r.writer != nil {
			if err := r.writer.Close(); err != nil {
				r.setError(err)
			}
		}
		if c, ok := r.fontFace.(io.Closer); ok {
			_ = c.Close()
		}
		if err := r.writeMetadata(); err != nil {
			r.setError(err)
		}
	})
	return r.err()
}

func (r *Recorder) SessionDir() string {
	if r == nil {
		return ""
	}
	return r.sessionDir
}

func (r *Recorder) VideoPath() string {
	if r == nil {
		return ""
	}
	return r.videoPath
}

func (r *Recorder) FrameCount() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.framesWritten
}

func (r *Recorder) DroppedCount() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.droppedFrames
}

func (r *Recorder) captureLoop() {
	defer r.wg.Done()
	defer close(r.frameCh)

	interval := time.Second / time.Duration(r.cfg.FPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.captureNow()
		}
	}
}

func (r *Recorder) captureNow() {
	if r == nil {
		return
	}
	if cap(r.frameCh) > 0 && len(r.frameCh) >= cap(r.frameCh) {
		r.mu.Lock()
		r.droppedFrames++
		r.mu.Unlock()
		return
	}

	model := r.cloneModel()
	if model == nil {
		return
	}

	task := captureTask{
		CapturedAt: time.Now(),
		Model:      model,
	}
	if r.snapshot != nil {
		task.Snapshot = r.snapshot()
	}
	if r.currentMap != nil {
		task.MapPath = r.currentMap()
	}
	if r.players != nil {
		task.Players = r.players()
	}

	select {
	case r.frameCh <- task:
	default:
		r.mu.Lock()
		r.droppedFrames++
		r.mu.Unlock()
	}
}

func (r *Recorder) renderLoop() {
	defer r.wg.Done()

	written := 0
	for task := range r.frameCh {
		frameNumber := written + 1
		entry, err := r.renderTask(frameNumber, task)
		if err != nil {
			r.setError(err)
			continue
		}
		written++
		r.mu.Lock()
		r.framesWritten = written
		r.mu.Unlock()
		if err := appendJSONLine(r.manifest, entry); err != nil {
			r.setError(err)
		}
	}
}

func (r *Recorder) renderTask(frameNumber int, task captureTask) (frameManifestEntry, error) {
	layout := r.ensureLayout(task.Model)
	img := renderModel(task.Model, layout, task.Players, r.fontFace)
	if err := r.writer.AddFrame(img); err != nil {
		return frameManifestEntry{}, err
	}
	return frameManifestEntry{
		Frame:     frameNumber,
		Timestamp: task.CapturedAt,
		MapPath:   task.MapPath,
		Tick:      task.Snapshot.Tick,
		Wave:      task.Snapshot.Wave,
		Units:     len(task.Model.Units),
		Entities:  len(task.Model.Entities),
		Players:   task.Players,
	}, nil
}

func (r *Recorder) ensureLayout(model *world.WorldModel) renderLayout {
	r.mu.Lock()
	defer r.mu.Unlock()

	modelWidth := 0
	modelHeight := 0
	if model != nil {
		modelWidth = model.Width
		modelHeight = model.Height
	}

	if !r.layoutReady || modelWidth != r.mapWidth || modelHeight != r.mapHeight {
		r.layout = computeRenderLayout(model, r.cfg.Width, r.cfg.Height, r.cfg.TileSize)
		r.mapWidth = modelWidth
		r.mapHeight = modelHeight
		r.layoutReady = true
	}

	return r.layout
}

func (r *Recorder) writeMetadata() error {
	r.mu.Lock()
	meta := metadataFile{
		StartedAt:     r.startedAt,
		EndedAt:       r.endedAt,
		MapPath:       safeCurrentMap(r.currentMap),
		FPS:           r.cfg.FPS,
		Width:         r.layout.canvasWidth,
		Height:        r.layout.canvasHeight,
		PixelsPerTile: r.layout.scale,
		FramesWritten: r.framesWritten,
		DroppedFrames: r.droppedFrames,
		EncodingMode:  "realtime_mjpeg_avi",
	}
	if r.framesWritten > 0 {
		meta.VideoPath = r.videoPath
	}
	r.mu.Unlock()

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(r.metadata, data, 0o644)
}

func (r *Recorder) setError(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.firstErr == nil {
		r.firstErr = err
	}
}

func (r *Recorder) err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.firstErr
}

func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.OutputDir) == "" {
		cfg.OutputDir = filepath.Join("data", "video")
	}
	if cfg.FPS <= 0 {
		cfg.FPS = 30
	}
	if cfg.Width <= 0 {
		cfg.Width = 1920
	}
	if cfg.Height <= 0 {
		cfg.Height = 1080
	}
	cfg.Width = makeEven(max(2, cfg.Width))
	cfg.Height = makeEven(max(2, cfg.Height))
	if cfg.JPEGQuality <= 0 {
		cfg.JPEGQuality = 90
	}
	if cfg.JPEGQuality > 100 {
		cfg.JPEGQuality = 100
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 2
	}
	return cfg
}

func buildSessionName(now time.Time, currentMap func() string) string {
	mapName := "session"
	if currentMap != nil {
		if base := strings.TrimSpace(filepath.Base(currentMap())); base != "" {
			base = strings.TrimSuffix(base, filepath.Ext(base))
			base = sanitizeFilename(base)
			if base != "" {
				mapName = base
			}
		}
	}
	return fmt.Sprintf("%s-%s", mapName, now.Format("20060102-150405"))
}

func safeCurrentMap(currentMap func() string) string {
	if currentMap == nil {
		return ""
	}
	return currentMap()
}

func appendJSONLine(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

func computeRenderLayout(model *world.WorldModel, width, height, requestedTileSize int) renderLayout {
	width = max(2, width)
	height = max(2, height)
	layout := renderLayout{
		canvasWidth:  width,
		canvasHeight: height,
		scale:        1,
	}
	if model == nil || model.Width <= 0 || model.Height <= 0 {
		return layout
	}

	worldWidth := float64(model.Width)
	worldHeight := float64(model.Height)
	scaleX := float64(width) / worldWidth
	scaleY := float64(height) / worldHeight
	scale := math.Min(scaleX, scaleY)
	if requestedTileSize > 0 {
		scale = math.Min(scale, float64(requestedTileSize))
	}
	if scale <= 0 {
		scale = 1
	}

	layout.scale = scale
	layout.offsetX = (float64(width) - worldWidth*scale) / 2
	layout.offsetY = (float64(height) - worldHeight*scale) / 2
	return layout
}

func renderModel(model *world.WorldModel, layout renderLayout, players []PlayerState, face font.Face) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, max(1, layout.canvasWidth), max(1, layout.canvasHeight)))
	fillRect(canvas, 0, 0, layout.canvasWidth, layout.canvasHeight, color.RGBA{R: 12, G: 14, B: 18, A: 255})
	if model == nil || layout.scale <= 0 {
		return canvas
	}

	left, top, right, bottom := mapBounds(layout, model)
	fillRect(canvas, left, top, max(1, right-left), max(1, bottom-top), color.RGBA{R: 16, G: 18, B: 22, A: 255})

	for i := range model.Tiles {
		tile := model.Tiles[i]
		x0, y0, x1, y1 := tileRect(layout, tile.X, tile.Y)
		fillRect(canvas, x0, y0, max(1, x1-x0), max(1, y1-y0), floorColor(tile))
	}

	for i := range model.Tiles {
		tile := model.Tiles[i]
		if tile.Build == nil && tile.Block == 0 {
			continue
		}
		x0, y0, x1, y1 := tileRect(layout, tile.X, tile.Y)
		inset := max(1, int(math.Round(layout.scale*0.14)))
		blockID := tile.Block
		team := tile.Team
		if tile.Build != nil {
			blockID = tile.Build.Block
			team = tile.Build.Team
		}
		fillRect(canvas, x0+inset, y0+inset, max(1, (x1-x0)-inset*2), max(1, (y1-y0)-inset*2), buildingColor(team, blockID))
	}

	entityRadius := markerRadius(layout.scale, 0.20)
	unitRadius := markerRadius(layout.scale, 0.35)
	playerRadius := markerRadius(layout.scale, 0.50)

	for _, ent := range model.Entities {
		px, py := worldPoint(layout, ent.X, ent.Y)
		drawCross(canvas, px, py, entityRadius, entityColor(ent.TypeID))
	}

	for _, unit := range model.Units {
		if unit == nil {
			continue
		}
		px, py := worldPoint(layout, unit.Pos.X, unit.Pos.Y)
		drawCircle(canvas, px, py, unitRadius, teamColor(unit.Team))
	}

	for _, player := range players {
		if !player.Connected {
			continue
		}
		px, py := worldPoint(layout, player.X, player.Y)
		if unit := lookupUnit(model, player.UnitID); unit != nil {
			px, py = worldPoint(layout, unit.Pos.X, unit.Pos.Y)
		}

		ringColor := teamColor(world.TeamID(player.TeamID))
		if player.Dead {
			ringColor = color.RGBA{R: 120, G: 124, B: 130, A: 255}
		}
		drawRing(canvas, px, py, max(2, playerRadius), ringColor)
		drawPlayerName(canvas, face, px, py-max(8, playerRadius+4), player.Name, ringColor)
	}

	drawMapBorder(canvas, left, top, right, bottom)
	return canvas
}

func mapBounds(layout renderLayout, model *world.WorldModel) (left, top, right, bottom int) {
	left = int(math.Floor(layout.offsetX))
	top = int(math.Floor(layout.offsetY))
	right = int(math.Ceil(layout.offsetX + float64(model.Width)*layout.scale))
	bottom = int(math.Ceil(layout.offsetY + float64(model.Height)*layout.scale))
	return
}

func tileRect(layout renderLayout, tileX, tileY int) (x0, y0, x1, y1 int) {
	x0 = int(math.Floor(layout.offsetX + float64(tileX)*layout.scale))
	y0 = int(math.Floor(layout.offsetY + float64(tileY)*layout.scale))
	x1 = int(math.Ceil(layout.offsetX + float64(tileX+1)*layout.scale))
	y1 = int(math.Ceil(layout.offsetY + float64(tileY+1)*layout.scale))
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}
	return
}

func worldPoint(layout renderLayout, worldX, worldY float32) (int, int) {
	tileX := float64(worldX) / worldUnitsPerTile
	tileY := float64(worldY) / worldUnitsPerTile
	px := int(math.Round(layout.offsetX + tileX*layout.scale))
	py := int(math.Round(layout.offsetY + tileY*layout.scale))
	return px, py
}

func markerRadius(scale float64, factor float64) int {
	r := int(math.Round(scale * factor))
	if r < 1 {
		return 1
	}
	return r
}

func drawMapBorder(img *image.RGBA, left, top, right, bottom int) {
	border := color.RGBA{R: 60, G: 64, B: 72, A: 255}
	fillRect(img, left-1, top-1, max(1, right-left+2), 1, border)
	fillRect(img, left-1, bottom, max(1, right-left+2), 1, border)
	fillRect(img, left-1, top, 1, max(1, bottom-top), border)
	fillRect(img, right, top, 1, max(1, bottom-top), border)
}

func lookupUnit(model *world.WorldModel, unitID int32) *world.Unit {
	if model == nil || unitID == 0 || model.Units == nil {
		return nil
	}
	return model.Units[unitID]
}

func floorColor(tile world.Tile) color.RGBA {
	base := mutedHashColor(int(tile.Floor)*37+int(tile.Overlay)*19+17, 26, 30)
	if tile.Overlay != 0 {
		base = mixColor(base, color.RGBA{R: 72, G: 84, B: 90, A: 255}, 0.18)
	}
	return base
}

func buildingColor(team world.TeamID, block world.BlockID) color.RGBA {
	return mixColor(teamColor(team), mutedHashColor(int(block)*29+7, 72, 82), 0.28)
}

func entityColor(typeID int16) color.RGBA {
	return mutedHashColor(int(typeID)*41+3, 100, 96)
}

func teamColor(team world.TeamID) color.RGBA {
	switch int(team) {
	case 1:
		return color.RGBA{R: 88, G: 190, B: 255, A: 255}
	case 2:
		return color.RGBA{R: 255, G: 102, B: 86, A: 255}
	case 3:
		return color.RGBA{R: 121, G: 214, B: 93, A: 255}
	case 4:
		return color.RGBA{R: 249, G: 195, B: 76, A: 255}
	case 5:
		return color.RGBA{R: 188, G: 134, B: 255, A: 255}
	default:
		return color.RGBA{R: 210, G: 214, B: 222, A: 255}
	}
}

func mutedHashColor(seed int, spread byte, base byte) color.RGBA {
	h := uint32(seed)
	h ^= h >> 16
	h *= 0x7feb352d
	h ^= h >> 15
	h *= 0x846ca68b
	h ^= h >> 16

	clamp := func(v int) byte {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return byte(v)
	}

	return color.RGBA{
		R: clamp(int(base) + int(h&uint32(spread))),
		G: clamp(int(base) + int((h>>8)&uint32(spread))),
		B: clamp(int(base) + int((h>>16)&uint32(spread))),
		A: 255,
	}
}

func mixColor(a, b color.RGBA, amount float64) color.RGBA {
	if amount <= 0 {
		return a
	}
	if amount >= 1 {
		return b
	}
	mix := func(x, y byte) byte {
		return byte(math.Round(float64(x)*(1-amount) + float64(y)*amount))
	}
	return color.RGBA{
		R: mix(a.R, b.R),
		G: mix(a.G, b.G),
		B: mix(a.B, b.B),
		A: 255,
	}
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	if img == nil || w <= 0 || h <= 0 {
		return
	}
	bounds := img.Bounds()
	startX := max(x, bounds.Min.X)
	startY := max(y, bounds.Min.Y)
	endX := min(x+w, bounds.Max.X)
	endY := min(y+h, bounds.Max.Y)
	for py := startY; py < endY; py++ {
		for px := startX; px < endX; px++ {
			img.SetRGBA(px, py, c)
		}
	}
}

func drawCircle(img *image.RGBA, cx, cy, radius int, c color.RGBA) {
	if img == nil {
		return
	}
	if radius <= 0 {
		if image.Pt(cx, cy).In(img.Bounds()) {
			img.SetRGBA(cx, cy, c)
		}
		return
	}
	r2 := radius * radius
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y > r2 {
				continue
			}
			px := cx + x
			py := cy + y
			if image.Pt(px, py).In(img.Bounds()) {
				img.SetRGBA(px, py, c)
			}
		}
	}
}

func drawRing(img *image.RGBA, cx, cy, radius int, c color.RGBA) {
	if img == nil || radius <= 0 {
		return
	}
	outer := radius * radius
	innerRadius := max(0, radius-2)
	inner := innerRadius * innerRadius
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			dist := x*x + y*y
			if dist > outer || dist < inner {
				continue
			}
			px := cx + x
			py := cy + y
			if image.Pt(px, py).In(img.Bounds()) {
				img.SetRGBA(px, py, c)
			}
		}
	}
}

func drawCross(img *image.RGBA, cx, cy, radius int, c color.RGBA) {
	if img == nil {
		return
	}
	for delta := -radius; delta <= radius; delta++ {
		for _, pt := range [][2]int{
			{cx + delta, cy},
			{cx, cy + delta},
		} {
			if image.Pt(pt[0], pt[1]).In(img.Bounds()) {
				img.SetRGBA(pt[0], pt[1], c)
			}
		}
	}
}

func drawPlayerName(img *image.RGBA, face font.Face, x, y int, text string, c color.RGBA) {
	if img == nil || face == nil || strings.TrimSpace(text) == "" {
		return
	}
	d := &font.Drawer{Face: face}
	textWidth := d.MeasureString(text).Ceil()
	ascent := face.Metrics().Ascent.Ceil()
	baseX := x - textWidth/2
	baseY := y
	if baseX < 0 {
		baseX = 0
	}
	if baseX+textWidth >= img.Bounds().Dx() {
		baseX = max(0, img.Bounds().Dx()-textWidth-1)
	}
	baseY = max(ascent, baseY)

	d.Dst = img
	d.Face = face

	shadow := image.NewUniform(color.RGBA{R: 0, G: 0, B: 0, A: 200})
	d.Src = shadow
	d.Dot = fixed.P(baseX+1, baseY+1)
	d.DrawString(text)

	d.Src = image.NewUniform(c)
	d.Dot = fixed.P(baseX, baseY)
	d.DrawString(text)
}

func loadRecorderFont(videoHeight int) (font.Face, error) {
	size := float64(max(14, videoHeight/54))
	candidates := []string{
		filepath.Join(os.Getenv("SystemRoot"), "Fonts", "msyh.ttc"),
		filepath.Join(os.Getenv("SystemRoot"), "Fonts", "simhei.ttf"),
		filepath.Join(os.Getenv("SystemRoot"), "Fonts", "simsun.ttc"),
		filepath.Join(os.Getenv("SystemRoot"), "Fonts", "arial.ttf"),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		face, err := parseFontFace(data, size)
		if err == nil {
			return face, nil
		}
	}
	return basicfont.Face7x13, nil
}

func parseFontFace(data []byte, size float64) (font.Face, error) {
	opts := &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	}
	if f, err := opentype.Parse(data); err == nil {
		return opentype.NewFace(f, opts)
	}
	coll, err := opentype.ParseCollection(data)
	if err != nil {
		return nil, err
	}
	for i := 0; i < coll.NumFonts(); i++ {
		f, err := coll.Font(i)
		if err != nil {
			continue
		}
		face, err := opentype.NewFace(f, opts)
		if err == nil {
			return face, nil
		}
	}
	return nil, fmt.Errorf("no usable font face in collection")
}

func sanitizeFilename(s string) string {
	repl := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_", " ", "_",
	)
	out := repl.Replace(strings.TrimSpace(s))
	out = strings.Trim(out, "._")
	if out == "" {
		return ""
	}
	return out
}

func makeEven(v int) int {
	if v%2 != 0 {
		return v + 1
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
