package legacy

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// makeUserRecord builds a synthetic USERS.DAT record of the given size with the
// high-confidence fields populated, to exercise the parser + size detector.
func makeUserRecord(size int, name, alias, pass, city, phone, birth string, sec int) []byte {
	rec := make([]byte, size)
	put := func(off int, s string, max int) {
		b := []byte(s)
		if len(b) > max-1 {
			b = b[:max-1]
		}
		copy(rec[off:], b)
	}
	put(offUserName, name, 31)
	put(offUserAlias, alias, 31)
	put(offUserPassword, pass, 16)
	put(offUserCity, city, 31)
	put(offUserPhone, phone, 13)
	put(offUserBirth, birth, 9)
	binary.LittleEndian.PutUint16(rec[offUserSecurity:], uint16(sec))
	return rec
}

func TestParseUserRecord(t *testing.T) {
	rec := makeUserRecord(256, "Jason Dostal", "jase", "hunter2", "Plover, WI", "715-555-0100", "01/01/70", 90)
	u := parseUserRecord(rec)
	if u.Name != "Jason Dostal" || u.Alias != "jase" || u.Password != "hunter2" {
		t.Errorf("name/alias/pass wrong: %+v", u)
	}
	if u.City != "Plover, WI" || u.Phone != "715-555-0100" || u.BirthDate != "01/01/70" {
		t.Errorf("city/phone/birth wrong: %+v", u)
	}
	if u.SecurityLevel != 90 || u.LockedOut {
		t.Errorf("security/locked wrong: sec=%d locked=%v", u.SecurityLevel, u.LockedOut)
	}
}

func TestDetectAndParseUsers(t *testing.T) {
	const size = 256
	var data []byte
	data = append(data, makeUserRecord(size, "Alice", "al", "pw1", "Town, XX", "555-1", "01/02/80", 90)...)
	data = append(data, makeUserRecord(size, "Bob", "bob", "pw2", "City, YY", "555-2", "03/04/81", 20)...)

	if got := detectUserRecordSize(data); got != size {
		t.Fatalf("detectUserRecordSize = %d, want %d", got, size)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "USERS.DAT")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	users, warns, err := ParseUsers(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Fatalf("got %d users, want 2 (warns: %v)", len(users), warns)
	}
	if users[0].Alias != "al" || users[1].Alias != "bob" {
		t.Errorf("parsed users wrong: %+v", users)
	}
}
