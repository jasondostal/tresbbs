// Package template implements the template engine for tresbbs.
//
// The original TriBBS used @VARIABLE placeholders in text files that were
// loaded and rendered at runtime. This allowed sysops to customize their
// BBS without recompiling — just edit the .tpl files.
//
// Template syntax:
//   - @VARIABLE — replaced with the variable's value
//   - @X0A through @X0F — color codes (mapped to ANSI)
//   - @X70 — reverse video
//   - @CLS — clear screen
//   - @PAUSE — wait for keypress
//   - @HANGUP — disconnect
//   - @BEEP — sound bell
//   - @MOREON / @MOREOFF — toggle "more" prompting
//   - @BREAKON / @BREAKOFF — toggle Ctrl-Break
//
// Templates are loaded from the templates/ directory. Each template
// file is a plain text file with @VARIABLE placeholders.
package template

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jasondostal/tresbbs/domain"
)

// Engine renders templates with variable substitution.
type Engine struct {
	config   *domain.Config
	templateDir string
}

// NewEngine creates a new template engine.
func NewEngine(config *domain.Config, templateDir string) *Engine {
	return &Engine{
		config:      config,
		templateDir: templateDir,
	}
}

// TemplateData provides the context for template rendering.
type TemplateData struct {
	User    *domain.User
	Config  *domain.Config
	Session *SessionData
	System  *SystemData
}

// SessionData provides session-specific variables.
type SessionData struct {
	NodeNumber  int
	BaudRate    string
	LoginTime   time.Time
	TimeLeft    int
	CurrentMenu string
}

// SystemData provides system-wide variables.
type SystemData struct {
	SystemCalls  int64
	CallsToday   int
	TotalUsers   int
}

// RenderFile loads and renders a template file.
func (e *Engine) RenderFile(filename string, data *TemplateData) (string, error) {
	path := filepath.Join(e.templateDir, filename)
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening template %s: %w", filename, err)
	}
	defer file.Close()

	var result strings.Builder
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := e.RenderLine(scanner.Text(), data)
		result.WriteString(line)
		result.WriteString("\r\n")
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading template %s: %w", filename, err)
	}

	return result.String(), nil
}

// RenderLine processes a single line of a template, substituting variables.
// Control-code sentinels. RenderLine flattens a template to a flat string, so
// the flow-control directives (@PAUSE/@HANGUP/@MOREON/@MOREOFF/@BREAKON/@BREAKOFF)
// are emitted as these markers for the writing layer to act on (see
// Session.writeTemplate). CtrlMarker is a Unicode private-use rune that never
// occurs in real BBS content, followed by a one-rune command code.
const CtrlMarker = '\uE000'

const (
	CtrlPause    = "\uE000P"
	CtrlHangup   = "\uE000H"
	CtrlMoreOn   = "\uE000M"
	CtrlMoreOff  = "\uE000m"
	CtrlBreakOn  = "\uE000B"
	CtrlBreakOff = "\uE000b"
)

func (e *Engine) RenderLine(line string, data *TemplateData) string {
	var result strings.Builder
	src := []rune(line)
	i := 0

	for i < len(src) {
		if src[i] == '@' {
			i++ // skip @

			if i >= len(src) {
				result.WriteRune('@')
				break
			}

			// Check for color code: @X<B><F> — DOS attribute byte, first hex
			// digit is the background nibble, second is the foreground nibble.
			if src[i] == 'X' && i+2 < len(src) {
				bgCode := byte(src[i+1])
				fgCode := byte(src[i+2])
				if bgAnsi, ok := bgColorMap[bgCode]; ok {
					result.WriteString(bgAnsi)
				}
				if fgAnsi, ok := colorMap[fgCode]; ok {
					result.WriteString(fgAnsi)
				}
				i += 3 // consume 'X' + both hex digits
				continue
			}

			// Check for printf-style format: @VAR%%FORMAT or @VAR:FORMAT
			// e.g., @ALIAS%-20s or @CALLS%05d
			varStart := i
			for i < len(src) && (isAlphaNum(src[i]) || src[i] == '_') {
				i++
			}

			if i > varStart {
				varName := string(src[varStart:i])
				
				// Check for format specifier after variable name
				if i < len(src) && src[i] == '%' {
					// Parse printf-style format string
					fmtStart := i
					i++
					// Read format specifier: flags, width, precision, verb
					for i < len(src) && (src[i] == '-' || src[i] == '+' || src[i] == ' ' || src[i] == '#' || src[i] == '0') {
						i++
					}
					// Width
					for i < len(src) && src[i] >= '0' && src[i] <= '9' {
						i++
					}
					// Precision
					if i < len(src) && src[i] == '.' {
						i++
						for i < len(src) && src[i] >= '0' && src[i] <= '9' {
							i++
						}
					}
					// Verb (s, d, f, etc.)
					if i < len(src) {
						i++
					}
					
					fmtStr := string(src[fmtStart:i])
					value := e.resolveVariableFormatted(varName, fmtStr, data)
					result.WriteString(value)
				} else {
					value := e.resolveVariable(varName, data)
					result.WriteString(value)
				}
			} else {
				result.WriteRune('@')
			}
		} else {
			result.WriteRune(src[i])
			i++
		}
	}

	return result.String()
}

