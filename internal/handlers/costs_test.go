package handlers

import (
	"testing"

	"github.com/brianvoe/gofakeit/v7"
)

func TestStorageCostCents(t *testing.T) {
	cases := []struct {
		name   string
		bytes  int64
		perGB  float64
		expect int64
	}{
		{"vazio", 0, b2StoragePerGBMonth, 0},
		{"negativo", -1, b2StoragePerGBMonth, 0},
		{"56 GB na B2", 56_000_000_000, b2StoragePerGBMonth, 34},
		{"56 GB no S3", 56_000_000_000, s3StoragePerGBMonth, 129},
		{"1 TB na B2", 1_000_000_000_000, b2StoragePerGBMonth, 600},
		{"1 TB no S3", 1_000_000_000_000, s3StoragePerGBMonth, 2300},
		{"15 TB na B2", 15_000_000_000_000, b2StoragePerGBMonth, 9000},
	}
	for _, tc := range cases {
		if got := storageCostCents(tc.bytes, tc.perGB); got != tc.expect {
			t.Errorf("%s: storageCostCents = %d, want %d", tc.name, got, tc.expect)
		}
	}
}

// B2 is always the cheaper rate, so the comparison can never invert.
func TestCostPanelCents_b2AlwaysCheaper(t *testing.T) {
	faker := gofakeit.New(7)
	for i := 0; i < 200; i++ {
		bytes := int64(faker.Float64Range(0, 1e15))
		b2, s3 := costPanelCents(bytes)
		if b2 < 0 || s3 < 0 {
			t.Fatalf("custos negativos para %d bytes: %d / %d", bytes, b2, s3)
		}
		if s3 < b2 {
			t.Fatalf("S3 (%d) menor que B2 (%d) para %d bytes", s3, b2, bytes)
		}
	}
}

func TestCostMultipleLabel(t *testing.T) {
	if got := costMultipleLabel(129, 34, "pt"); got != "3,8×" {
		t.Errorf("pt = %q, want 3,8×", got)
	}
	if got := costMultipleLabel(129, 34, "en"); got != "3.8×" {
		t.Errorf("en = %q, want 3.8×", got)
	}
	if got := costMultipleLabel(0, 0, "pt"); got != "—" {
		t.Errorf("sem dados = %q, want —", got)
	}
}

func TestCostPercentLabel(t *testing.T) {
	cases := []struct {
		percent float64
		locale  string
		expect  string
	}{
		{0.38, "pt", "0,4%"},
		{0.38, "en", "0.4%"},
		{0.01, "pt", "0%"},
		{12.4, "pt", "12%"},
		{99.6, "en", "100%"},
		{-5, "pt", "0%"},
	}
	for _, tc := range cases {
		if got := costPercentLabel(tc.percent, tc.locale); got != tc.expect {
			t.Errorf("costPercentLabel(%v, %s) = %q, want %q", tc.percent, tc.locale, got, tc.expect)
		}
	}
}

func TestDailyCapUSD_override(t *testing.T) {
	t.Setenv("CLOUDSTORE_DAILY_CAP_USD", "12.5")
	if got := dailyCapUSD(); got != 12.5 {
		t.Errorf("dailyCapUSD = %v, want 12.5", got)
	}
	t.Setenv("CLOUDSTORE_DAILY_CAP_USD", "abc")
	if got := dailyCapUSD(); got != defaultDailyCapUSD {
		t.Errorf("valor inválido deveria cair no default, got %v", got)
	}
	t.Setenv("CLOUDSTORE_DAILY_CAP_USD", "")
	if got := dailyCapUSD(); got != defaultDailyCapUSD {
		t.Errorf("vazio deveria cair no default, got %v", got)
	}
}
