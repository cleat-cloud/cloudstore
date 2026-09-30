package console

import (
	"context"
	"testing"

	"github.com/cleat-cloud/cloudstore/internal/store"
)

func TestSeedDemo_populatesFirstVisitAndIsIdempotent(t *testing.T) {
	svc, st := newService(t)
	ctx := context.Background()

	if err := svc.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}

	scans, err := st.BucketScans()
	if err != nil {
		t.Fatal(err)
	}
	if len(scans) == 0 {
		t.Fatal("SeedDemo did not scan the demo buckets")
	}
	history, err := svc.UsageHistory(30)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 25 {
		t.Fatalf("history points = %d, want the 30-day curve", len(history))
	}
	events, err := svc.AuditEvents(store.AuditFilter{Action: "ScanBuckets"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("audit events = %d, want 1 seeded scan", len(events))
	}

	// Second boot must not rescan or duplicate the audit entry.
	if err := svc.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := svc.AuditEvents(store.AuditFilter{Action: "ScanBuckets"})
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 {
		t.Fatalf("SeedDemo is not idempotent: %d scan events", len(again))
	}
}

func TestSeedDemo_skipsLiveAccounts(t *testing.T) {
	svc, st := newService(t)
	ctx := context.Background()

	if _, err := svc.AddAccount(ctx, "Backblaze B2", "key-id", "app-key", "us-west-004"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	scans, err := st.BucketScans()
	if err != nil {
		t.Fatal(err)
	}
	if len(scans) != 0 {
		t.Fatalf("live accounts must not be auto-scanned: %d scans", len(scans))
	}
}
