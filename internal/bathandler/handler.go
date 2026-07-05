// Package bathandler provides support for v11.6+ BAT file hooks.
//
// In TriBBS 11.6, several BAT file hooks were added to allow sysop
// customization of various BBS events:
//
//   CHAT.BAT    - Called when chat is initiated
//   GOODBYE.BAT - Called when user logs off
//   EDITOR.BAT  - Called for external message editing
//   PAGE.BAT    - Called when paging another user
//
// These BAT files receive context via environment variables and
// can customize the BBS behavior without modifying the source.
package bathandler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// BATEvent represents a BAT file hook event.
type BATEvent string

const (
	EventChat    BATEvent = "CHAT"
	EventGoodbye BATEvent = "GOODBYE"
	EventEditor  BATEvent = "EDITOR"
	EventPage    BATEvent = "PAGE"
)

// BATContext provides context to BAT file execution.
type BATContext struct {
	// User info
	UserName  string
	UserAlias string
	City      string
	Phone     string
	Security  int
	BaudRate  string
	Node      int

	// Session info
	TimeLeft    int
	TotalCalls  int64
	LoginTime   time.Time

	// Event-specific
	TargetUser string // For PAGE - who's being paged
	TargetNode int    // For PAGE - target node
	Message    string // For CHAT - initial message
	EditorFile string // For EDITOR - file being edited
}

// RunBAT executes a BAT file hook if it exists.
// Returns true if the BAT file was found and executed.
func RunBAT(event BATEvent, baseDir string, ctx *BATContext) (bool, error) {
	batFile := filepath.Join(baseDir, string(event)+".BAT")

	// Check if BAT file exists
	if _, err := os.Stat(batFile); os.IsNotExist(err) {
		return false, nil
	}

	// Set environment variables for the BAT file
	env := buildEnvironment(event, ctx)
	
	// Execute the BAT file
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", batFile)
	} else {
		// On non-Windows, try using sh or skip
		cmd = exec.Command("sh", batFile)
	}
	cmd.Env = env
	cmd.Dir = baseDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return true, fmt.Errorf("BAT file %s failed: %w (output: %s)", string(event), err, string(output))
	}

	return true, nil
}

// buildEnvironment creates environment variables for the BAT file.
func buildEnvironment(event BATEvent, ctx *BATContext) []string {
	env := os.Environ()

	if ctx == nil {
		return env
	}

	// Common variables
	env = append(env,
		fmt.Sprintf("BBSEVENT=%s", string(event)),
		fmt.Sprintf("BBSNODE=%d", ctx.Node),
		fmt.Sprintf("BBSUSERNAME=%s", ctx.UserName),
		fmt.Sprintf("BBSUSERALIAS=%s", ctx.UserAlias),
		fmt.Sprintf("BBSCITY=%s", ctx.City),
		fmt.Sprintf("BBSPHONE=%s", ctx.Phone),
		fmt.Sprintf("BBSSECURITY=%d", ctx.Security),
		fmt.Sprintf("BBSBAUD=%s", ctx.BaudRate),
		fmt.Sprintf("BBSTIMELEFT=%d", ctx.TimeLeft),
		fmt.Sprintf("BBSTOTALCALLS=%d", ctx.TotalCalls),
	)

	// Event-specific variables
	switch event {
	case EventPage:
		env = append(env,
			fmt.Sprintf("BBSTARGETUSER=%s", ctx.TargetUser),
			fmt.Sprintf("BBSTARGETNODE=%d", ctx.TargetNode),
		)
	case EventChat:
		env = append(env,
			fmt.Sprintf("BBSCHATMSG=%s", ctx.Message),
		)
	case EventEditor:
		env = append(env,
			fmt.Sprintf("BBSEDITORFILE=%s", ctx.EditorFile),
		)
	}

	return env
}

// ChatBAT executes CHAT.BAT when chat is initiated.
func ChatBAT(baseDir string, ctx *BATContext) error {
	_, err := RunBAT(EventChat, baseDir, ctx)
	return err
}

// GoodbyeBAT executes GOODBYE.BAT when user logs off.
func GoodbyeBAT(baseDir string, ctx *BATContext) error {
	_, err := RunBAT(EventGoodbye, baseDir, ctx)
	return err
}

// EditorBAT executes EDITOR.BAT for external message editing.
func EditorBAT(baseDir string, ctx *BATContext) (string, error) {
	// Editor.BAT should write the message to a temp file
	// and return the path
	executed, err := RunBAT(EventEditor, baseDir, ctx)
	if !executed {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	// Check for the edited file
	if ctx.EditorFile != "" {
		return ctx.EditorFile, nil
	}
	return "", nil
}

// PageBAT executes PAGE.BAT when paging another user.
func PageBAT(baseDir string, ctx *BATContext) error {
	_, err := RunBAT(EventPage, baseDir, ctx)
	return err
}
