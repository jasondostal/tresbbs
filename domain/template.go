package domain

import (
	"fmt"
	"strings"
	"time"
)

// ===================================================================
// Template Engine — The @VARIABLE System
//
// This is a faithful recreation of TriBBS's template engine. Every
// screen in the original BBS was a text file with @VARIABLE placeholders.
// The engine reads the file, substitutes variables with their current
// values, handles color codes, and sends the result to the terminal.
//
// In 1994, this was basically React — state → template → rendered output.
// ===================================================================

// Color codes — maps @X__ to ANSI escape sequences.
// These are DOS text-mode attribute bytes mapped to ANSI colors.
var colorMap = map[byte]string{
	'A': "\033[92m", // Light Green  @X0A
	'B': "\033[96m", // Light Cyan   @X0B
	'C': "\033[91m", // Light Red    @X0C
	'D': "\033[95m", // Light Magenta @X0D
	'E': "\033[93m", // Yellow       @X0E
	'F': "\033[97m", // White        @X0F
}

// TemplateEngine renders @VARIABLE templates for display.
type TemplateEngine struct {
	config *Config
}

// NewTemplateEngine creates a new template engine with the given config.
func NewTemplateEngine(config *Config) *TemplateEngine {
	return &TemplateEngine{config: config}
}

// Render processes a template string, substituting @VARIABLES and color codes.
// The user parameter provides session-specific values; it can be nil for
// system-level templates that don't need user context.
func (te *TemplateEngine) Render(template string, user *User, session *SessionInfo) string {
	var result strings.Builder
	src := []rune(template)
	i := 0

	for i < len(src) {
		if src[i] == '@' {
			i++ // skip @

			if i >= len(src) {
				result.WriteRune('@')
				break
			}

			// Check for color code: @X__
			if src[i] == 'X' && i+2 < len(src) {
				code := byte(src[i+1])
				if ansi, ok := colorMap[code]; ok {
					result.WriteString(ansi)
					i += 2 // skip the color code
					continue
				}
			}

			// Check for @} (close brace — end of variable in some contexts)
			if src[i] == '}' {
				i++
				continue
			}

			// Extract variable name (alphanumeric)
			varStart := i
			for i < len(src) && (isAlphaNum(src[i]) || src[i] == '_') {
				i++
			}

			if i > varStart {
				varName := string(src[varStart:i])
				value := te.resolveVariable(varName, user, session)
				result.WriteString(value)
			} else {
				// Just an @ sign
				result.WriteRune('@')
			}
		} else {
			result.WriteRune(src[i])
			i++
		}
	}

	return result.String()
}

// SessionInfo provides session context that isn't stored in the User record.
type SessionInfo struct {
	NodeNumber   int
	BaudRate     string // "SSH", "Telnet", "Local"
	LoginTime    time.Time
	TimeLeft     int // minutes remaining
	CurrentMenu  string
	SystemCalls  int64
	CallsToday   int
}

// resolveVariable looks up an @VARIABLE name and returns its value.
// This is the core of the template engine — it maps variable names to
// data from the user record, session state, and system config.
func (te *TemplateEngine) resolveVariable(name string, user *User, session *SessionInfo) string {
	// Session/system variables (don't require user)
	switch name {
	case "CLS":
		return "\033[2J\033[H"
	case "BEEP":
		return "\a"
	case "SYSOPNAME":
		if te.config != nil {
			return te.config.SysopName
		}
		return ""
	case "BOARDNAME":
		if te.config != nil {
			return te.config.BoardName
		}
		return ""
	case "VERSIONNUMBER":
		return Version
	case "SYSTEMDATE":
		return time.Now().Format("01/02/2006")
	case "SYSTEMTIME":
		return time.Now().Format("03:04 PM")
	case "BBSSTARTDATE":
		if te.config != nil {
			return te.config.BBSStartDate
		}
		return ""
	}

	// Session-dependent variables
	if session != nil {
		switch name {
		case "NODE":
			return fmt.Sprintf("%d", session.NodeNumber)
		case "BAUDRATE":
			return session.BaudRate
		case "TIMELEFT":
			return fmt.Sprintf("%d", session.TimeLeft)
		case "TIMEREMAININGFORDAY":
			return fmt.Sprintf("%d", session.TimeLeft)
		case "TIMETHISCALL":
			elapsed := int(time.Since(session.LoginTime).Minutes())
			return fmt.Sprintf("%d", elapsed)
		case "SYSTEMCALLS":
			return fmt.Sprintf("%d", session.SystemCalls)
		case "SYSTEMCALLSTODAY":
			return fmt.Sprintf("%d", session.CallsToday)
		}
	}

	// User-dependent variables
	if user != nil {
		switch name {
		// Identity
		case "USER":
			return user.Name
		case "ALIAS":
			return user.Alias
		case "FIRST":
			parts := strings.Fields(user.Name)
			if len(parts) > 0 {
				return parts[0]
			}
			return user.Name
		case "PHONE":
			return user.Phone
		case "CITY":
			return user.City
		case "BIRTHDATE":
			return user.BirthDate

		// Security
		case "SECURITY":
			return fmt.Sprintf("%d", user.SecurityLevel)
		case "CALLS":
			return fmt.Sprintf("%d", user.TotalCalls)
		case "CALLSTODAY":
			return fmt.Sprintf("%d", user.CallsToday)

		// File stats
		case "DOWNLOADS":
			return fmt.Sprintf("%d", user.FilesDownloaded)
		case "UPLOADS":
			return fmt.Sprintf("%d", user.FilesUploaded)
		case "KDOWNLOADED":
			return fmt.Sprintf("%d", user.KDownloaded)
		case "KUPLOADED":
			return fmt.Sprintf("%d", user.KUploaded)
		case "DOWNLOADSTODAY":
			return fmt.Sprintf("%d", user.DownloadsToday)
		case "UPLOADSTODAY":
			return fmt.Sprintf("%d", user.UploadsToday)
		case "FILERATIO":
			return fmt.Sprintf("%.1f", user.FileRatio)
		case "DAILYFILELIMIT":
			return fmt.Sprintf("%d", user.DailyFileLimit)

		// Message stats
		case "MESSAGES":
			return fmt.Sprintf("%d", user.MessagesPosted)
		case "MESSAGESTODAY":
			return fmt.Sprintf("%d", user.MessagesToday)

		// Dates
		case "LASTDATEON":
			if !user.LastLogin.IsZero() {
				return user.LastLogin.Format("01/02/2006")
			}
			return ""
		case "LASTTIMEON":
			if !user.LastLogin.IsZero() {
				return user.LastLogin.Format("03:04 PM")
			}
			return ""
		case "LASTFILECHECK":
			if !user.LastFileCheck.IsZero() {
				return user.LastFileCheck.Format("01/02/2006")
			}
			return ""

		// Registration
		case "REGISTRATIONNUMBER":
			return user.Registration
		case "SUBSCRIPTIONDATE":
			if user.Subscription != nil {
				return user.Subscription.Format("01/02/2006")
			}
			return "None"
		}
	}

	// Unknown variable — return empty string
	return ""
}

// Colorize applies a TriBBS color code to a string.
func Colorize(text string, colorCode byte) string {
	if ansi, ok := colorMap[colorCode]; ok {
		return ansi + text + "\033[0m"
	}
	return text
}

// Helper: check if rune is alphanumeric
func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
