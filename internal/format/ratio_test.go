package format

import (
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
)

func TestRatio(t *testing.T) {
	cases := []struct {
		part, total int64
		want        string
	}{
		{0, 0, "0%"},
		{0, 10, "0%"},
		{10, 10, "100%"},
		{99998, 100000, "99.998%"},
		{998, 1000, "99.8%"},
		{180, 100000, "0.180%"},
		{10, 100000, "0.010%"},
		{1, 100000, "0.001%"},
		{42, 1000, "4.2%"},
		{680, 1000, "68%"},
		{3, 9, "33%"},
	}
	for _, tc := range cases {
		if got := Ratio(tc.part, tc.total); got != tc.want {
			t.Errorf("Ratio(%d, %d) = %q, want %q", tc.part, tc.total, got, tc.want)
		}
	}
}

func TestRatio_fakerStaysInRange(t *testing.T) {
	faker := gofakeit.New(9)
	for i := 0; i < 100; i++ {
		total := int64(faker.Number(1, 1_000_000))
		part := int64(faker.Number(0, int(total)))
		got := Ratio(part, total)
		if got == "" {
			t.Fatalf("Ratio(%d, %d) is empty", part, total)
		}
	}
}

func TestDonutDash(t *testing.T) {
	if got := DonutDash(1, 2, 100); got != "50.00 50.00" {
		t.Errorf("DonutDash = %q", got)
	}
	if got := DonutDash(0, 0, 238.76); got != "0 238.76" {
		t.Errorf("empty DonutDash = %q", got)
	}
}

func TestDateTime(t *testing.T) {
	when := time.Date(2026, time.February, 26, 14, 32, 8, 0, time.UTC)
	if got := DateTime(when); got != "2026-02-26 14:32:08 UTC" {
		t.Errorf("DateTime = %q", got)
	}
}

func TestGroupDigits(t *testing.T) {
	cases := []struct {
		n      int64
		locale string
		want   string
	}{
		{0, "en", "0"},
		{142, "en", "142"},
		{1424891020, "en", "1,424,891,020"},
		{1424891020, "pt", "1.424.891.020"},
	}
	for _, tc := range cases {
		if got := GroupDigits(tc.n, tc.locale); got != tc.want {
			t.Errorf("GroupDigits(%d, %q) = %q, want %q", tc.n, tc.locale, got, tc.want)
		}
	}
}
