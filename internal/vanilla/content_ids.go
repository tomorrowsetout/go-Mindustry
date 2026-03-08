package vanilla

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mdt-server/internal/protocol"
)

type ContentIDEntry struct {
	ID   int16  `json:"id"`
	Name string `json:"name"`
}

type ContentIDsFile struct {
	Blocks   []ContentIDEntry `json:"blocks"`
	Units    []ContentIDEntry `json:"units"`
	Items    []ContentIDEntry `json:"items"`
	Liquids  []ContentIDEntry `json:"liquids"`
	Statuses []ContentIDEntry `json:"statuses"`
	Weathers []ContentIDEntry `json:"weathers"`
	Bullets  []ContentIDEntry `json:"bullets"`
	Effects  []ContentIDEntry `json:"effects"`
	Sounds   []ContentIDEntry `json:"sounds"`
	Teams    []ContentIDEntry `json:"teams"`
	Commands []ContentIDEntry `json:"unit_commands"`
	Stances  []ContentIDEntry `json:"unit_stances"`
	// Logic IDs are not content IDs; they are logic lookup IDs from logicids.dat.
	LogicBlocks  []ContentIDEntry `json:"logic_blocks"`
	LogicUnits   []ContentIDEntry `json:"logic_units"`
	LogicItems   []ContentIDEntry `json:"logic_items"`
	LogicLiquids []ContentIDEntry `json:"logic_liquids"`
}