// resolveVariable looks up a template variable.
func (e *Engine) resolveVariable(name string, data *TemplateData) string {
	// System variables
	switch name {
	case "CLS":
		return "\033[2J\033[H"
	case "BEEP":
		return "\a"
	case "PAUSE":
		return CtrlPause
	case "HANGUP":
		return CtrlHangup
	case "MOREON":
		return CtrlMoreOn
	case "MOREOFF":
		return CtrlMoreOff
	case "BREAKON":
		return CtrlBreakOn
	case "BREAKOFF":
		return CtrlBreakOff
	case "BOARDNAME":
		if data.Config != nil {
			return data.Config.BoardName
		}
		return ""
	case "SYSOPNAME":
		if data.Config != nil {
			return data.Config.SysopName
		}
		return ""
	case "VERSIONNUMBER":
		return domain.Version
	case "SYSTEMDATE":
		return time.Now().Format("01/02/2006")
	case "SYSTEMTIME":
		return time.Now().Format("03:04 PM")
	case "BBSSTARTDATE":
		if data.Config != nil {
			return data.Config.BBSStartDate
		}
		return ""
	}

	// Session variables
	if data.Session != nil {
		switch name {
		case "NODE":
			return fmt.Sprintf("%d", data.Session.NodeNumber)
		case "BAUDRATE":
			return data.Session.BaudRate
		case "TIMELEFT":
			return fmt.Sprintf("%d", data.Session.TimeLeft)
		case "TIMEREMAININGFORDAY":
			return fmt.Sprintf("%d", data.Session.TimeLeft)
		case "TIMETHISCALL":
			elapsed := int(time.Since(data.Session.LoginTime).Minutes())
			return fmt.Sprintf("%d", elapsed)
		}
	}

	// System stats
	if data.System != nil {
		switch name {
		case "SYSTEMCALLS":
			return fmt.Sprintf("%d", data.System.SystemCalls)
		case "SYSTEMCALLSTODAY":
			return fmt.Sprintf("%d", data.System.CallsToday)
		case "TOTALUSERS":
			return fmt.Sprintf("%d", data.System.TotalUsers)
		}
	}

	// User variables
	if data.User != nil {
		switch name {
		case "USER":
			return data.User.Name
		case "ALIAS":
			return data.User.Alias
		case "FIRST":
			parts := strings.Fields(data.User.Name)
			if len(parts) > 0 {
				return parts[0]
			}
			return data.User.Name
		case "PHONE":
			return data.User.Phone
		case "CITY":
			return data.User.City
		case "BIRTHDATE":
			return data.User.BirthDate
		case "SECURITY":
			return fmt.Sprintf("%d", data.User.SecurityLevel)
		case "SYSOPMENU":
			// Renders the Sysop-menu option only for users with sysop access,
			// so it isn't shown (and confusingly gated) for everyone else.
			if data.Config != nil && data.User.SecurityLevel >= data.Config.SysopSecurity {
				return "<S> Sysop Menu"
			}
			return ""
		case "CALLS":
			return fmt.Sprintf("%d", data.User.TotalCalls)
		case "CALLSTODAY":
			return fmt.Sprintf("%d", data.User.CallsToday)
		case "DOWNLOADS":
			return fmt.Sprintf("%d", data.User.FilesDownloaded)
		case "UPLOADS":
			return fmt.Sprintf("%d", data.User.FilesUploaded)
		case "KDOWNLOADED":
			return fmt.Sprintf("%d", data.User.KDownloaded)
		case "KUPLOADED":
			return fmt.Sprintf("%d", data.User.KUploaded)
		case "MESSAGES":
			return fmt.Sprintf("%d", data.User.MessagesPosted)
		case "MESSAGESTODAY":
			return fmt.Sprintf("%d", data.User.MessagesToday)
		case "FILERATIO":
			return fmt.Sprintf("%.1f", data.User.FileRatio)
		case "LASTDATEON":
			if !data.User.LastLogin.IsZero() {
				return data.User.LastLogin.Format("01/02/2006")
			}
			return ""
		case "LASTTIMEON":
			if !data.User.LastLogin.IsZero() {
				return data.User.LastLogin.Format("03:04 PM")
			}
			return ""
		case "REGISTRATIONNUMBER":
			return data.User.Registration
		case "SUBSCRIPTIONDATE":
			if data.User.Subscription != nil {
				return data.User.Subscription.Format("01/02/2006")
			}
			return "None"
		
		// v11.6+ fields
		case "EMAILADDRESS":
			return data.User.Email
		case "STREETADDRESS":
			return data.User.StreetAddress
		case "STATE":
			return data.User.State
		case "ZIPCODE":
			return data.User.ZipCode
		case "COUNTRY":
			return data.User.Country
		case "CITYANDSTATE":
			if data.User.State != "" {
				return data.User.City + ", " + data.User.State
			}
			return data.User.City
		case "DEFAULTCOUNTRY":
			if data.Config != nil {
				return data.Config.DefaultCountry
			}
			return ""
		}
	}

	// Unknown variable — return empty
	return ""
}

