package config

import (
	"testing"
)

func TestBackendType_Constants(t *testing.T) {
	// Test that constants have expected values
	if BackendTypeBabylon != "babylon" {
		t.Errorf("BackendTypeBabylon = %v, want babylon", BackendTypeBabylon)
	}
	if BackendTypeEthereum != "ethereum" {
		t.Errorf("BackendTypeEthereum = %v, want ethereum", BackendTypeEthereum)
	}
}

func TestBackendType_Comparison(t *testing.T) {
	// Test that enum comparisons work correctly
	if BackendTypeBabylon == BackendTypeEthereum {
		t.Error("BackendTypeBabylon should not equal BackendTypeEthereum")
	}

	// Test that we can use in switch statements
	var result string
	switch BackendTypeBabylon {
	case BackendTypeBabylon:
		result = "babylon"
	case BackendTypeEthereum:
		result = "ethereum"
	default:
		result = "unknown"
	}

	if result != "babylon" {
		t.Errorf("Switch statement failed, got %v, want babylon", result)
	}
}

func TestReporterConfig_Validate_BackendType(t *testing.T) {
	tests := []struct {
		name        string
		backendType BackendType
		expectErr   bool
	}{
		{"Valid Babylon", BackendTypeBabylon, false},
		{"Valid Ethereum", BackendTypeEthereum, false},
		{"Invalid type", BackendType("invalid"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultReporterConfig()
			cfg.BackendType = tt.backendType

			err := cfg.Validate()
			if tt.expectErr && err == nil {
				t.Error("Expected validation error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}
