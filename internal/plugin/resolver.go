package plugin

// Resolution mirrors YZFDependencyResolver.Resolution.
type Resolution struct {
	Ordered  []*Definition
	Errors   []string
	Warnings []string
}

// Resolver implements the same DFS topological sort as
// YZFDependencyResolver: hard dependency misses are errors, soft
// dependency misses are warnings, cycles are reported with the full
// dependency chain, and every visited definition ends up in Ordered
// (missing-dependency definitions are included so callers can decide
// whether to skip them).
type Resolver struct {
	byFullID map[string]*Definition
	temp     map[string]bool
	perm     map[string]bool
	ordered  []*Definition
	errors   []string
	warnings []string
}

// Resolve sorts discovered definitions into load order.
func Resolve(discovered []*Definition) Resolution {
	r := &Resolver{
		byFullID: make(map[string]*Definition, len(discovered)),
		temp:     make(map[string]bool),
		perm:     make(map[string]bool),
	}
	for _, def := range discovered {
		r.byFullID[def.FullID()] = def
	}
	for _, def := range discovered {
		r.visit(def, nil)
	}
	return Resolution{
		Ordered:  r.ordered,
		Errors:   r.errors,
		Warnings: r.warnings,
	}
}

func (r *Resolver) visit(def *Definition, stack []string) {
	id := def.FullID()
	if r.perm[id] {
		return
	}
	if r.temp[id] {
		cycle := append(append([]string{}, stack...), id)
		r.errors = append(r.errors, "检测到循环依赖: "+joinStrings(cycle, " -> "))
		return
	}
	r.temp[id] = true
	stack = append(stack, id)
	for _, depend := range def.Meta.Depends {
		target := r.byFullID[depend]
		if target == nil {
			r.errors = append(r.errors, "模块 "+id+" 缺少硬依赖 "+depend)
		} else {
			r.visit(target, stack)
		}
	}
	for _, depend := range def.Meta.SoftDepends {
		target := r.byFullID[depend]
		if target == nil {
			r.warnings = append(r.warnings, "模块 "+id+" 缺少软依赖 "+depend)
		} else {
			r.visit(target, stack)
		}
	}
	stack = stack[:len(stack)-1]
	delete(r.temp, id)
	r.perm[id] = true
	if !containsDefinition(r.ordered, def) {
		r.ordered = append(r.ordered, def)
	}
}

func containsDefinition(list []*Definition, target *Definition) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

func joinStrings(list []string, sep string) string {
	out := ""
	for i, s := range list {
		if i > 0 {
			out += sep
		}
		out += s
	}
	return out
}
