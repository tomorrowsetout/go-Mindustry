// Package plugin implements the YF-compatible extension system.
//
// It mirrors the 月见草YF框架 module model: plugins live under
// config/yzf/plugins/<name>/module.hjson (flat layout) and
// config/yzf/modules/<author>/<id>/module.hjson (namespaced layout),
// share the same metadata schema, dependency resolution rules and
// per-module config store, and expose the same `yzf` API surface to
// scripts so that plugins written for the Java framework can run on
// this server with minimal changes.
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	hjson "github.com/hjson/hjson-go/v4"
)

// Meta mirrors YZFModuleMeta field-for-field so existing module.hjson
// files load without modification.
type Meta struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Author       string   `json:"author"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	Main         string   `json:"main"`
	Runtime      string   `json:"runtime"`
	Enabled      bool     `json:"enabled"`
	Hidden       bool     `json:"hidden"`
	RequiresArgs bool     `json:"requiresArgs"`
	Category     string   `json:"category"`
	Permission   string   `json:"permission"`
	Tags         []string `json:"tags"`
	Depends      []string `json:"depends"`
	SoftDepends  []string `json:"softDepends"`
	JVMArgs      []string `json:"jvmArgs"`
	ProgramArgs  []string `json:"programArgs"`
	MemoryMin    string   `json:"memoryMin"`
	MemoryMax    string   `json:"memoryMax"`
	LoadType     string   `json:"loadType"`
	// Source records which directory the definition was scanned from
	// ("modules" or "plugins"); it mirrors YZFModuleMeta._source.
	Source string `json:"-"`
}

// DefaultMeta matches the defaults applied by YZFModuleMeta /
// YZFModuleLoader.loadModule.
func DefaultMeta() Meta {
	return Meta{
		Author:   "unknown",
		Version:  "0.1.0",
		Main:     "scripts/main.js",
		Runtime:  "js",
		Enabled:  true,
		Category: "Runtime",
		LoadType: "module",
		Source:   "modules",
	}
}

// Definition mirrors YZFModuleDefinition: metadata plus resolved paths.
type Definition struct {
	Meta       Meta
	Root       string   // module root directory
	MetaFile   string   // module.hjson / module.json path
	ScriptsDir string   // <root>/scripts
	DataDir    string   // <root>/data
	CacheDir   string   // <root>/cache
	MainScript string   // <root>/<meta.main>
	Scripts    []string // collected script files (main first when present)
}

// FullID returns "author/id", the canonical dependency key.
func (d *Definition) FullID() string { return d.Meta.Author + "/" + d.Meta.ID }

// HasMain reports whether the declared main script exists.
func (d *Definition) HasMain() bool {
	if d.MainScript == "" {
		return false
	}
	info, err := os.Stat(d.MainScript)
	return err == nil && !info.IsDir()
}

// scriptExtensions mirrors YZFModuleLoader.scriptExtensions.
var scriptExtensions = map[string]bool{
	"js": true, "mjs": true, "kt": true, "kts": true,
	"yfs": true, "java": true, "node": true, "jar": true,
}

// SupportedRuntimes mirrors YZFModuleLoader.supportedRuntimes plus the
// Go-native runtime this server adds.
func SupportedRuntimes() []string { return []string{"js", "node", "java", "kt", "kts", "go"} }

// LoadModule mirrors YZFModuleLoader.loadModule: resolve the metadata
// file, apply field defaults, and collect script files.
func LoadModule(root string) (*Definition, error) {
	metaFile := resolveMetaFile(root)
	if metaFile == "" {
		return nil, nil // not a module directory
	}
	meta, err := readMeta(metaFile)
	if err != nil {
		return nil, fmt.Errorf("读取模块元数据失败 %s: %w", metaFile, err)
	}
	base := filepath.Base(root)
	parent := filepath.Base(filepath.Dir(root))
	if strings.TrimSpace(meta.ID) == "" {
		meta.ID = base
	}
	if strings.TrimSpace(meta.Name) == "" {
		meta.Name = base
	}
	if strings.TrimSpace(meta.Author) == "" {
		if parent == "." || parent == string(os.PathSeparator) {
			meta.Author = "unknown"
		} else {
			meta.Author = parent
		}
	}
	if strings.TrimSpace(meta.Main) == "" {
		meta.Main = "scripts/main.js"
	}
	if strings.TrimSpace(meta.Runtime) == "" {
		meta.Runtime = "js"
	}

	def := &Definition{
		Meta:       *meta,
		Root:       root,
		MetaFile:   metaFile,
		ScriptsDir: filepath.Join(root, "scripts"),
		DataDir:    filepath.Join(root, "data"),
		CacheDir:   filepath.Join(root, "cache"),
		MainScript: filepath.Join(root, filepath.FromSlash(meta.Main)),
	}
	def.Scripts = collectScripts(def.ScriptsDir)
	if def.HasMain() && !containsString(def.Scripts, def.MainScript) {
		def.Scripts = append([]string{def.MainScript}, def.Scripts...)
	}
	return def, nil
}

func resolveMetaFile(root string) string {
	hjsonPath := filepath.Join(root, "module.hjson")
	if fileExists(hjsonPath) {
		return hjsonPath
	}
	jsonPath := filepath.Join(root, "module.json")
	if fileExists(jsonPath) {
		return jsonPath
	}
	return ""
}

func readMeta(path string) (*Meta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// hjson accepts plain JSON as well, so a single parser covers both
	// module.hjson and module.json.
	var decoded map[string]any
	if err := hjson.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	meta := DefaultMeta()
	if v, ok := decoded["id"].(string); ok {
		meta.ID = v
	}
	if v, ok := decoded["name"].(string); ok {
		meta.Name = v
	}
	if v, ok := decoded["author"].(string); ok {
		meta.Author = v
	}
	if v, ok := decoded["description"].(string); ok {
		meta.Description = v
	}
	if v, ok := decoded["version"].(string); ok {
		meta.Version = v
	}
	if v, ok := decoded["main"].(string); ok {
		meta.Main = v
	}
	if v, ok := decoded["runtime"].(string); ok {
		meta.Runtime = v
	}
	if v, ok := decoded["enabled"].(bool); ok {
		meta.Enabled = v
	}
	if v, ok := decoded["hidden"].(bool); ok {
		meta.Hidden = v
	}
	if v, ok := decoded["requiresArgs"].(bool); ok {
		meta.RequiresArgs = v
	}
	if v, ok := decoded["category"].(string); ok {
		meta.Category = v
	}
	if v, ok := decoded["permission"].(string); ok {
		meta.Permission = v
	}
	meta.Tags = readStringArray(decoded, "tags")
	meta.Depends = readStringArray(decoded, "depends")
	meta.SoftDepends = readStringArray(decoded, "softDepends")
	meta.JVMArgs = readStringArray(decoded, "jvmArgs")
	meta.ProgramArgs = readStringArray(decoded, "programArgs")
	if v, ok := decoded["memoryMin"].(string); ok {
		meta.MemoryMin = v
	} else if v, ok := decoded["minHeap"].(string); ok {
		meta.MemoryMin = v
	}
	if v, ok := decoded["memoryMax"].(string); ok {
		meta.MemoryMax = v
	} else if v, ok := decoded["maxHeap"].(string); ok {
		meta.MemoryMax = v
	}
	if v, ok := decoded["loadType"].(string); ok {
		meta.LoadType = v
	}
	return &meta, nil
}

func readStringArray(decoded map[string]any, key string) []string {
	raw, ok := decoded[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func collectScripts(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		if scriptExtensions[ext] {
			out = append(out, path)
		}
		return nil
	})
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

// metaJSON is used when a runtime needs the metadata as JSON (Go RPC
// init handshake).
func (d *Definition) metaJSON() string {
	payload, err := json.Marshal(d.Meta)
	if err != nil {
		return "{}"
	}
	return string(payload)
}
