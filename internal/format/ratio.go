package format

import (
	"fmt"
	"strings"
	"time"
)

// Ratio renders part/total as a console percentage ("99.998%", "0.180%"),
// keeping enough decimals to stay meaningful for tiny error rates.
func Ratio(part, total int64) string {
	if total <= 0 {
		return "0%"
	}
	p := float64(part) * 100 / float64(total)
	switch {
	case p <= 0:
		return "0%"
	case p >= 100:
		return "100%"
	case p < 1, p > 99.9:
		return fmt.Sprintf("%.3f%%", p)
	case p >= 99, p < 10:
		return fmt.Sprintf("%.1f%%", p)
	default:
		return fmt.Sprintf("%.0f%%", p)
	}
}

// DonutDash renders an SVG stroke-dasharray pair for a donut segment covering
// part/total of the circumference.
func DonutDash(part, total int64, circumference float64) string {
	if total <= 0 {
		return fmt.Sprintf("0 %.2f", circumference)
	}
	dash := circumference * float64(part) / float64(total)
	return fmt.Sprintf("%.2f %.2f", dash, circumference-dash)
}

// DateTime renders the audit trail timestamp ("2026-02-26 14:32:08 UTC").
func DateTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05") + " UTC"
}

// GroupDigits renders an integer with locale separators (pt: "1.424.891.020",
// en: "1,424,891,020") for exact byte counts in object details.
func GroupDigits(n int64, locale string) string {
	separator := ","
	if locale == "pt" {
		separator = "."
	}
	raw := fmt.Sprintf("%d", n)
	groups := make([]string, 0, len(raw)/3+1)
	for len(raw) > 3 {
		groups = append([]string{raw[len(raw)-3:]}, groups...)
		raw = raw[:len(raw)-3]
	}
	groups = append([]string{raw}, groups...)
	return strings.Join(groups, separator)
}
