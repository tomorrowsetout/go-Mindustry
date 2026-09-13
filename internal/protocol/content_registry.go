package protocol

import (
	"strings"
	"sync"
)

// ContentRegistry stores content by type/id and provides TypeIO lookups.
type ContentRegistry struct {
	mu sync.RWMutex

	contents map[ContentType]map[int16]Content

	items       map[int16]Item
	liquids     map[int16]Liquid
	blocks      map[int16]Block
	unitTypes   map[int16]UnitType
	bulletTypes map[int16]BulletType
	status      map[int16]StatusEffect
	weather     map[int16]Weather
	effects     map[int16]Effect
	sounds      map[int16]Sound

	teams    map[byte]Team
	commands map[int16]UnitCommand
	stances  map[int16]UnitStance
}

func NewContentRegistry() *ContentRegistry {
	return &ContentRegistry{
		contents:    map[ContentType]map[int16]Content{},
		items:       map[int16]Item{},
		liquids:     map[int16]Liquid{},
		blocks:      map[int16]Block{},
		unitTypes:   map[int16]UnitType{},
		bulletTypes: map[int16]BulletType{},
		status:      map[int16]StatusEffect{},
		weather:     map[int16]Weather{},
		effects:     map[int16]Effect{},
		sounds:      map[int16]Sound{},
		teams:       map[byte]Team{},
		commands:    map[int16]UnitCommand{},
		stances:     map[int16]UnitStance{},
	}
}

func (r *ContentRegistry) RegisterContent(c Content) {
	if c == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.contents[c.ContentType()]
	if m == nil {
		m = map[int16]Content{}
		r.contents[c.ContentType()] = m
	}
	m[c.ID()] = c
}

func (r *ContentRegistry) RegisterItem(i Item) {
	if i == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[i.ID()] = i
	r.ensureContent(ContentItem)[i.ID()] = i
}

func (r *ContentRegistry) RegisterLiquid(l Liquid) {
	if l == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.liquids[l.ID()] = l
	r.ensureContent(ContentLiquid)[l.ID()] = l
}

func (r *ContentRegistry) RegisterBlock(b Block) {
	if b == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blocks[b.ID()] = b
	r.ensureContent(ContentBlock)[b.ID()] = b
}

func (r *ContentRegistry) RegisterUnitType(u UnitType) {
	if u == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unitTypes[u.ID()] = u
	r.ensureContent(ContentUnit)[u.ID()] = u
}

func (r *ContentRegistry) RegisterBulletType(b BulletType) {
	if b == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bulletTypes[b.ID()] = b
	r.ensureContent(ContentBullet)[b.ID()] = b
}

func (r *ContentRegistry) RegisterStatusEffect(s StatusEffect) {
	if s == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status[s.ID()] = s
	r.ensureContent(ContentStatus)[s.ID()] = s
}

func (r *ContentRegistry) RegisterWeather(w Weather) {
	if w == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.weather[w.ID()] = w
	r.ensureContent(ContentWeather)[w.ID()] = w
}

func (r *ContentRegistry) RegisterEffect(e Effect) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.effects[e.ID] = e
}

func (r *ContentRegistry) RegisterSound(s Sound) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sounds[s.ID] = s
}

func (r *ContentRegistry) RegisterTeam(t Team) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.teams[t.ID] = t
}

func (r *ContentRegistry) RegisterUnitCommand(c UnitCommand) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[c.ID] = c
}

func (r *ContentRegistry) RegisterUnitStance(s UnitStance) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stances[s.ID] = s
}

func (r *ContentRegistry) Get(t ContentType, id int16) Content {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if m := r.contents[t]; m != nil {
		return m[id]
	}
	return nil
}

func (r *ContentRegistry) Block(id int16) Block {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.blocks[id]
}

func (r *ContentRegistry) Item(id int16) Item {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.items[id]
}

func (r *ContentRegistry) Liquid(id int16) Liquid {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.liquids[id]
}

func (r *ContentRegistry) UnitType(id int16) UnitType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.unitTypes[id]
}

func (r *ContentRegistry) BulletType(id int16) BulletType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bulletTypes[id]
}

func (r *ContentRegistry) StatusEffect(id int16) StatusEffect {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status[id]
}

func (r *ContentRegistry) Weather(id int16) Weather {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.weather[id]
}

func (r *ContentRegistry) Effect(id int16) Effect {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.effects[id]
}

func (r *ContentRegistry) Sound(id int16) Sound {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sounds[id]
}

func (r *ContentRegistry) Team(id byte) Team {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if t, ok := r.teams[id]; ok {
		return t
	}
	return Team{ID: id}
}

func (r *ContentRegistry) UnitCommand(id int16) UnitCommand {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.commands[id]; ok {
		return c
	}
	return UnitCommand{ID: id}
}

func (r *ContentRegistry) UnitStance(id int16) UnitStance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.stances[id]; ok {
		return s
	}
	return UnitStance{ID: id}
}

// Context builds a TypeIOContext wired to this registry.
func (r *ContentRegistry) Context() *TypeIOContext {
	return &TypeIOContext{
		Content:            r,
		BlockLookup:        r.Block,
		ItemLookup:         r.Item,
		LiquidLookup:       r.Liquid,
		UnitTypeLookup:     r.UnitType,
		BulletTypeLookup:   r.BulletType,
		StatusEffectLookup: r.StatusEffect,
		WeatherLookup:      r.Weather,
		EffectLookup:       r.Effect,
		SoundLookup:        r.Sound,
		TeamLookup:         r.Team,
		UnitCommandLookup:  r.UnitCommand,
		UnitStanceLookup:   r.UnitStance,
	}
}

func (r *ContentRegistry) ensureContent(t ContentType) map[int16]Content {
	m := r.contents[t]
	if m == nil {
		m = map[int16]Content{}
		r.contents[t] = m
	}
	return m
}

func (r *ContentRegistry) IterateBlocks(fn func(Block) bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, block := range r.blocks {
		if !fn(block) {
			break
		}
	}
}

func (r *ContentRegistry) IterateUnitTypes(fn func(UnitType) bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, unit := range r.unitTypes {
		if !fn(unit) {
			break
		}
	}
}

func (r *ContentRegistry) SnapshotContentNames() map[ContentType]map[int16]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[ContentType]map[int16]string, len(r.contents))
	for typ, items := range r.contents {
		if len(items) == 0 {
			continue
		}
		copied := make(map[int16]string, len(items))
		for id, content := range items {
			if content == nil {
				continue
			}
			name := strings.TrimSpace(content.Name())
			if name == "" {
				continue
			}
			copied[id] = name
		}
		if len(copied) > 0 {
			out[typ] = copied
		}
	}
	return out
}
