package store

import (
	"testing"
	"time"
)

// The overview renders "last scan" from bucket_scans.scanned_at; if the driver
// returns a format parseSQLiteTime does not know, the KPI silently shows
// "pending scan" even right after a scan.
func TestBucketScans_scannedAtIsParsed(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertBucketScan("alpha-cdn", 1, 2, "Standard"); err != nil {
		t.Fatal(err)
	}

	var raw string
	if err := s.db.QueryRow("SELECT scanned_at FROM bucket_scans WHERE bucket_name = ?", "alpha-cdn").Scan(&raw); err != nil {
		t.Fatal(err)
	}

	scans, err := s.BucketScans()
	if err != nil {
		t.Fatal(err)
	}
	usage := scans["alpha-cdn"]
	if usage.ScannedAt.IsZero() {
		t.Fatalf("ScannedAt is zero — raw driver value = %q", raw)
	}
	if age := time.Since(usage.ScannedAt); age < 0 || age > time.Hour {
		t.Fatalf("ScannedAt = %v (raw %q), want a recent timestamp", usage.ScannedAt, raw)
	}
}

func TestUsageHistory_dayIsParsed(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertBucketScan("alpha-cdn", 1, 2, "Standard"); err != nil {
		t.Fatal(err)
	}

	var raw string
	if err := s.db.QueryRow("SELECT day FROM usage_daily LIMIT 1").Scan(&raw); err != nil {
		t.Fatal(err)
	}

	history, err := s.UsageHistory(30)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) == 0 {
		t.Fatalf("empty history — raw day = %q", raw)
	}
	if history[len(history)-1].Day.IsZero() {
		t.Fatalf("history day is zero — raw = %q", raw)
	}
}
