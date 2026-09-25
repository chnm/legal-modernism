package main

import (
	"fmt"
	"html/template"
	"math"
	"net/url"
	"strconv"
	"strings"
)

// funcMap holds the functions the templates call.
var funcMap = template.FuncMap{
	"num":         num,
	"pct":         pct,
	"deref":       deref,
	"derefStr":    derefStr,
	"ptrOr":       ptrOr,
	"cleanRaw":    cleanRaw,
	"truncate":    truncate,
	"join":        strings.Join,
	"pathEscape":  url.PathEscape,
	"queryEscape": url.QueryEscape,
	"add":         func(a, b int) int { return a + b },
	"sub":         func(a, b int) int { return a - b },
	"mul":         func(a, b int) int { return a * b },
}

// num formats a count with thousands separators. It accepts the integer types
// the queries scan into, and pointers to them, so a template can pass a
// nullable column without dereferencing it first; nil renders as a dash.
func num(v any) string {
	switch n := v.(type) {
	case int:
		return commas(int64(n))
	case int32:
		return commas(int64(n))
	case int64:
		return commas(n)
	case *int:
		if n == nil {
			return "—"
		}
		return commas(int64(*n))
	case *int64:
		if n == nil {
			return "—"
		}
		return commas(*n)
	case float64:
		return commas(int64(math.Round(n)))
	default:
		return fmt.Sprint(v)
	}
}

func commas(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	if len(s) > 3 {
		var b strings.Builder
		first := len(s) % 3
		if first > 0 {
			b.WriteString(s[:first])
		}
		for i := first; i < len(s); i += 3 {
			if b.Len() > 0 {
				b.WriteByte(',')
			}
			b.WriteString(s[i : i+3])
		}
		s = b.String()
	}
	if neg {
		return "-" + s
	}
	return s
}

// pct renders part as a rounded percentage of whole, or a dash when whole is
// zero.
func pct(part, whole int) string {
	if whole == 0 {
		return "—"
	}
	return strconv.Itoa((part*100+whole/2)/whole) + "%"
}

// deref returns the value behind an *int or *string, or nil.
func deref(v any) any {
	switch p := v.(type) {
	case *int:
		if p != nil {
			return *p
		}
	case *int64:
		if p != nil {
			return *p
		}
	case *string:
		if p != nil {
			return *p
		}
	case *bool:
		if p != nil {
			return *p
		}
	}
	return nil
}

// derefStr returns the string behind the pointer, or "".
func derefStr(v *string) string {
	if v != nil {
		return *v
	}
	return ""
}

// ptrOr returns the escaped string behind the pointer, or the fallback HTML
// (an entity such as &mdash;) when the pointer is nil or empty.
func ptrOr(v *string, fallback string) template.HTML {
	if v != nil && *v != "" {
		return template.HTML(template.HTMLEscapeString(*v))
	}
	return template.HTML(fallback)
}

// cleanRaw collapses the line breaks a detected citation often carries (OCR
// line breaks inside the cite) into single spaces for display.
func cleanRaw(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// truncate shortens s to at most n runes, marking the cut with an ellipsis.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

// yearSpan renders a first and last year as "1765–1854", a single year when
// they coincide, or a dash when there is none.
func yearSpan(first, last *int) string {
	switch {
	case first == nil && last == nil:
		return "—"
	case first == nil:
		return strconv.Itoa(*last)
	case last == nil || *first == *last:
		return strconv.Itoa(*first)
	default:
		return strconv.Itoa(*first) + "–" + strconv.Itoa(*last)
	}
}
