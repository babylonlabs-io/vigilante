package version

import (
	"runtime/debug"
)

// version set at build-time
var version = "main"

func CommitInfo() (string, string) {
	hash, timestamp := "unknown", "unknown"
	hashLen := 7

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return hash, timestamp
	}

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) < hashLen {
				hashLen = len(s.Value)
			}
			hash = s.Value[:hashLen]
		case "vcs.time":
			timestamp = s.Value
		}
	}

	return hash, timestamp
}

// Version returns the version
func Version() string {
	return version
}
