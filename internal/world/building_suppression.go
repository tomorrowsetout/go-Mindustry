package world

func (w *World) blockSupportsHealSuppressionLocked(blockID BlockID) bool {
	if w == nil || blockID == 0 {
		return false
	}
	switch normalizeBlockLookupName(w.blockNameByID(int16(blockID))) {
	case "buildturret", "mendprojector", "regenprojector", "unitrepairtower":
		return true
	default:
		return false
	}
}

func (w *World) isBuildingHealSuppressedLocked(build *Building) bool {
	if w == nil || build == nil || !w.blockSupportsHealSuppressionLocked(build.Block) {
		return false
	}
	return w.timeSec <= build.healSuppressionUntilSec
}

func (w *World) applyBuildingHealSuppressionLocked(build *Building, durationSec float32) bool {
	if w == nil || build == nil || durationSec <= 0 || !w.blockSupportsHealSuppressionLocked(build.Block) {
		return false
	}
	until := w.timeSec + durationSec
	if until <= build.healSuppressionUntilSec {
		return false
	}
	build.healSuppressionUntilSec = until
	return true
}
