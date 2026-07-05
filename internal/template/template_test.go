package template

import (
	"strings"
	"testing"
	"time"

	"github.com/jasondostal/tresbbs/domain"
)

func TestRenderLine_BasicVariables(t *testing.T) {
	engine := NewEngine(&domain.Config{BoardName: "Test BBS"}, "templates")
	
	data := &TemplateData{
		User: &domain.User{
			Name:  "John Doe",
			Alias: "Johnny",
			City:  "Madison",
		},
		Config: &domain.Config{
			BoardName: "Test BBS",
			SysopName: "Sysop",
		},
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"Hello @USER", "Hello John Doe"},
		{"@ALIAS is here", "Johnny is here"},
		{"Board: @BOARDNAME", "Board: Test BBS"},
		{"City: @CITY", "City: Madison"},
		{"@UNKNOWN", ""},
	}

	for _, tt := range tests {
		result := engine.RenderLine(tt.input, data)
		if result != tt.expected {
			t.Errorf("RenderLine(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestRenderLine_ColorCodes(t *testing.T) {
	engine := NewEngine(nil, "templates")
	data := &TemplateData{}

	// @X takes a two-hex-digit DOS attribute (@X<bg><fg>): @X0A = light green
	// on black (92m), @X0F = white on black (97m).
	input := "@X0AGreen@X0FWhite"
	result := engine.RenderLine(input, data)

	if !strings.Contains(result, "\033[92m") {
		t.Errorf("Expected green ANSI code (92m), got: %q", result)
	}
	if !strings.Contains(result, "\033[97m") {
		t.Errorf("Expected white ANSI code (97m), got: %q", result)
	}
	// The color-code digits must not leak into the output as literal text.
	if strings.Contains(result, "AGreen") || strings.Contains(result, "FWhite") {
		t.Errorf("color-code digit leaked into text: %q", result)
	}
}

func TestRenderLine_PrintfFormat(t *testing.T) {
	engine := NewEngine(nil, "templates")
	data := &TemplateData{
		User: &domain.User{
			TotalCalls: 42,
		},
	}

	input := "Calls: @CALLS%05d"
	result := engine.RenderLine(input, data)

	if !strings.Contains(result, "00042") {
		t.Errorf("Expected padded number, got: %s", result)
	}
}

func TestRenderLine_ControlSequences(t *testing.T) {
	engine := NewEngine(nil, "templates")
	data := &TemplateData{}

	tests := []struct {
		input    string
		contains string
	}{
		{"@CLS", "\033[2J\033[H"},
		{"@BEEP", "\a"},
	}

	for _, tt := range tests {
		result := engine.RenderLine(tt.input, data)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("RenderLine(%q) = %q, want containing %q", tt.input, result, tt.contains)
		}
	}
}

func TestRenderLine_FlowControlSentinels(t *testing.T) {
	engine := NewEngine(nil, "templates")
	data := &TemplateData{}
	cases := map[string]string{
		"@PAUSE":    CtrlPause,
		"@HANGUP":   CtrlHangup,
		"@MOREON":   CtrlMoreOn,
		"@MOREOFF":  CtrlMoreOff,
		"@BREAKON":  CtrlBreakOn,
		"@BREAKOFF": CtrlBreakOff,
	}
	for input, want := range cases {
		if got := engine.RenderLine(input, data); got != want {
			t.Errorf("RenderLine(%q) = %q, want sentinel %q", input, got, want)
		}
		// Every sentinel must start with the private-use marker so the writer
		// can find it and never collide with real content.
		if []rune(want)[0] != CtrlMarker {
			t.Errorf("sentinel for %q does not begin with CtrlMarker", input)
		}
	}
}

func TestRenderLine_V116Variables(t *testing.T) {
	engine := NewEngine(&domain.Config{DefaultCountry: "US"}, "templates")
	data := &TemplateData{
		User: &domain.User{
			Email:         "john@example.com",
			StreetAddress: "123 Main St",
			State:         "WI",
			ZipCode:       "53703",
			Country:       "US",
			City:          "Madison",
		},
		Config: &domain.Config{
			DefaultCountry: "US",
		},
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"Email: @EMAILADDRESS", "Email: john@example.com"},
		{"Street: @STREETADDRESS", "Street: 123 Main St"},
		{"State: @STATE", "State: WI"},
		{"ZIP: @ZIPCODE", "ZIP: 53703"},
		{"Country: @COUNTRY", "Country: US"},
		{"@CITYANDSTATE", "Madison, WI"},
		{"@DEFAULTCOUNTRY", "US"},
	}

	for _, tt := range tests {
		result := engine.RenderLine(tt.input, data)
		if result != tt.expected {
			t.Errorf("RenderLine(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func BenchmarkRenderLine(b *testing.B) {
	engine := NewEngine(&domain.Config{BoardName: "Test BBS"}, "templates")
	data := &TemplateData{
		User: &domain.User{
			Name:         "Test User",
			Alias:        "TestAlias",
			City:         "Madison",
			TotalCalls:   100,
			SecurityLevel: 10,
		},
		Config: &domain.Config{
			BoardName: "Test BBS",
			SysopName: "Sysop",
		},
		Session: &SessionData{
			NodeNumber: 1,
			BaudRate:   "Telnet",
			LoginTime:  time.Now(),
			TimeLeft:   30,
		},
	}

	input := "@X0AUser: @X0E@ALIAS @X0A(@X0F@USER@X0A) Node: @X0E@NODE @X0ATime: @X0E@TIMELEFT"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.RenderLine(input, data)
	}
}

func BenchmarkRenderLine_Printf(b *testing.B) {
	engine := NewEngine(nil, "templates")
	data := &TemplateData{
		User: &domain.User{
			TotalCalls: 12345,
		},
	}

	input := "Calls: @CALLS%06d"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.RenderLine(input, data)
	}
}
