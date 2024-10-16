package monitor

func minBTCHeight(h1, h2 uint32) uint32 {
	if h1 > h2 {
		return h2
	}

	return h1
}
