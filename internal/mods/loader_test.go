package mods

import "testing"

func TestModLoaderLoadTypes(t *testing.T) {
	ml := NewModLoader()
	ml.SetEnabled(true)

	cases := []struct {
		path string
		typ  string
	}{
		{path: "mods/A.jar", typ: "java"},
		{path: "mods/b.js", typ: "js"},
		{path: "mods/c.go", typ: "go"},
		{path: "mods/d.node", typ: "node"},
		{path: "mods/E.NODE", typ: "node"},
	}

	for _, tc := range cases {
		if err := ml.Load(tc.path); err != nil {
			t.Fatalf("load %s failed: %v", tc.path, err)
		}
		mod, err := ml.GetMod(lastPathBase(tc.path))
		if err != nil {
			t.Fatalf("get mod %s failed: %v", tc.path, err)
		}
		if mod.DataType != tc.typ {
			t.Fatalf("expected type %s for %s, got %s", tc.typ, tc.path, mod.DataType)
		}
	}
}

func TestModLoaderDisabledAndUnsupported(t *testing.T) {
	ml := NewModLoader()

	if err := ml.Load("mods/x.js"); err == nil {
		t.Fatalf("expected error when mods disabled")
	}

	ml.SetEnabled(true)
	if err := ml.Load("mods/x.txt"); err == nil {
		t.Fatalf("expected unsupported type error")
	}
}

func lastPathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
