package testutil

import (
	"fmt"
	mrand "math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// Directory for storing lock files
const lockDir = "/tmp/testutil_ports"

// AllocateUniquePort tries to find an available TCP port on localhost
// by testing multiple random ports within a specified range.
func AllocateUniquePort(t *testing.T) int {
	randPort := func(base, spread int) int {
		return base + mrand.IntN(spread)
	}

	// Base port and spread range for port selection
	const (
		basePort  = 20000
		portRange = 30000
	)

	// Ensure lock directory exists
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		t.Fatalf("failed to create lock directory: %v", err)
	}

	// Try up to 10 times to find an available port
	for i := 0; i < 10; i++ {
		port := randPort(basePort, portRange)
		lockFile := filepath.Join(lockDir, fmt.Sprintf("%d.lock", port))

		// Try to create a lock file
		lock, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			continue // Port is likely in use
		}

		// Ensure the file is removed if we fail to bind the port
		defer lock.Close()
		defer os.Remove(lockFile)

		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue // Port is already in use
		}

		// Close the listener to release the port
		if err := listener.Close(); err != nil {
			continue
		}

		// Successfully found an available port
		return port
	}

	// If no available port was found, fail the test
	t.Fatalf("failed to find an available port in range %d-%d", basePort, basePort+portRange)

	return 0
}
