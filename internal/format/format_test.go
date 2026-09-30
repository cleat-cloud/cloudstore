package format

import (
	"regexp"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
)

func TestBytes_boundaries(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{999, "999 B"},
		{1000, "1 KB"},
		{1500, "1.5 KB"},
		{480000, "480 KB"},
		{12000, "12 KB"},
		{1420000, "1.42 MB"},
		{68400000000000, "68.4 TB"},
		{2100000000000, "2.1 TB"},
		{420 * 1000 * 1000 * 1000, "420 GB"},
	}
	for _, tc := range cases {
		if got := Bytes(tc.in); got != tc.want {
			t.Errorf("Bytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBytes_fakerValuesKeepThreeSignificantDigits(t *testing.T) {
	faker := gofakeit.New(42)
	pattern := regexp.MustCompile(`^[1-9]\d{0,2}(\.\d{1,2})? (B|KB|MB|GB|TB|PB)$`)
	for i := 0; i < 200; i++ {
		n := int64(faker.Number(1, 1<<50))
		if got := Bytes(n); !pattern.MatchString(got) {
			t.Fatalf("Bytes(%d) = %q, want 1-3 significant digits with a unit", n, got)
		}
	}
}

func TestCount_compact(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1500, "1.5K"},
		{115000, "115K"},
		{450000, "450K"},
		{1840000, "1.84M"},
		{12100000, "12.1M"},
	}
	for _, tc := range cases {
		if got := Count(tc.in); got != tc.want {
			t.Errorf("Count(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDate_locales(t *testing.T) {
	when := time.Date(2023, time.January, 14, 16, 32, 0, 0, time.UTC)
	if got := Date(when, "en"); got != "14 Jan 2023" {
		t.Errorf("Date(en) = %q", got)
	}
	if got := Date(when, "pt"); got != "14 Jan 2023" {
		t.Errorf("Date(pt) = %q", got)
	}
	ago := time.Date(2023, time.August, 22, 9, 0, 0, 0, time.UTC)
	if got := Date(ago, "pt"); got != "22 Ago 2023" {
		t.Errorf("Date(pt) = %q", got)
	}
}

func TestRelative_todayAndYesterday(t *testing.T) {
	now := time.Date(2026, time.February, 26, 18, 0, 0, 0, time.UTC)
	today := time.Date(2026, time.February, 26, 10, 14, 0, 0, time.UTC)
	yesterday := time.Date(2026, time.February, 25, 16, 32, 0, 0, time.UTC)
	older := time.Date(2025, time.February, 24, 9, 0, 0, 0, time.UTC)

	if got := Relative(today, now, "pt"); got != "Hoje, 10:14" {
		t.Errorf("Relative(today) = %q", got)
	}
	if got := Relative(yesterday, now, "pt"); got != "Ontem, 16:32" {
		t.Errorf("Relative(yesterday) = %q", got)
	}
	if got := Relative(yesterday, now, "en"); got != "Yesterday, 16:32" {
		t.Errorf("Relative(yesterday, en) = %q", got)
	}
	if got := Relative(older, now, "pt"); got != "24 Fev 2025" {
		t.Errorf("Relative(older) = %q", got)
	}
}

func TestMoneyUSD(t *testing.T) {
	cases := []struct {
		cents int64
		want  string
	}{
		{0, "$0.00"},
		{4200, "$42.00"},
		{642000, "$6,420.00"},
		{1842050, "$18,420.50"},
	}
	for _, tc := range cases {
		if got := MoneyUSD(tc.cents); got != tc.want {
			t.Errorf("MoneyUSD(%d) = %q, want %q", tc.cents, got, tc.want)
		}
	}
}
