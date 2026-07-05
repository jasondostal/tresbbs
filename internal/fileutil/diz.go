// Package fileutil provides file management utilities for tresbbs.
//
// This package handles FILE_ID.DIZ extraction, file list sorting, and
// other file-related operations that were part of the original TriBBS.
package fileutil

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/jasondostal/tresbbs/domain"
)

// ExtractFileIDDIZ extracts FILE_ID.DIZ from a ZIP archive.
// FILE_ID.DIZ is a standard file description format used by BBS systems.
// It's typically a plain text file with a 10-line limit (45 chars per line).
func ExtractFileIDDIZ(zipPath string) (string, error) {
	// Open the ZIP file
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("opening zip: %w", err)
	}
	defer reader.Close()

	// Look for FILE_ID.DIZ (case-insensitive)
	for _, file := range reader.File {
		upper := strings.ToUpper(file.Name)
		if upper == "FILE_ID.DIZ" || upper == "FILE_ID.TXT" || upper == "DESC.SDI" || upper == "DESC.DAT" {
			// Found it — read the contents
			rc, err := file.Open()
			if err != nil {
				continue
			}
			defer rc.Close()

			var buf bytes.Buffer
			if _, err := io.Copy(&buf, rc); err != nil {
				continue
			}

			// Clean up the description
			desc := cleanDescription(buf.String())
			return desc, nil
		}
	}

	return "", nil // No FILE_ID.DIZ found
}

// cleanDescription cleans up a FILE_ID.DIZ description.
// - Trims whitespace
// - Limits to 10 lines
// - Limits each line to 45 characters
func cleanDescription(raw string) string {
	lines := strings.Split(raw, "\n")
	var cleaned []string

	for _, line := range lines {
		line = strings.TrimRight(line, "\r\n\t ")
		if len(line) > 45 {
			line = line[:45]
		}
		if len(line) > 0 {
			cleaned = append(cleaned, line)
		}
	}

	// Limit to 10 lines
	if len(cleaned) > 10 {
		cleaned = cleaned[:10]
	}

	return strings.Join(cleaned, "\n")
}

// SortFiles sorts a slice of file entries by the specified sort type.
// Supported sort types:
//   - "name" (default): alphabetical by filename
//   - "date": newest first
//   - "size": largest first
//   - "downloads": most downloaded first
func SortFiles(files []domain.FileEntry, sortType string) []domain.FileEntry {
	if len(files) == 0 {
		return files
	}

	// Make a copy to avoid modifying the original
	sorted := make([]domain.FileEntry, len(files))
	copy(sorted, files)

	switch strings.ToLower(sortType) {
	case "date":
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].UploadedAt.After(sorted[j].UploadedAt)
		})
	case "size":
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Size > sorted[j].Size
		})
	case "downloads":
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Downloads > sorted[j].Downloads
		})
	default: // "name" or empty
		sort.Slice(sorted, func(i, j int) bool {
			return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
		})
	}

	return sorted
}

// IsCDROMArea checks if a file area is a CD-ROM (read-only) area.
func IsCDROMArea(area domain.FileArea) bool {
	return area.CDROM
}

// ValidateFileUpload validates a file upload against area restrictions.
func ValidateFileUpload(area domain.FileArea, filename string, fileSize int64, user *domain.User) error {
	// Check if area is CD-ROM (read-only)
	if area.CDROM {
		return fmt.Errorf("cannot upload to CD-ROM area (read-only)")
	}

	// Check file size limits (if configured)
	// TODO: Add per-area size limits

	// Check user daily byte limit
	if user.DailyByteLimit > 0 {
		if user.KUploaded*1024+fileSize > int64(user.DailyByteLimit)*1024 {
			return fmt.Errorf("daily upload limit reached (%d KB)", user.DailyByteLimit)
		}
	}

	// Check user daily file limit
	if user.DailyFileLimit > 0 {
		if user.UploadsToday >= user.DailyFileLimit {
			return fmt.Errorf("daily file upload limit reached (%d files)", user.DailyFileLimit)
		}
	}

	return nil
}

// FormatFileSize formats a file size in human-readable format.
func FormatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	} else if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
}

// FormatFileDate formats a file date for display.
func FormatFileDate(t time.Time) string {
	return t.Format("01/02/06 03:04 PM")
}
