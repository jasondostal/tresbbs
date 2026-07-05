package session

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// queItem is one line of a NEWUSER.QUE questionnaire: a bound user field (or "?"
// for a free-form question stored in the caller's .ANS file) plus the prompt
// shown to the caller.
type queItem struct {
	field  string // alias|name|city|phone|street|state|zip|email|password, or "?"
	prompt string
}

// knownQueFields are the queItem.field values that bind to a user record.
var knownQueFields = map[string]bool{
	"alias": true, "name": true, "city": true, "phone": true,
	"street": true, "state": true, "zip": true, "email": true, "password": true,
}

// defaultNewUserQue is the built-in questionnaire used when NEWUSER.QUE is
// absent — it preserves the fields the original hardcoded flow collected, so
// the board works out of the box.
func defaultNewUserQue() []queItem {
	return []queItem{
		{"alias", "Choose an alias: "},
		{"name", "Your real name: "},
		{"city", "Your city: "},
		{"phone", "Phone number: "},
		{"street", "Street address (optional): "},
		{"state", "State (optional): "},
		{"zip", "ZIP code (optional): "},
		{"email", "Email address (optional): "},
		{"password", "Choose a password: "},
	}
}

// loadNewUserQue parses a sysop-editable NEWUSER.QUE questionnaire. Each line is
// "field|prompt" (field bound to the user record) or just "prompt" / "?|prompt"
// (a free-form question stored in the caller's .ANS file). Blank lines and
// lines starting with # are ignored. Falls back to the built-in default when
// the file is missing or empty, and always guarantees an alias+password prompt
// so an account can be created.
func loadNewUserQue(path string) []queItem {
	f, err := os.Open(path)
	if err != nil {
		return defaultNewUserQue()
	}
	defer f.Close()

	var items []queItem
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		field, prompt := "?", line
		if i := strings.Index(line, "|"); i >= 0 {
			field = strings.TrimSpace(line[:i])
			prompt = strings.TrimSpace(line[i+1:]) + " "
			if field != "?" && !knownQueFields[field] {
				// Unknown binding — treat as a free-form question.
				field = "?"
			}
		}
		items = append(items, queItem{field: field, prompt: prompt})
	}
	if len(items) == 0 {
		return defaultNewUserQue()
	}

	// Guarantee alias + password prompts exist (account can't be created without).
	has := func(f string) bool {
		for _, it := range items {
			if it.field == f {
				return true
			}
		}
		return false
	}
	if !has("alias") {
		items = append([]queItem{{"alias", "Choose an alias: "}}, items...)
	}
	if !has("password") {
		items = append(items, queItem{"password", "Choose a password: "})
	}
	return items
}

// writeAnsFile stores a caller's free-form questionnaire answers to
// answers/<alias>.ANS, in TriBBS's Q%d./A%d. format.
func writeAnsFile(alias string, qa []string) error {
	if len(qa) == 0 {
		return nil
	}
	if err := os.MkdirAll("answers", 0o755); err != nil {
		return err
	}
	safe := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, alias)
	path := filepath.Join("answers", safe+".ANS")
	return os.WriteFile(path, []byte(strings.Join(qa, "\n")+"\n"), 0o644)
}
