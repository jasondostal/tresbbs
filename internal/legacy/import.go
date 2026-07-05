package legacy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/internal/auth"
)

// Target is the subset of the storage layer the importer writes into. The
// sqlite store satisfies it; tests can supply a fake.
type Target interface {
	AddUser(user *domain.User) (int, error)
	AddConference(conf *domain.Conference) error
	AddFileArea(area *domain.FileArea) error
	SaveConfig(config *domain.Config) error
}

// Result summarizes what an import run did, for reporting to the sysop.
type Result struct {
	Users        int
	Conferences  int
	FileAreas    int
	ConfigLoaded bool
	Warnings     []string
}

func (r Result) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Imported: %d users, %d conferences, %d file areas",
		r.Users, r.Conferences, r.FileAreas)
	if r.ConfigLoaded {
		b.WriteString(", system config")
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "\n  warning: %s", w)
	}
	return b.String()
}

// find looks for name in dir case-insensitively (DOS files were uppercase;
// they may arrive lower-cased off a modern filesystem or archive).
func find(dir, name string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// Import reads an original TriBBS data directory and lands it into the target.
// Missing files are skipped (a partial board still imports what it has).
func Import(dir string, t Target) (Result, error) {
	var res Result
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return res, fmt.Errorf("%s is not a directory", dir)
	}

	// System configuration (SYSDAT1.DAT).
	if p := find(dir, "SYSDAT1.DAT"); p != "" {
		cfg, warns, err := ParseSystemConfig(p)
		res.Warnings = append(res.Warnings, warns...)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("SYSDAT1.DAT: %v", err))
		} else if err := t.SaveConfig(cfg); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("saving config: %v", err))
		} else {
			res.ConfigLoaded = true
		}
	}

	// Users (USERS.DAT).
	if p := find(dir, "USERS.DAT"); p != "" {
		users, warns, err := ParseUsers(p)
		res.Warnings = append(res.Warnings, warns...)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("USERS.DAT: %v", err))
		}
		for _, u := range users {
			// TriBBS stored passwords in plaintext; re-hash so imported users
			// can log in with their original password.
			if u.Password != "" {
				if h, herr := auth.HashPassword(u.Password); herr == nil {
					u.Password = h
				}
			}
			if _, err := t.AddUser(&u); err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("user %q: %v", u.Alias, err))
				continue
			}
			res.Users++
		}
	}

	// Conferences (MCONF.DAT).
	if p := find(dir, "MCONF.DAT"); p != "" {
		confs, warns, err := ParseConferences(p)
		res.Warnings = append(res.Warnings, warns...)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("MCONF.DAT: %v", err))
		}
		for _, c := range confs {
			if err := t.AddConference(&c); err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("conference %q: %v", c.Name, err))
				continue
			}
			res.Conferences++
		}
	}

	// File areas (FAREA.DAT).
	if p := find(dir, "FAREA.DAT"); p != "" {
		areas, warns, err := ParseFileAreas(p)
		res.Warnings = append(res.Warnings, warns...)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("FAREA.DAT: %v", err))
		}
		for _, a := range areas {
			if err := t.AddFileArea(&a); err != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("file area %q: %v", a.Name, err))
				continue
			}
			res.FileAreas++
		}
	}

	return res, nil
}
