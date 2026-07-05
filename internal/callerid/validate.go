// Package callerid provides Caller ID validation for tresbbs.
//
// In TriBBS 11.6+, Caller ID was used to screen incoming calls.
// The BBS could block calls from:
//   - Blocked caller ID numbers
//   - Missing caller ID info
//   - Out-of-area calls
//   - Specific "twitted" (banned) phone numbers
//
// In tresbbs (telnet/SSH), caller ID is simulated via IP address
// or can be disabled for non-modem connections.
package callerid

import (
	"fmt"
	"strings"

	"github.com/jasondostal/tresbbs/domain"
)

// CallerIDStatus represents the result of a caller ID check.
type CallerIDStatus int

const (
	StatusAllowed    CallerIDStatus = iota // Call is allowed
	StatusBlockedNoInfo                    // Blocked: no caller ID info
	StatusBlockedCID                       // Blocked: caller ID is blocked
	StatusBlockedTwit                      // Blocked: twitted phone number
)

// CallerIDResult contains the result of a caller ID check.
type CallerIDResult struct {
	Status  CallerIDStatus
	Number  string // The caller ID number (if available)
	Message string // User-facing message
}

// ValidateCallerID checks if a caller is allowed based on caller ID settings.
// In a real modem BBS, this would check the caller ID from the modem.
// In tresbbs (telnet/SSH), we simulate this based on configuration.
func ValidateCallerID(config *domain.Config, callerID string, twittedNumbers []string) CallerIDResult {
	// If caller ID is disabled, allow all calls
	if !config.EnableCallerID {
		return CallerIDResult{
			Status: StatusAllowed,
			Number: callerID,
		}
	}

	// Check for no caller ID info
	if callerID == "" || callerID == "O" || callerID == "P" {
		if config.BlockNoCallerID {
			return CallerIDResult{
				Status:  StatusBlockedNoInfo,
				Message: "Sorry no caller id info calls are not allowed on this board!",
			}
		}
		return CallerIDResult{Status: StatusAllowed}
	}

	// Check for blocked caller ID
	if callerID == "PRIVATE" || callerID == "ANONYMOUS" || callerID == "BLOCKED" {
		if config.BlockBlockedCID {
			return CallerIDResult{
				Status:  StatusBlockedCID,
				Message: "Sorry blocked caller id calls are not allowed on this board!",
			}
		}
		return CallerIDResult{Status: StatusAllowed}
	}

	// Check for twitted (banned) phone numbers
	for _, twit := range twittedNumbers {
		if normalizePhoneNumber(callerID) == normalizePhoneNumber(twit) {
			return CallerIDResult{
				Status:  StatusBlockedTwit,
				Message: "Access denied.",
			}
		}
	}

	// Caller ID is valid and allowed
	return CallerIDResult{
		Status: StatusAllowed,
		Number: callerID,
	}
}

// FormatCallerID formats a caller ID for display/logging.
func FormatCallerID(callerID string) string {
	if callerID == "" {
		return "Unknown"
	}

	// Format as (XXX) XXX-XXXX if 10 digits
	cleaned := normalizePhoneNumber(callerID)
	if len(cleaned) == 10 {
		return fmt.Sprintf("(%s) %s-%s", cleaned[:3], cleaned[3:6], cleaned[6:])
	}

	return callerID
}

// normalizePhoneNumber strips non-digit characters from a phone number.
func normalizePhoneNumber(phone string) string {
	var result strings.Builder
	for _, c := range phone {
		if c >= '0' && c <= '9' {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// LogCallerIDEvent logs a caller ID related event.
func FormatCallerIDLog(callerID string, status CallerIDStatus) string {
	switch status {
	case StatusBlockedNoInfo:
		return fmt.Sprintf("Caller ID blocked (no info): %s", callerID)
	case StatusBlockedCID:
		return fmt.Sprintf("Caller ID blocked (blocked CID): %s", callerID)
	case StatusBlockedTwit:
		return fmt.Sprintf("Caller ID blocked (twitted): %s", callerID)
	default:
		return fmt.Sprintf("Caller ID: %s", FormatCallerID(callerID))
	}
}
