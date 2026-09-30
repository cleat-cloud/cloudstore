package handlers

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cleat-cloud/cloudstore/internal/format"
)

// Storage rates (USD per GB-month) used for the cost panel. B2 is the real
// rate; S3 is the Standard-class rate, shown only as a comparison.
const (
	b2StoragePerGBMonth = 0.006
	s3StoragePerGBMonth = 0.023
)

// defaultDailyCapUSD mirrors the account's B2 "Daily Storage Cap" so the panel
// can show how far today's storage spend is from it. Override with
// CLOUDSTORE_DAILY_CAP_USD when the cap in the Backblaze panel changes.
const defaultDailyCapUSD = 3.00

func dailyCapUSD() float64 {
	raw := strings.TrimSpace(os.Getenv("CLOUDSTORE_DAILY_CAP_USD"))
	if raw == "" {
		return defaultDailyCapUSD
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return defaultDailyCapUSD
	}
	return value
}

// storageCostCents converts a stored byte volume to cents per month.
func storageCostCents(bytes int64, perGBMonth float64) int64 {
	if bytes <= 0 || perGBMonth <= 0 {
		return 0
	}
	gigabytes := float64(bytes) / 1e9
	return int64(gigabytes*perGBMonth*100 + 0.5)
}

// costPanelCents returns the monthly storage cost on B2 and the same volume on
// S3 Standard, in cents.
func costPanelCents(bytes int64) (b2, s3 int64) {
	return storageCostCents(bytes, b2StoragePerGBMonth), storageCostCents(bytes, s3StoragePerGBMonth)
}

// costPanel fills the analytics view model with the estimated storage spend,
// the S3 comparison and how much of the account's daily cap it represents.
// Downloads are not part of B2's cap maths here: the console does not measure
// egress, so the panel says so.
func (vm *analyticsVM) costPanel(bytes int64, locale string) {
	b2Cents, s3Cents := costPanelCents(bytes)
	capCents := int64(dailyCapUSD()*100 + 0.5)

	vm.CostReady = bytes > 0
	vm.CostB2Monthly = format.MoneyUSD(b2Cents)
	vm.CostS3Monthly = format.MoneyUSD(s3Cents)
	vm.CostSavings = costMultipleLabel(s3Cents, b2Cents, locale)
	vm.CostDaily = format.MoneyUSD(b2Cents / 30)
	vm.CostCap = format.MoneyUSD(capCents)

	percent := 0.0
	if capCents > 0 {
		percent = float64(b2Cents) / 30 / float64(capCents) * 100
	}
	vm.CostPercent = costPercentLabel(percent, locale)

	width := int(percent + 0.5)
	if width < 2 {
		width = 2
	}
	if width > 100 {
		width = 100
	}
	vm.CostBarWidth = width
}

// costMultipleLabel formats "how many times cheaper B2 is", e.g. "3,8×".
func costMultipleLabel(s3Cents, b2Cents int64, locale string) string {
	if b2Cents <= 0 || s3Cents <= 0 {
		return "—"
	}
	label := fmt.Sprintf("%.1f×", float64(s3Cents)/float64(b2Cents))
	if locale == "pt" {
		label = strings.Replace(label, ".", ",", 1)
	}
	return label
}

// costPercentLabel renders the share of the daily cap already spent, keeping a
// decimal place while it is still under 10%.
func costPercentLabel(percent float64, locale string) string {
	if percent < 0 {
		percent = 0
	}
	label := fmt.Sprintf("%d%%", int(percent+0.5))
	if percent < 10 {
		label = fmt.Sprintf("%.1f%%", percent)
		if locale == "pt" {
			label = strings.Replace(label, ".", ",", 1)
		}
	}
	if percent < 0.05 {
		label = "0%"
		if locale == "pt" {
			label = "0%"
		}
	}
	return label
}
