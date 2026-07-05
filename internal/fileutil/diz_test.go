package fileutil

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jasondostal/tresbbs/domain"
)

func TestExtractFileIDDIZ(t *testing.T) {
	// Create a test ZIP with FILE_ID.DIZ
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")

	// Create ZIP file
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	writer := zip.NewWriter(zipFile)

	// Add FILE_ID.DIZ
	dizEntry, _ := writer.Create("FILE_ID.DIZ")
	dizEntry.Write([]byte("Test Description\nSecond Line\nThird Line"))

	// Add another file
	readmeEntry, _ := writer.Create("readme.txt")
	readmeEntry.Write([]byte("Hello"))

	writer.Close()
	zipFile.Close()

	// Test extraction
	desc, err := ExtractFileIDDIZ(zipPath)
	if err != nil {
		t.Fatalf("ExtractFileIDDIZ() error: %v", err)
	}

	if desc != "Test Description\nSecond Line\nThird Line" {
		t.Errorf("Expected description, got: %q", desc)
	}
}

func TestExtractFileIDDIZ_NoDIZ(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")

	zipFile, _ := os.Create(zipPath)
	writer := zip.NewWriter(zipFile)
	// Add another file
	readmeEntry, _ := writer.Create("readme.txt")
	readmeEntry.Write([]byte("Hello"))
	writer.Close()
	zipFile.Close()

	desc, err := ExtractFileIDDIZ(zipPath)
	if err != nil {
		t.Fatalf("ExtractFileIDDIZ() error: %v", err)
	}

	if desc != "" {
		t.Errorf("Expected empty description, got: %q", desc)
	}
}

func TestExtractFileIDDIZ_DescDat(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")

	zipFile, _ := os.Create(zipPath)
	writer := zip.NewWriter(zipFile)

	// Add DESC.DAT (v11.6 format)
	descEntry, _ := writer.Create("DESC.DAT")
	descEntry.Write([]byte("v11.6 description format"))

	writer.Close()
	zipFile.Close()

	desc, err := ExtractFileIDDIZ(zipPath)
	if err != nil {
		t.Fatalf("ExtractFileIDDIZ() error: %v", err)
	}

	if desc != "v11.6 description format" {
		t.Errorf("Expected v11.6 description, got: %q", desc)
	}
}

