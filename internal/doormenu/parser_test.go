package doormenu

import (
	"path/filepath"
	"testing"
)

// TestParseDoorsMnu pins the real DOORS.MNU format from the TriBBS 5.01 manual:
// a color header followed by "door-type,description,batch-file,security" lines,
// with doors selected by ordinal.
func TestParseDoorsMnu(t *testing.T) {
	entries, err := ParseDoorsMnu(filepath.Join("testdata", "DOORS.MNU"))
	if err != nil {
		t.Fatalf("ParseDoorsMnu: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3 (header skipped)", len(entries))
	}
	e0 := entries[0]
	if e0.Ordinal != 1 || e0.DoorType != 'U' || e0.Description != "MaineRelay Hub" ||
		e0.BatchFile != "HUBDOOR" || e0.Security != 50 {
		t.Errorf("entry[0] = %+v; want ordinal 1, type U, MaineRelay Hub, HUBDOOR, 50", e0)
	}
	e1 := entries[1]
	if e1.Ordinal != 2 || e1.DoorType != 'S' || e1.BatchFile != "GLOBAL" || e1.Security != 10 {
		t.Errorf("entry[1] = %+v; want ordinal 2, type S, GLOBAL, 10", e1)
	}
}

// TestLoadDoorsFromMnu checks the mapping onto domain.Door: door-type -> drop
// format, batch file -> command, ordinal -> numeric hotkey.
func TestLoadDoorsFromMnu(t *testing.T) {
	doors, err := LoadDoorsFromMnu(filepath.Join("testdata", "DOORS.MNU"))
	if err != nil {
		t.Fatalf("LoadDoorsFromMnu: %v", err)
	}
	if len(doors) != 3 {
		t.Fatalf("got %d doors, want 3", len(doors))
	}
	if doors[0].Name != "MaineRelay Hub" || doors[0].DropFormat != "UTIDOOR" ||
		doors[0].Command != "HUBDOOR" || doors[0].HotKey != '1' || doors[0].Security != 50 {
		t.Errorf("door[0] = %+v; want MaineRelay Hub/UTIDOOR/HUBDOOR/'1'/50", doors[0])
	}
	if doors[1].DropFormat != "SFDOORS" || doors[1].HotKey != '2' {
		t.Errorf("door[1] = %+v; want SFDOORS/'2'", doors[1])
	}
}

func TestDropFormatForType(t *testing.T) {
	cases := map[byte]string{
		'R': "DORINFO", 'W': "CALLINFO", 'U': "UTIDOOR",
		'T': "TRIBBS", 'S': "SFDOORS", 'D': "DOORSYS",
	}
	for typ, want := range cases {
		if got := dropFormatForType(typ); got != want {
			t.Errorf("dropFormatForType(%c) = %q, want %q", typ, got, want)
		}
	}
}

func TestHotKeyForOrdinal(t *testing.T) {
	cases := map[int]byte{1: '1', 9: '9', 10: 'A', 11: 'B'}
	for n, want := range cases {
		if got := hotKeyForOrdinal(n); got != want {
			t.Errorf("hotKeyForOrdinal(%d) = %q, want %q", n, got, want)
		}
	}
}
