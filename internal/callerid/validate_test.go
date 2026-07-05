package callerid

import (
	"testing"

	"github.com/jasondostal/tresbbs/domain"
)

func TestValidateCallerID_Disabled(t *testing.T) {
	config := &domain.Config{
		EnableCallerID: false,
	}

	result := ValidateCallerID(config, "", nil)
	if result.Status != StatusAllowed {
		t.Errorf("Expected StatusAllowed when disabled, got %v", result.Status)
	}
}

func TestValidateCallerID_AllowNoInfo(t *testing.T) {
	config := &domain.Config{
		EnableCallerID:  true,
		BlockNoCallerID: false,
	}

	result := ValidateCallerID(config, "", nil)
	if result.Status != StatusAllowed {
		t.Errorf("Expected StatusAllowed for no info when not blocking, got %v", result.Status)
	}
}

func TestValidateCallerID_BlockNoInfo(t *testing.T) {
	config := &domain.Config{
		EnableCallerID:  true,
		BlockNoCallerID: true,
	}

	result := ValidateCallerID(config, "", nil)
	if result.Status != StatusBlockedNoInfo {
		t.Errorf("Expected StatusBlockedNoInfo, got %v", result.Status)
	}
}

func TestValidateCallerID_BlockBlocked(t *testing.T) {
	config := &domain.Config{
		EnableCallerID:  true,
		BlockBlockedCID: true,
	}

	tests := []string{"PRIVATE", "ANONYMOUS", "BLOCKED"}
	for _, callerID := range tests {
		result := ValidateCallerID(config, callerID, nil)
		if result.Status != StatusBlockedCID {
			t.Errorf("Expected StatusBlockedCID for %q, got %v", callerID, result.Status)
		}
	}
}

func TestValidateCallerID_AllowBlocked(t *testing.T) {
	config := &domain.Config{
		EnableCallerID:  true,
		BlockBlockedCID: false,
	}

	result := ValidateCallerID(config, "PRIVATE", nil)
	if result.Status != StatusAllowed {
		t.Errorf("Expected StatusAllowed when not blocking blocked CID, got %v", result.Status)
	}
}

func TestValidateCallerID_TwittedNumber(t *testing.T) {
	config := &domain.Config{
		EnableCallerID: true,
	}

	twitted := []string{"5551234567", "9999999999"}

	tests := []struct {
		callerID string
		expected CallerIDStatus
	}{
		{"555-123-4567", StatusBlockedTwit},
		{"(555) 123-4567", StatusBlockedTwit},
		{"555.123.4567", StatusBlockedTwit},
		{"5551234567", StatusBlockedTwit},
		{"999-999-9999", StatusBlockedTwit},
		{"555-867-5309", StatusAllowed},
	}

	for _, tt := range tests {
		result := ValidateCallerID(config, tt.callerID, twitted)
		if result.Status != tt.expected {
			t.Errorf("ValidateCallerID(%q) = %v, want %v", tt.callerID, result.Status, tt.expected)
		}
	}
}

func TestValidateCallerID_ValidNumber(t *testing.T) {
	config := &domain.Config{
		EnableCallerID: true,
	}

	result := ValidateCallerID(config, "555-867-5309", nil)
	if result.Status != StatusAllowed {
		t.Errorf("Expected StatusAllowed for valid number, got %v", result.Status)
	}
	if result.Number != "555-867-5309" {
		t.Errorf("Expected caller ID returned, got %q", result.Number)
	}
}

func TestFormatCallerID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "Unknown"},
		{"5558675309", "(555) 867-5309"},
		{"555-867-5309", "(555) 867-5309"},
		{"+15558675309", "+15558675309"}, // 11 digits, no formatting
		{"PRIVATE", "PRIVATE"},
	}

	for _, tt := range tests {
		result := FormatCallerID(tt.input)
		if result != tt.expected {
			t.Errorf("FormatCallerID(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestNormalizePhoneNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"555-867-5309", "5558675309"},
		{"(555) 867-5309", "5558675309"},
		{"555.867.5309", "5558675309"},
		{"+1 555 867 5309", "15558675309"},
		{"abc555def867ghi5309", "5558675309"},
	}

	for _, tt := range tests {
		result := normalizePhoneNumber(tt.input)
		if result != tt.expected {
			t.Errorf("normalizePhoneNumber(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func BenchmarkValidateCallerID(b *testing.B) {
	config := &domain.Config{
		EnableCallerID:  true,
		BlockNoCallerID: true,
		BlockBlockedCID: true,
	}
	twitted := []string{"5551234567", "9999999999", "8887776666"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidateCallerID(config, "555-867-5309", twitted)
	}
}