// resolveVariableFormatted resolves a variable and applies a printf-style format.
func (e *Engine) resolveVariableFormatted(name string, format string, data *TemplateData) string {
	value := e.resolveVariable(name, data)
	
	// Apply format based on the verb
	if len(format) < 2 {
		return value
	}
	
	verb := format[len(format)-1]
	
	switch verb {
	case 's', 'S':
		// String format
		return fmt.Sprintf(format, value)
	case 'd', 'D', 'i', 'I':
		// Integer format
		var intVal int64
		fmt.Sscanf(value, "%d", &intVal)
		return fmt.Sprintf(format, intVal)
	case 'f', 'F', 'e', 'E', 'g', 'G':
		// Float format
		var floatVal float64
		fmt.Sscanf(value, "%f", &floatVal)
		return fmt.Sprintf(format, floatVal)
	case 'x', 'X':
		// Hex format
		var intVal int64
		fmt.Sscanf(value, "%d", &intVal)
		return fmt.Sprintf(format, intVal)
	case 'c', 'C':
		// Character format
		var intVal int
		fmt.Sscanf(value, "%d", &intVal)
		return fmt.Sprintf(format, intVal)
	default:
		return value
	}
}

// Color map from @X__ codes to ANSI escape sequences.
// TriBBS uses a hex color system: @XFG where F=foreground, G=background.
// The original DOS colors map to ANSI terminal colors.
var colorMap = map[byte]string{
	// Foreground colors (standard DOS palette)
	'0': "\033[30m", // Black
	'1': "\033[34m", // Blue
	'2': "\033[32m", // Green
	'3': "\033[36m", // Cyan
	'4': "\033[31m", // Red
	'5': "\033[35m", // Magenta
	'6': "\033[33m", // Brown/Yellow
	'7': "\033[37m", // Light Gray
	// Bright foreground colors
	'8': "\033[90m", // Dark Gray
	'9': "\033[94m", // Light Blue
	'A': "\033[92m", // Light Green
	'B': "\033[96m", // Light Cyan
	'C': "\033[91m", // Light Red
	'D': "\033[95m", // Light Magenta
	'E': "\033[93m", // Yellow
	'F': "\033[97m", // White
}

// Background color map from @X_ codes to ANSI escape sequences.
var bgColorMap = map[byte]string{
	'0': "\033[40m",  // Black
	'1': "\033[44m",  // Blue
	'2': "\033[42m",  // Green
	'3': "\033[46m",  // Cyan
	'4': "\033[41m",  // Red
	'5': "\033[45m",  // Magenta
	'6': "\033[43m",  // Brown/Yellow
	'7': "\033[47m",  // Light Gray
	'8': "\033[100m", // Dark Gray
	'9': "\033[104m", // Light Blue
	'A': "\033[102m", // Light Green
	'B': "\033[106m", // Light Cyan
	'C': "\033[101m", // Light Red
	'D': "\033[105m", // Light Magenta
	'E': "\033[103m", // Yellow
	'F': "\033[107m", // White
}

// Helper
func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
