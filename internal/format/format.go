package format

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var byteUnits = []string{"B", "KB", "MB", "GB", "TB", "PB"}
var countUnits = []string{"", "K", "M", "B"}

// Bytes formats a byte count with decimal units and up to three significant
// digits, matching the console design ("68.4 TB", "480 KB", "1.42 GB").
func Bytes(n int64) string {
	if n < 0 {
		n = 0
	}
	value := float64(n)
	unit := 0
	for value >= 999.5 && unit < len(byteUnits)-1 {
		value /= 1000
		unit++
	}
	return significant(value) + " " + byteUnits[unit]
}

// Count formats large counts compactly ("1.84M", "450K").
func Count(n int64) string {
	if n < 0 {
		n = 0
	}
	value := float64(n)
	unit := 0
	for value >= 999.5 && unit < len(countUnits)-1 {
		value /= 1000
		unit++
	}
	return significant(value) + countUnits[unit]
}

// significant prints a float with two decimals below 10, one below 100 and
// none above, trimming trailing zeros (three significant digits).
func significant(value float64) string {
	var s string
	switch {
	case value >= 100:
		s = strconv.FormatFloat(value, 'f', 0, 64)
	case value >= 10:
		s = strconv.FormatFloat(value, 'f', 1, 64)
	default:
		s = strconv.FormatFloat(value, 'f', 2, 64)
	}
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

var monthsEN = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
var monthsPT = [...]string{"Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"}

// Date formats a timestamp as the console's dense table date ("14 Jan 2023").
func Date(t time.Time, locale string) string {
	t = t.UTC()
	months := monthsEN[:]
	if locale == "pt" {
		months = monthsPT[:]
	}
	return fmt.Sprintf("%02d %s %d", t.Day(), months[t.Month()-1], t.Year())
}

// Relative renders today/yesterday with a time and falls back to Date.
func Relative(t time.Time, now time.Time, locale string) string {
	t, now = t.UTC(), now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch {
	case !t.Before(today):
		return clockLabel("Hoje", "Today", t, locale)
	case !t.Before(today.AddDate(0, 0, -1)):
		return clockLabel("Ontem", "Yesterday", t, locale)
	default:
		return Date(t, locale)
	}
}

func clockLabel(pt, en string, t time.Time, locale string) string {
	label := en
	if locale == "pt" {
		label = pt
	}
	return fmt.Sprintf("%s, %02d:%02d", label, t.Hour(), t.Minute())
}

// MoneyUSD formats US cents with grouping ("$6,420.00").
func MoneyUSD(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	dollars := strconv.FormatInt(cents/100, 10)
	var groups []string
	for len(dollars) > 3 {
		groups = append([]string{dollars[len(dollars)-3:]}, groups...)
		dollars = dollars[:len(dollars)-3]
	}
	groups = append([]string{dollars}, groups...)
	return fmt.Sprintf("%s$%s.%02d", sign, strings.Join(groups, ","), cents%100)
}
