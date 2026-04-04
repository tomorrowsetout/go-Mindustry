package protocol

import "strings"

type unitEntityLayout struct {
	classID      byte
	revision     int16
	baseRotation bool
	payloads     bool
	building     bool
	timedKill    bool
}

var defaultUnitEntityLayout = unitEntityLayout{
	classID:  3,
	revision: 7,
}

func unitEntityLayoutByClassID(classID byte) (unitEntityLayout, bool) {
	switch classID {
	case 0:
		return unitEntityLayout{classID: 0, revision: 3}, true // alpha
	case 2:
		return unitEntityLayout{classID: 2, revision: 7}, true // block
	case 3:
		return unitEntityLayout{classID: 3, revision: 7}, true // flare family
	case 4:
		return unitEntityLayout{classID: 4, revision: 7, baseRotation: true}, true // mace family
	case 5:
		return unitEntityLayout{classID: 5, revision: 5, payloads: true}, true // mega family
	case 16:
		return unitEntityLayout{classID: 16, revision: 6}, true // mono
	case 17:
		return unitEntityLayout{classID: 17, revision: 5, baseRotation: true}, true // nova
	case 18:
		return unitEntityLayout{classID: 18, revision: 5}, true // poly
	case 19:
		return unitEntityLayout{classID: 19, revision: 3, baseRotation: true}, true // pulsar
	case 20:
		return unitEntityLayout{classID: 20, revision: 7}, true // risso family
	case 21:
		return unitEntityLayout{classID: 21, revision: 6}, true // spiroct
	case 23:
		return unitEntityLayout{classID: 23, revision: 6, payloads: true}, true // quad
	case 24:
		return unitEntityLayout{classID: 24, revision: 7}, true // corvus family
	case 26:
		return unitEntityLayout{classID: 26, revision: 5, payloads: true}, true // oct
	case 29:
		return unitEntityLayout{classID: 29, revision: 3}, true // arkyid
	case 30:
		return unitEntityLayout{classID: 30, revision: 3}, true // beta
	case 31:
		return unitEntityLayout{classID: 31, revision: 3}, true // gamma
	case 32:
		return unitEntityLayout{classID: 32, revision: 3, baseRotation: true}, true // quasar
	case 33:
		return unitEntityLayout{classID: 33, revision: 3}, true // toxopid
	case 36:
		return unitEntityLayout{classID: 36, revision: 1, payloads: true, building: true}, true // manifold
	case 39:
		return unitEntityLayout{classID: 39, revision: 1, timedKill: true}, true // missile
	case 45:
		return unitEntityLayout{classID: 45, revision: 0}, true // elude
	case 46:
		return unitEntityLayout{classID: 46, revision: 0}, true // latum
	case 47:
		return unitEntityLayout{classID: 47, revision: 0}, true // renale
	case 40:
		return unitEntityLayout{classID: 40, revision: 1}, true // vanquish family
	case 43:
		return unitEntityLayout{classID: 43, revision: 0}, true // stell family
	default:
		return unitEntityLayout{}, false
	}
}

func unitEntityLayoutByName(name string) (unitEntityLayout, bool) {
	switch normalizeUnitEntityName(name) {
	case "alpha":
		return unitEntityLayoutByClassID(0)
	case "beta":
		return unitEntityLayoutByClassID(30)
	case "gamma":
		return unitEntityLayoutByClassID(31)
	case "block":
		return unitEntityLayoutByClassID(2)
	case "flare", "eclipse", "horizon", "zenith", "antumbra", "avert", "obviate":
		return unitEntityLayoutByClassID(3)
	case "mono":
		return unitEntityLayoutByClassID(16)
	case "poly":
		return unitEntityLayoutByClassID(18)
	case "mace", "dagger", "crawler", "fortress", "scepter", "reign", "vela":
		return unitEntityLayoutByClassID(4)
	case "nova":
		return unitEntityLayoutByClassID(17)
	case "pulsar":
		return unitEntityLayoutByClassID(19)
	case "quasar":
		return unitEntityLayoutByClassID(32)
	case "mega", "evoke", "incite", "emanate", "quell", "disrupt":
		return unitEntityLayoutByClassID(5)
	case "quad":
		return unitEntityLayoutByClassID(23)
	case "oct":
		return unitEntityLayoutByClassID(26)
	case "risso", "minke", "bryde", "sei", "omura", "retusa", "oxynoe", "cyerce", "aegires", "navanax":
		return unitEntityLayoutByClassID(20)
	case "corvus", "atrax", "merui", "cleroi", "anthicus", "tecta", "collaris":
		return unitEntityLayoutByClassID(24)
	case "spiroct":
		return unitEntityLayoutByClassID(21)
	case "arkyid":
		return unitEntityLayoutByClassID(29)
	case "toxopid":
		return unitEntityLayoutByClassID(33)
	case "manifold", "assemblydrone", "assembly-drone":
		return unitEntityLayoutByClassID(36)
	case "missile":
		return unitEntityLayoutByClassID(39)
	case "elude":
		return unitEntityLayoutByClassID(45)
	case "latum":
		return unitEntityLayoutByClassID(46)
	case "renale":
		return unitEntityLayoutByClassID(47)
	case "stell", "locus", "precept":
		return unitEntityLayoutByClassID(43)
	case "vanquish", "conquer":
		return unitEntityLayoutByClassID(40)
	default:
		return unitEntityLayout{}, false
	}
}

func normalizeUnitEntityName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "-")
	return name
}

func (u *UnitEntitySync) layout() unitEntityLayout {
	if u != nil && u.ClassIDSet {
		if layout, ok := unitEntityLayoutByClassID(u.ClassIDValue); ok {
			return layout
		}
		return unitEntityLayout{classID: u.ClassIDValue}
	}
	return defaultUnitEntityLayout
}

func (u *UnitEntitySync) ApplyLayoutByName(name string) {
	if u == nil {
		return
	}
	layout, ok := unitEntityLayoutByName(name)
	if !ok {
		if !u.ClassIDSet {
			u.ClassIDValue = defaultUnitEntityLayout.classID
			u.ClassIDSet = true
		}
		layout = u.layout()
	} else {
		u.ClassIDValue = layout.classID
		u.ClassIDSet = true
	}
	if layout.baseRotation && u.BaseRotation == 0 {
		u.BaseRotation = u.Rotation
	}
}
