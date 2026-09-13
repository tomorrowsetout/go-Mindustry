package video

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"

	"mdt-server/internal/world"
)

func TestComputeRenderLayoutFitsWorldIntoCanvas(t *testing.T) {
	model := world.NewWorldModel(400, 200)
	layout := computeRenderLayout(model, 1920, 1080, 0)

	if layout.canvasWidth != 1920 || layout.canvasHeight != 1080 {
		t.Fatalf("unexpected canvas size: %+v", layout)
	}
	if layout.scale <= 0 {
		t.Fatalf("expected positive scale, got %f", layout.scale)
	}

	left, top, right, bottom := mapBounds(layout, model)
	if left < 0 || top < 0 {
		t.Fatalf("expected map origin within canvas, got (%d,%d)", left, top)
	}
	if right > layout.canvasWidth || bottom > layout.canvasHeight {
		t.Fatalf("expected map to fit canvas, got bounds (%d,%d)-(%d,%d) within %dx%d", left, top, right, bottom, layout.canvasWidth, layout.canvasHeight)
	}
}

func TestNormalizeConfigDefaultsTo1080p30(t *testing.T) {
	cfg := normalizeConfig(Config{})
	if cfg.FPS != 30 {
		t.Fatalf("expected default fps 30, got %d", cfg.FPS)
	}
	if cfg.Width != 1920 || cfg.Height != 1080 {
		t.Fatalf("expected default 1920x1080, got %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.JPEGQuality != 90 {
		t.Fatalf("expected default jpeg quality 90, got %d", cfg.JPEGQuality)
	}
}

func TestRenderModelDrawsBuildingsAndUnits(t *testing.T) {
	model := world.NewWorldModel(4, 4)

	tile, err := model.TileAt(1, 1)
	if err != nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	tile.Floor = 3
	tile.Block = 12
	tile.Team = 2
	tile.Build = &world.Building{Block: 12, Team: 2, X: 1, Y: 1}

	model.Units[1] = &world.Unit{
		ID:   1,
		Team: 1,
		Pos:  world.Vec2{X: 8 * 2, Y: 8 * 2},
	}

	layout := computeRenderLayout(model, 32, 32, 0)
	img := renderModel(model, layout, nil, basicFontFace())
	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 32 {
		t.Fatalf("unexpected image size: %v", img.Bounds())
	}

	buildPx := img.RGBAAt(12, 12)
	if buildPx == (color.RGBA{R: 12, G: 14, B: 18, A: 255}) {
		t.Fatal("expected building pixel to differ from background")
	}

	unitPx := img.RGBAAt(16, 16)
	if unitPx == (color.RGBA{R: 12, G: 14, B: 18, A: 255}) {
		t.Fatal("expected unit pixel to differ from background")
	}
}

func TestRenderModelHighlightsTrackedPlayers(t *testing.T) {
	model := world.NewWorldModel(4, 4)
	model.Units[9] = &world.Unit{
		ID:   9,
		Team: 1,
		Pos:  world.Vec2{X: 8 * 2, Y: 8 * 2},
	}

	layout := computeRenderLayout(model, 64, 64, 0)
	img := renderModel(model, layout, []PlayerState{{
		Name:      "alpha",
		UUID:      "u1",
		UnitID:    9,
		TeamID:    1,
		X:         16,
		Y:         16,
		Connected: true,
	}}, basicFontFace())

	highlight := img.RGBAAt(37, 32)
	if highlight == (color.RGBA{R: 12, G: 14, B: 18, A: 255}) {
		t.Fatal("expected player highlight ring to be drawn")
	}
}

func TestAVIWriterCreatesRIFFFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.avi")
	w, err := NewAVIWriter(path, 64, 64, 15, 90)
	if err != nil {
		t.Fatalf("new avi writer: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	if err := w.AddFrame(img); err != nil {
		t.Fatalf("add frame: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if len(data) < 12 {
		t.Fatalf("unexpected file size: %d", len(data))
	}
	if string(data[:4]) != "RIFF" || string(data[8:12]) != "AVI " {
		t.Fatalf("unexpected avi header: %q %q", string(data[:4]), string(data[8:12]))
	}
}

func basicFontFace() font.Face {
	return basicfont.Face7x13
}
