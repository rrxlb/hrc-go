package utils

// XP requirement grows by 20% compounded per prestige
func GetXPForLevel(level int, prestige int) int64 {
	rank, ok := Ranks[level]
	if !ok {
		return 0
	}
	base := int64(rank.XPRequired)
	if prestige <= 0 {
		return base
	}
	// multiply by 1.2^prestige
	val := float64(base)
	for i := 0; i < prestige; i++ {
		val *= 1.2
	}
	return int64(val)
}

// GetUserLevel returns highest level achieved at given xp and prestige
func GetUserLevel(xp int64, prestige int) int {
	level := 0
	for l := 0; l < len(Ranks); l++ {
		req := GetXPForLevel(l, prestige)
		if xp >= req {
			level = l
		} else {
			break
		}
	}
	return level
}
