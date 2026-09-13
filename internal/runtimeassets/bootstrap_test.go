package runtimeassets

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestBootstrapWorldCandidatesIncludeRuntimeAndBinAssets(t *testing.T) {
	roots := []string{
		filepath.Clean(`C:\srv\mdt-server`),
		filepath.Clean(`C:\srv\mdt-server\bin`),
	}
	got := bootstrapWorldCandidates("assets", roots)
	want := []string{
		filepath.Clean(`C:\srv\mdt-server\assets\bootstrap-world.bin`),
		filepath.Clean(`C:\srv\mdt-server\bin\assets\bootstrap-world.bin`),
		filepath.Clean(`C:\srv\mdt-server\bin\bin\assets\bootstrap-world.bin`),
		filepath.Clean(`C:\srv\mdt-server\go-server\assets\bootstrap-world.bin`),
		filepath.Clean(`C:\srv\mdt-server\bin\go-server\assets\bootstrap-world.bin`),
		filepath.Clean(`C:\srv\mdt-server\bootstrap-world.bin`),
		filepath.Clean(`C:\srv\mdt-server\bin\bootstrap-world.bin`),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected bootstrap candidates:\nwant=%#v\ngot=%#v", want, got)
	}
}

func TestBootstrapWorldCandidatesPreferAbsoluteRuntimeAssetsDir(t *testing.T) {
	roots := []string{filepath.Clean(`C:\srv\mdt-server\bin`)}
	got := bootstrapWorldCandidates(filepath.Clean(`D:\runtime\assets`), roots)
	if len(got) == 0 || got[0] != filepath.Clean(`D:\runtime\assets\bootstrap-world.bin`) {
		t.Fatalf("expected absolute runtime assets candidate first, got %#v", got)
	}
}
