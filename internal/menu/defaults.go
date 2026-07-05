package menu

import (
	"bytes"
	"embed"
	"path/filepath"
	"strings"
)

// defaultFS holds the stock TriBBS 5.01 menu files, embedded so tresbbs renders
// faithful menus with no external assets. A sysop's own files (loaded from disk)
// take precedence — see LoadOrDefault.
//
//go:embed default/*.MNU
var defaultFS embed.FS

// LoadOrDefault loads menu file (e.g. "MAIN.MNU") from dir, falling back to the
// embedded stock menu when dir is empty or the file is missing/unreadable. This
// is the drop-in path: point dir at a TriBBS NWORK/ directory and the sysop's
// existing .MNU files render unchanged; omit it and the board still works.
func LoadOrDefault(dir, file string) (*Menu, error) {
	if dir != "" {
		if m, err := Load(filepath.Join(dir, file)); err == nil {
			return m, nil
		}
	}
	data, err := defaultFS.ReadFile("default/" + strings.ToUpper(file))
	if err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(strings.ToUpper(file), ".MNU")
	return Parse(name, bytes.NewReader(data))
}
