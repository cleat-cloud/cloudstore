package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A console with no scans yet must say so explicitly (and offer the scan)
// instead of showing zeros as if the account were empty.
func TestBucketsHandler_firstScanBannerAndKPI(t *testing.T) {
	f := newConsoleFixture(t)

	rr := httptest.NewRecorder()
	NewBucketsHandler(f.deps).List(rr, authedRequest(http.MethodGet, "/buckets", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "Metrics not collected yet") {
		t.Error("missing the first-scan banner")
	}
	if !strings.Contains(body, "pending scan") {
		t.Error("KPI hint should read pending scan before the first scan")
	}

	if _, err := f.deps.Console.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	NewBucketsHandler(f.deps).List(rr, authedRequest(http.MethodGet, "/buckets", nil))
	body = rr.Body.String()
	if strings.Contains(body, "Metrics not collected yet") {
		t.Error("banner should disappear after a scan")
	}
	if !strings.Contains(body, "Last scan:") {
		t.Error("KPI hint should show the last scan time")
	}
}

// The optimizer only counts scanned buckets whose lifecycle is Standard:
// unscanned buckets are not "missing lifecycle rules".
func TestBucketsHandler_optimizerOnlyCountsScannedBuckets(t *testing.T) {
	f := newConsoleFixture(t)

	rr := httptest.NewRecorder()
	NewBucketsHandler(f.deps).List(rr, authedRequest(http.MethodGet, "/buckets", nil))
	if strings.Contains(rr.Body.String(), "Cost Saving Recommendation") {
		t.Error("optimizer banner must not fire before the first scan")
	}

	if _, err := f.deps.Console.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	scans, err := f.deps.Console.Scans()
	if err != nil {
		t.Fatal(err)
	}
	standard := 0
	for _, usage := range scans {
		if usage.Class == "Standard" {
			standard++
		}
	}

	rr = httptest.NewRecorder()
	NewBucketsHandler(f.deps).List(rr, authedRequest(http.MethodGet, "/buckets", nil))
	body := rr.Body.String()
	if (standard > 0) != strings.Contains(body, "Cost Saving Recommendation") {
		t.Fatalf("optimizer presence should follow the %d scanned Standard buckets", standard)
	}
}