func TestExtractFileIDDIZ_CleanDescription(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")

	zipFile, _ := os.Create(zipPath)
	writer := zip.NewWriter(zipFile)

	dizEntry, _ := writer.Create("FILE_ID.DIZ")
	// Write description with long lines and many lines
	longDesc := ""
	for i := 0; i < 15; i++ {
		longDesc += "This is a very long line that exceeds forty-five characters in length\n"
	}
	dizEntry.Write([]byte(longDesc))

	writer.Close()
	zipFile.Close()

	desc, err := ExtractFileIDDIZ(zipPath)
	if err != nil {
		t.Fatalf("ExtractFileIDDIZ() error: %v", err)
	}

	// Verify lines are limited
	lines := splitLines(desc)
	if len(lines) > 10 {
		t.Errorf("Expected max 10 lines, got %d", len(lines))
	}

	// Verify line length
	for _, line := range lines {
		if len(line) > 45 {
			t.Errorf("Line too long (%d chars): %q", len(line), line)
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	current := ""
	for _, c := range s {
		if c == '\n' {
			if current != "" {
				lines = append(lines, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func TestSortFiles_ByName(t *testing.T) {
	files := []domain.FileEntry{
		{Name: "zebra.txt"},
		{Name: "alpha.txt"},
		{Name: "beta.txt"},
	}

	sorted := SortFiles(files, "name")

	if sorted[0].Name != "alpha.txt" {
		t.Errorf("Expected alpha.txt first, got %s", sorted[0].Name)
	}
	if sorted[2].Name != "zebra.txt" {
		t.Errorf("Expected zebra.txt last, got %s", sorted[2].Name)
	}
}

func TestSortFiles_ByDate(t *testing.T) {
	files := []domain.FileEntry{
		{Name: "old.txt", UploadedAt: timeNow().Add(-24 * time.Hour)},
		{Name: "new.txt", UploadedAt: timeNow()},
		{Name: "mid.txt", UploadedAt: timeNow().Add(-12 * time.Hour)},
	}

	sorted := SortFiles(files, "date")

	if sorted[0].Name != "new.txt" {
		t.Errorf("Expected new.txt first, got %s", sorted[0].Name)
	}
}

func TestSortFiles_BySize(t *testing.T) {
	files := []domain.FileEntry{
		{Name: "small.txt", Size: 100},
		{Name: "large.txt", Size: 1000},
		{Name: "medium.txt", Size: 500},
	}

	sorted := SortFiles(files, "size")

	if sorted[0].Name != "large.txt" {
		t.Errorf("Expected large.txt first, got %s", sorted[0].Name)
	}
}

func TestSortFiles_ByDownloads(t *testing.T) {
	files := []domain.FileEntry{
		{Name: "unpopular.txt", Downloads: 10},
		{Name: "popular.txt", Downloads: 100},
		{Name: "average.txt", Downloads: 50},
	}

	sorted := SortFiles(files, "downloads")

	if sorted[0].Name != "popular.txt" {
		t.Errorf("Expected popular.txt first, got %s", sorted[0].Name)
	}
}

func TestSortFiles_CaseInsensitive(t *testing.T) {
	files := []domain.FileEntry{
		{Name: "Zebra.txt"},
		{Name: "alpha.txt"},
	}

	sorted := SortFiles(files, "name")

	if sorted[0].Name != "alpha.txt" {
		t.Errorf("Expected case-insensitive sort, got %s first", sorted[0].Name)
	}
}

func TestIsCDROMArea(t *testing.T) {
	tests := []struct {
		area     domain.FileArea
		expected bool
	}{
		{domain.FileArea{CDROM: true}, true},
		{domain.FileArea{CDROM: false}, false},
	}

	for _, tt := range tests {
		result := IsCDROMArea(tt.area)
		if result != tt.expected {
			t.Errorf("IsCDROMArea(%+v) = %v, want %v", tt.area, result, tt.expected)
		}
	}
}

func TestValidateFileUpload_CDROM(t *testing.T) {
	area := domain.FileArea{CDROM: true}
	user := &domain.User{}

	err := ValidateFileUpload(area, "test.txt", 100, user)
	if err == nil {
		t.Error("Expected error for CD-ROM area upload")
	}
}

func TestValidateFileUpload_DailyByteLimit(t *testing.T) {
	area := domain.FileArea{}
	user := &domain.User{
		DailyByteLimit: 100, // 100 KB limit
		KUploaded:      90,  // Already uploaded 90 KB
	}

	// Try to upload 20 KB (would exceed limit)
	err := ValidateFileUpload(area, "test.txt", 20*1024, user)
	if err == nil {
		t.Error("Expected error for daily byte limit")
	}
}

func TestValidateFileUpload_DailyFileLimit(t *testing.T) {
	area := domain.FileArea{}
	user := &domain.User{
		DailyFileLimit: 5,
		UploadsToday:   5,
	}

	err := ValidateFileUpload(area, "test.txt", 100, user)
	if err == nil {
		t.Error("Expected error for daily file limit")
	}
}

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		result := FormatFileSize(tt.size)
		if result != tt.expected {
			t.Errorf("FormatFileSize(%d) = %q, want %q", tt.size, result, tt.expected)
		}
	}
}

func BenchmarkSortFiles(b *testing.B) {
	files := make([]domain.FileEntry, 1000)
	for i := 0; i < 1000; i++ {
		files[i] = domain.FileEntry{
			Name: fmt.Sprintf("file%04d.txt", i),
			Size: int64(i * 100),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SortFiles(files, "name")
	}
}

func timeNow() time.Time {
	return time.Now()
}
