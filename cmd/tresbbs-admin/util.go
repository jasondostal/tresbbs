package main

import (
	"strconv"

	"github.com/rivo/tview"
)

// ---- form field readers --------------------------------------------------
//
// tview.Form.GetFormItemByLabel returns the FormItem interface; these helpers
// type-assert to the concrete widget and pull the current value out.

func formText(f *tview.Form, label string) string {
	if item, ok := f.GetFormItemByLabel(label).(*tview.InputField); ok {
		return item.GetText()
	}
	return ""
}

func formArea(f *tview.Form, label string) string {
	if item, ok := f.GetFormItemByLabel(label).(*tview.TextArea); ok {
		return item.GetText()
	}
	return ""
}

func formChecked(f *tview.Form, label string) bool {
	if item, ok := f.GetFormItemByLabel(label).(*tview.Checkbox); ok {
		return item.IsChecked()
	}
	return false
}

func formDropdown(f *tview.Form, label string) (int, string) {
	if item, ok := f.GetFormItemByLabel(label).(*tview.DropDown); ok {
		return item.GetCurrentOption()
	}
	return -1, ""
}

// ---- numeric parsing (keep the old value on a parse error) ---------------

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func atoi64Or(s string, def int64) int64 {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return def
}

func atofOr(s string, def float64) float64 {
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return n
	}
	return def
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