func GenerateContentIDs(repoRoot, outPath string) (*ContentIDsFile, error) {
	base := filepath.Join(repoRoot, "core", "src", "mindustry", "content")
	if st, err := os.Stat(base); err != nil || !st.IsDir() {
		base = filepath.Join(repoRoot, "..", "core", "src", "mindustry", "content")
	}
	assets := filepath.Join(repoRoot, "core", "assets")
	if st, err := os.Stat(assets); err != nil || !st.IsDir() {
		assets = filepath.Join(repoRoot, "..", "core", "assets")
	}

	out := &ContentIDsFile{}
	var err error

	out.Items, err = parseNamedNewEntries(filepath.Join(base, "Items.java"), `=\s*new\s+Item\s*\(\s*"([^"]+)"`)
	if err != nil {
		return nil, err
	}
	out.Liquids, err = parseNamedNewEntries(filepath.Join(base, "Liquids.java"), `=\s*new\s+Liquid\s*\(\s*"([^"]+)"`)
	if err != nil {
		return nil, err
	}
	out.Units, err = parseNamedNewEntries(filepath.Join(base, "UnitTypes.java"), `=\s*new\s+UnitType\s*\(\s*"([^"]+)"`)
	if err != nil {
		return nil, err
	}
	out.Blocks, err = parseNamedNewEntries(filepath.Join(base, "Blocks.java"), `=\s*new\s+[A-Za-z0-9_$.]+\s*\(\s*"([^"]+)"`)
	if err != nil {
		return nil, err
	}
	out.Statuses, err = parseNamedNewEntries(filepath.Join(base, "StatusEffects.java"), `=\s*new\s+StatusEffect\s*\(\s*"([^"]+)"`)
	if err != nil {
		return nil, err
	}
	out.Weathers, err = parseNamedNewEntries(filepath.Join(base, "Weathers.java"), `=\s*new\s+[A-Za-z0-9_$.]*Weather\s*\(\s*"([^"]+)"`)
	if err != nil {
		return nil, err
	}
	out.Bullets, err = parseBulletEntries(filepath.Join(base, "Bullets.java"))
	if err != nil {
		return nil, err
	}
	out.Effects, err = parseEffects(filepath.Join(base, "Fx.java"), filepath.Join(repoRoot, "core", "src", "mindustry", "logic", "LogicFx.java"))
	if err != nil {
		return nil, err
	}
	out.Sounds, err = parseSounds(filepath.Join(assets, "sounds"))
	if err != nil {
		return nil, err
	}
	out.Teams, err = parseTeams(filepath.Join(repoRoot, "core", "src", "mindustry", "game", "Team.java"))
	if err != nil {
		return nil, err
	}
	out.Commands, err = parseUnitCommands(filepath.Join(repoRoot, "core", "src", "mindustry", "ai", "UnitCommand.java"))
	if err != nil {
		return nil, err
	}
	out.Stances, err = parseUnitStances(
		filepath.Join(repoRoot, "core", "src", "mindustry", "ai", "UnitStance.java"),
		out.Items,
	)
	if err != nil {
		return nil, err
	}

	logicPath := filepath.Join(assets, "logicids.dat")
	if st, statErr := os.Stat(logicPath); statErr == nil && !st.IsDir() {
		if blocks, units, items, liquids, lerr := parseLogicIDs(logicPath); lerr == nil {
			out.LogicBlocks = blocks
			out.LogicUnits = units
			out.LogicItems = items
			out.LogicLiquids = liquids
		}
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(outPath, b, 0644); err != nil {
		return nil, err
	}
	return out, nil
}

func LoadContentIDs(path string) (*ContentIDsFile, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out ContentIDsFile
	if err := json.Unmarshal(src, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func ApplyContentIDs(reg *protocol.ContentRegistry, ids *ContentIDsFile) int {
	if reg == nil || ids == nil {
		return 0
	}
	total := 0
	for _, e := range ids.Items {
		reg.RegisterItem(namedContent{typ: protocol.ContentItem, id: e.ID, name: e.Name})
		total++
	}
	for _, e := range ids.Liquids {
		reg.RegisterLiquid(namedContent{typ: protocol.ContentLiquid, id: e.ID, name: e.Name})
		total++
	}
	for _, e := range ids.Blocks {
		reg.RegisterBlock(namedContent{typ: protocol.ContentBlock, id: e.ID, name: e.Name})
		total++
	}
	for _, e := range ids.Units {
		reg.RegisterUnitType(namedContent{typ: protocol.ContentUnit, id: e.ID, name: e.Name})
		total++
	}
	for _, e := range ids.Statuses {
		reg.RegisterStatusEffect(namedStatus{id: e.ID})
		total++
	}
	for _, e := range ids.Weathers {
		reg.RegisterWeather(namedContent{typ: protocol.ContentWeather, id: e.ID, name: e.Name})
		total++
	}
	for _, e := range ids.Bullets {
		reg.RegisterBulletType(namedContent{typ: protocol.ContentBullet, id: e.ID, name: e.Name})
		total++
	}
	for _, e := range ids.Effects {
		reg.RegisterEffect(protocol.Effect{ID: e.ID})
		total++
	}
	for _, e := range ids.Sounds {
		reg.RegisterSound(protocol.Sound{ID: e.ID})
		total++
	}
	for _, e := range ids.Teams {
		reg.RegisterTeam(protocol.Team{ID: byte(e.ID & 0xff)})
		total++
	}
	for _, e := range ids.Commands {
		reg.RegisterUnitCommand(protocol.UnitCommand{ID: e.ID})
		total++
	}
	for _, e := range ids.Stances {
		reg.RegisterUnitStance(protocol.UnitStance{ID: e.ID})
		total++
	}
	return total
}

type namedContent struct {
	typ protocol.ContentType
	id  int16
	name string
}

func (n namedContent) ContentType() protocol.ContentType { return n.typ }
func (n namedContent) ID() int16                         { return n.id }
func (n namedContent) Name() string                      { return n.name }

type namedStatus struct {
	id   int16
	name string
}

func (s namedStatus) ContentType() protocol.ContentType { return protocol.ContentStatus }
func (s namedStatus) ID() int16                         { return s.id }
func (s namedStatus) Dynamic() bool                     { return false }
func (s namedStatus) Name() string                      { return s.name }

func parseNamedNewEntries(path string, ctorPattern string) ([]ContentIDEntry, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`(?m)` + ctorPattern)
	ms := re.FindAllSubmatch(src, -1)
	out := make([]ContentIDEntry, 0, len(ms))
	seen := make(map[string]struct{}, len(ms))
	for _, m := range ms {
		name := strings.TrimSpace(strings.ToLower(string(m[1])))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, ContentIDEntry{
			ID:   int16(len(out)),
			Name: name,
		})
	}
	return out, nil
}

func parseBulletEntries(path string) ([]ContentIDEntry, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`(?m)\b([A-Za-z_]\w*)\s*=\s*new\s+[A-Za-z0-9_$.]*BulletType\s*\(`)
	ms := re.FindAllSubmatch(src, -1)
	out := make([]ContentIDEntry, 0, len(ms))
	seen := make(map[string]struct{}, len(ms))
	for _, m := range ms {
		name := strings.TrimSpace(strings.ToLower(string(m[1])))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, ContentIDEntry{
			ID:   int16(len(out)),
			Name: name,
		})
	}
	return out, nil
}

func parseEffects(paths ...string) ([]ContentIDEntry, error) {
	re := regexp.MustCompile(`(?m)\b([A-Za-z_]\w*)\s*=\s*new\s+[A-Za-z0-9_$.]*Effect\s*\(`)
	out := make([]ContentIDEntry, 0, 512)
	seen := make(map[string]struct{}, 512)
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		ms := re.FindAllSubmatch(src, -1)
		for _, m := range ms {
			name := strings.TrimSpace(strings.ToLower(string(m[1])))
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, ContentIDEntry{
				ID:   int16(len(out)),
				Name: name,
			})
		}
	}
	return out, nil
}

