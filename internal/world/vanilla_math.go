package world

import "math"

const (
	arcSinBits  = 14
	arcSinMask  = ^(-1 << arcSinBits)
	arcSinCount = arcSinMask + 1

	arcRadFull    = math.Pi * 2
	arcDegFull    = 360.0
	arcRadToIndex = float32(arcSinCount) / float32(arcRadFull)
	arcDegToIndex = float32(arcSinCount) / float32(arcDegFull)
)

var arcSinTable = initArcSinTable()

func initArcSinTable() [arcSinCount]float32 {
	var table [arcSinCount]float32
	for i := range table {
		table[i] = float32(math.Sin((float64(i) + 0.5) / float64(arcSinCount) * arcRadFull))
	}
	for i := 0; i < 360; i += 90 {
		table[int(float32(i)*arcDegToIndex)&arcSinMask] = float32(math.Sin(float64(i) * math.Pi / 180))
	}
	return table
}

func arcSin(rad float32) float32 {
	return arcSinTable[int(rad*arcRadToIndex)&arcSinMask]
}

func arcCos(rad float32) float32 {
	return arcSinTable[int((rad+float32(math.Pi)/2)*arcRadToIndex)&arcSinMask]
}

func arcSinDeg(deg float32) float32 {
	return arcSinTable[int(deg*arcDegToIndex)&arcSinMask]
}

func arcCosDeg(deg float32) float32 {
	return arcSinTable[int((deg+90)*arcDegToIndex)&arcSinMask]
}

func arcSinScaled(t, scl, mag float32) float32 {
	if scl == 0 {
		return 0
	}
	return arcSin(t/scl) * mag
}

func arcCosScaled(t, scl, mag float32) float32 {
	if scl == 0 {
		return 0
	}
	return arcCos(t/scl) * mag
}
