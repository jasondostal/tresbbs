package legacy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCString(t *testing.T) {
	// "Unnamed BBS\0" in a 20-byte field, mirroring SYSDAT1.DAT's board-name.
	buf := make([]byte, 20)
	copy(buf, "Unnamed BBS")
	if got := CString(buf, 0, 20); got != "Unnamed BBS" {
		t.Errorf("CString = %q, want %q", got, "Unnamed BBS")
	}
	// space-padded field trims trailing spaces
	copy(buf, "ZIP   ")
	for i := 3; i < 20; i++ {
		buf[i] = ' '
	}
	if got := CString(buf, 0, 20); got != "ZIP" {
		t.Errorf("CString space-pad = %q, want ZIP", got)
	}
	// out-of-range is safe
	if got := CString(buf, 100, 4); got != "" {
		t.Errorf("out-of-range CString = %q, want empty", got)
	}
}

func TestIntReaders(t *testing.T) {
	// little-endian: 0x3C 0x00 = 60, 0xFF 0xFF = -1
	buf := []byte{0x3C, 0x00, 0xFF, 0xFF, 0x64, 0x00, 0x00, 0x00}
	if got := Int16(buf, 0); got != 60 {
		t.Errorf("Int16 = %d, want 60", got)
	}
	if got := Int16(buf, 2); got != -1 {
		t.Errorf("Int16 signed = %d, want -1", got)
	}
	if got := Int32(buf, 4); got != 100 {
		t.Errorf("Int32 = %d, want 100", got)
	}
	if got := Byte(buf, 0); got != 0x3C {
		t.Errorf("Byte = %d, want 60", got)
	}
}

func TestReadRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.dat")
	// three 4-byte records + 2 bytes slack
	if err := os.WriteFile(path, []byte("AAAABBBBCCCCxy"), 0644); err != nil {
		t.Fatal(err)
	}
	recs, rem, err := ReadRecords(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || rem != 2 {
		t.Fatalf("got %d records, %d remainder; want 3, 2", len(recs), rem)
	}
	if string(recs[1]) != "BBBB" {
		t.Errorf("record[1] = %q, want BBBB", string(recs[1]))
	}
}
