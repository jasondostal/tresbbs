package menu

import "fmt"

// cgaToAnsi maps a DOS/CGA color index (0-7) to its ANSI SGR color index.
// DOS orders colors black,blue,green,cyan,red,magenta,brown,white; ANSI orders
// black,red,green,yellow,blue,magenta,cyan,white — so the blue/red and
// cyan/brown pairs swap.
var cgaToAnsi = [8]int{0, 4, 2, 6, 1, 5, 3, 7}

// fgSGR returns the SGR parameter for a DOS foreground index (0-15). Indices
// 8-15 are the bright colors (ANSI 90-97).
func fgSGR(n int) int {
	n &= 0x0F
	if n < 8 {
		return 30 + cgaToAnsi[n]
	}
	return 90 + cgaToAnsi[n-8]
}

// bgSGR returns the SGR parameter for a DOS background index (0-15). Indices
// 8-15 map to the bright background range (ANSI 100-107); standard DOS text
// mode only allows 0-7 for background, but .MNU files may specify either.
func bgSGR(n int) int {
	n &= 0x0F
	if n < 8 {
		return 40 + cgaToAnsi[n]
	}
	return 100 + cgaToAnsi[n-8]
}

// attr builds an ANSI SGR sequence setting foreground and background from DOS
// color indices, resetting prior attributes first so each color is absolute
// (matching how a DOS text attribute byte fully replaces the cell attribute).
func attr(bg, fg int) string {
	return fmt.Sprintf("\033[0;%d;%dm", fgSGR(fg), bgSGR(bg))
}

const ansiReset = "\033[0m"
