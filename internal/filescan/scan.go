// Package filescan adds files sitting in a file area's directory to the file
// base — the "drop files in a folder and they show up" workflow TriBBS's FILEMAN
// provided. New files are picked up; ones already listed are left alone.
package filescan

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/internal/fileutil"
)

// Store is the slice of storage the scanner needs.
type Store interface {
	GetFileAreas() ([]domain.FileArea, error)
	GetFiles(area string, limit int) ([]domain.FileEntry, error)
	AddFile(*domain.FileEntry) error
}

// skip is the set of housekeeping filenames the scanner ignores.
func skip(name string) bool {
	switch strings.ToLower(name) {
	case "file_id.diz", "files.bbs", ".ds_store", "descript.ion":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// ScanArea adds any files in area.Path that aren't already listed in the area.
// Returns how many were added.
func ScanArea(store Store, area domain.FileArea) (int, error) {
	entries, err := os.ReadDir(area.Path)
	if err != nil {
		return 0, err
	}
	existing, _ := store.GetFiles(area.Name, 1000000)
	known := make(map[string]bool, len(existing))
	for _, f := range existing {
		known[strings.ToLower(f.Name)] = true
	}
	added := 0
	for _, e := range entries {
		if e.IsDir() || skip(e.Name()) || known[strings.ToLower(e.Name())] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		desc := ""
		if diz, derr := fileutil.ExtractFileIDDIZ(filepath.Join(area.Path, e.Name())); derr == nil {
			desc = diz
		}
		if err := store.AddFile(&domain.FileEntry{
			Area:        area.Name,
			Name:        e.Name(),
			Size:        info.Size(),
			Description: desc,
			UploadedBy:  "Sysop",
		}); err != nil {
			continue
		}
		added++
	}
	return added, nil
}

// ScanAll scans every file area. Returns total files added and a per-area
// breakdown for reporting.
func ScanAll(store Store) (total int, byArea map[string]int, err error) {
	areas, err := store.GetFileAreas()
	if err != nil {
		return 0, nil, err
	}
	byArea = make(map[string]int)
	for _, a := range areas {
		n, aerr := ScanArea(store, a)
		if aerr != nil {
			continue // e.g. path doesn't exist yet; skip quietly
		}
		byArea[a.Name] = n
		total += n
	}
	return total, byArea, nil
}
