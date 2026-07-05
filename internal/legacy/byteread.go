// Package legacy imports original TriBBS 5.01 binary data files (USERS.DAT,
// MCONF.DAT, FAREA.DAT, SYSDAT1/2.DAT, …) into tresbbs's SQLite store, so a
// sysop can drop his decade-old board data in and go.
//
// TriBBS was 16-bit DOS Borland C++: records are fixed-length structs written
// straight to disk, so all multi-byte integers are little-endian, `int` is 2
// bytes and `long` is 4. Strings are stored in fixed-width fields, NUL-padded.
package legacy

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

// CString extracts a fixed-width string field: the bytes at [off, off+size),
// truncated at the first NUL and right-trimmed of spaces.
func CString(buf []byte, off, size int) string {
	if off < 0 || size <= 0 || off >= len(buf) {
		return ""
	}
	end := off + size
	if end > len(buf) {
		end = len(buf)
	}
	field := buf[off:end]
	if i := indexByte(field, 0); i >= 0 {
		field = field[:i]
	}
	return strings.TrimRight(string(field), " ")
}

// Int16 reads a little-endian signed 16-bit int (Borland `int`).
func Int16(buf []byte, off int) int {
	if off < 0 || off+2 > len(buf) {
		return 0
	}
	return int(int16(binary.LittleEndian.Uint16(buf[off:])))
}

// Int32 reads a little-endian signed 32-bit int (Borland `long`).
func Int32(buf []byte, off int) int64 {
	if off < 0 || off+4 > len(buf) {
		return 0
	}
	return int64(int32(binary.LittleEndian.Uint32(buf[off:])))
}

// Byte reads a single byte (Borland `char`/`unsigned char` flag).
func Byte(buf []byte, off int) int {
	if off < 0 || off >= len(buf) {
		return 0
	}
	return int(buf[off])
}

// ReadRecords reads path as a sequence of fixed-size records. A trailing
// partial record (some TriBBS files carry a header record or slack) is dropped
// with the count returned so callers can log it rather than silently truncate.
func ReadRecords(path string, recordSize int) (records [][]byte, remainder int, err error) {
	if recordSize <= 0 {
		return nil, 0, fmt.Errorf("record size must be positive, got %d", recordSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	n := len(data) / recordSize
	remainder = len(data) % recordSize
	records = make([][]byte, n)
	for i := 0; i < n; i++ {
		records[i] = data[i*recordSize : (i+1)*recordSize]
	}
	return records, remainder, nil
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}