func parseSounds(soundsDir string) ([]ContentIDEntry, error) {
	out := make([]ContentIDEntry, 0, 512)
	seen := make(map[string]struct{}, 512)
	type item struct {
		rel  string
		name string
	}
	list := make([]item, 0, 512)

	if st, err := os.Stat(soundsDir); err != nil || !st.IsDir() {
		return out, nil
	}
	err := filepath.WalkDir(soundsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".ogg" && ext != ".wav" && ext != ".mp3" {
			return nil
		}
		rel, rerr := filepath.Rel(soundsDir, path)
		if rerr != nil {
			return nil
		}
		name := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(filepath.Base(path), ext)))
		if name == "" {
			return nil
		}
		list = append(list, item{
			rel:  filepath.ToSlash(rel),
			name: name,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Stable order by relative path gives deterministic IDs.
	sort.Slice(list, func(i, j int) bool {
		return list[i].rel < list[j].rel
	})
	for _, it := range list {
		if _, ok := seen[it.name]; ok {
			continue
		}
		seen[it.name] = struct{}{}
		out = append(out, ContentIDEntry{
			ID:   int16(len(out)),
			Name: it.name,
		})
	}
	return out, nil
}

func parseTeams(path string) ([]ContentIDEntry, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := make([]ContentIDEntry, 0, 256)
	for i := 0; i < 256; i++ {
		out = append(out, ContentIDEntry{
			ID:   int16(i),
			Name: fmt.Sprintf("team#%d", i),
		})
	}
	re := regexp.MustCompile(`new\s+Team\s*\(\s*(\d+)\s*,\s*"([^"]+)"`)
	ms := re.FindAllSubmatch(src, -1)
	for _, m := range ms {
		id, ok := parseInt16(string(m[1]))
		if !ok || id < 0 || id >= 256 {
			continue
		}
		out[id].Name = strings.ToLower(strings.TrimSpace(string(m[2])))
	}
	return out, nil
}

func parseUnitCommands(path string) ([]ContentIDEntry, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`new\s+UnitCommand\s*\(\s*"([^"]+)"`)
	ms := re.FindAllSubmatch(src, -1)
	out := make([]ContentIDEntry, 0, len(ms))
	seen := make(map[string]struct{}, len(ms))
	for _, m := range ms {
		name := strings.ToLower(strings.TrimSpace(string(m[1])))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, ContentIDEntry{
			ID:   int16(len(out)),
			Name: name,
		})
	}
	return out, nil
}

func parseUnitStances(path string, items []ContentIDEntry) ([]ContentIDEntry, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`new\s+UnitStance\s*\(\s*"([^"]+)"`)
	ms := re.FindAllSubmatch(src, -1)
	out := make([]ContentIDEntry, 0, len(ms)+len(items))
	seen := make(map[string]struct{}, len(ms)+len(items))
	for _, m := range ms {
		name := strings.ToLower(strings.TrimSpace(string(m[1])))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, ContentIDEntry{
			ID:   int16(len(out)),
			Name: name,
		})
	}
	// ItemUnitStance IDs are appended after vanilla base stances.
	for _, it := range items {
		name := "item-" + strings.ToLower(strings.TrimSpace(it.Name))
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, ContentIDEntry{
			ID:   int16(len(out)),
			Name: name,
		})
	}
	return out, nil
}

func parseInt16(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	sign := 1
	if strings.HasPrefix(s, "-") {
		sign = -1
		s = strings.TrimPrefix(s, "-")
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return sign * n, true
}

func parseLogicIDs(path string) (blocks, units, items, liquids []ContentIDEntry, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	off := 0
	readU16 := func() (int, error) {
		if off+2 > len(raw) {
			return 0, fmt.Errorf("unexpected EOF")
		}
		v := int(binary.BigEndian.Uint16(raw[off : off+2]))
		off += 2
		return v, nil
	}
	readUTF := func() (string, error) {
		n, e := readU16()
		if e != nil {
			return "", e
		}
		if off+n > len(raw) {
			return "", fmt.Errorf("unexpected EOF")
		}
		s := string(raw[off : off+n])
		off += n
		return strings.ToLower(strings.TrimSpace(s)), nil
	}
	readSeq := func() ([]ContentIDEntry, error) {
		n, e := readU16()
		if e != nil {
			return nil, e
		}
		out := make([]ContentIDEntry, 0, n)
		for i := 0; i < n; i++ {
			name, se := readUTF()
			if se != nil {
				return nil, se
			}
			out = append(out, ContentIDEntry{ID: int16(i), Name: name})
		}
		return out, nil
	}

	if blocks, err = readSeq(); err != nil {
		return nil, nil, nil, nil, err
	}
	if units, err = readSeq(); err != nil {
		return nil, nil, nil, nil, err
	}
	if items, err = readSeq(); err != nil {
		return nil, nil, nil, nil, err
	}
	if liquids, err = readSeq(); err != nil {
		return nil, nil, nil, nil, err
	}
	return blocks, units, items, liquids, nil
}
