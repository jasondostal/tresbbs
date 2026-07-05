package ansi

import (
	"bytes"
	"strings"
	"testing"
)

// TestLocalEcho verifies ReadLine echoes typed characters (and the newline) only
// when local echo is enabled — the mode Local Logon uses over a raw terminal.
func TestLocalEcho(t *testing.T) {
	// Echo on: the input is written back so the sysop sees what they type.
	var out bytes.Buffer
	d := New(strings.NewReader("hello\r\n"), &out)
	d.SetLocalEcho(true)
	line, err := d.ReadLine()
	if err != nil || line != "hello" {
		t.Fatalf("ReadLine = %q, %v; want \"hello\"", line, err)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Errorf("local echo should have written the typed characters, got %q", out.String())
	}

	// Echo off (default): nothing is written back.
	var out2 bytes.Buffer
	d2 := New(strings.NewReader("hello\r\n"), &out2)
	line2, _ := d2.ReadLine()
	if line2 != "hello" {
		t.Fatalf("ReadLine (no echo) = %q; want \"hello\"", line2)
	}
	if out2.Len() != 0 {
		t.Errorf("with echo off nothing should be written, got %q", out2.String())
	}
}
