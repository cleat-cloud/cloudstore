package console

import (
	"context"
	"testing"
	"time"

	"github.com/puppe1990/cloudstore/internal/crypto"
	"github.com/puppe1990/cloudstore/internal/models"
	"github.com/puppe1990/cloudstore/internal/storage"
	"github.com/puppe1990/cloudstore/internal/storage/fakestore"
	"github.com/puppe1990/cloudstore/internal/store"
)

func newService(t *testing.T) (*Service, *store.SQLiteStore) {
	t.Helper()
	st, err := store.NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	demo := fakestore.New(42)
	svc := New(st, crypto.DeriveKey("test-secret"), Options{
		Demo:       demo,
		QuotaBytes: 100 * 1000 * 1000 * 1000,
		NewProvider: func(account models.StorageAccount, secret string) storage.Provider {
			if secret == "" {
				t.Error("provider built with an empty secret")
			}
			return demo
		},
	})
	return svc, st
}

func TestActive_fallsBackToDemo(t *testing.T) {
	svc, _ := newService(t)
	active, err := svc.Active(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if active.Live {
		t.Error("no account configured should mean demo mode")
	}
	if active.Provider == nil {
		t.Fatal("demo provider missing")
	}
}

func TestAddAccount_encryptsSecretAndBecomesActive(t *testing.T) {
	svc, st := newService(t)
	ctx := context.Background()

	account, err := svc.AddAccount(ctx, "CloudStore EU", "key-id-1", "super-secret-key", "eu-central-003")
	if err != nil {
		t.Fatal(err)
	}
	if account.Label != "CloudStore EU" || account.KeyID != "key-id-1" || account.Region != "eu-central-003" {
		t.Fatalf("account = %+v", account)
	}

	sealed, err := st.StorageAccountSecret(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "super-secret-key" {
		t.Fatal("application key stored in plaintext")
	}
	opened, err := crypto.Decrypt(crypto.DeriveKey("test-secret"), sealed)
	if err != nil || opened != "super-secret-key" {
		t.Fatalf("stored secret does not decrypt back: %q err=%v", opened, err)
	}

	active, err := svc.Active(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !active.Live || active.Account.ID != account.ID {
		t.Fatalf("active = %+v, want the new live account", active)
	}

	if err := svc.DeleteAccount(ctx, account.ID); err != nil {
		t.Fatal(err)
	}
	active, err = svc.Active(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active.Live {
		t.Error("after deleting the only account the console should be back to demo mode")
	}
}

func TestRefresh_scansBucketsAndSeedsDemoHistory(t *testing.T) {
	svc, st := newService(t)
	ctx := context.Background()

	summary, err := svc.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Buckets < 6 || summary.Objects == 0 || summary.Bytes == 0 {
		t.Fatalf("summary = %+v", summary)
	}

	scans, err := st.BucketScans()
	if err != nil {
		t.Fatal(err)
	}
	if len(scans) != summary.Buckets {
		t.Fatalf("persisted scans = %d, want %d", len(scans), summary.Buckets)
	}

	used, quota, err := svc.Quota()
	if err != nil {
		t.Fatal(err)
	}
	if used != summary.Bytes {
		t.Errorf("quota used = %d, want scanned total %d", used, summary.Bytes)
	}
	if quota != 100*1000*1000*1000 {
		t.Errorf("quota = %d", quota)
	}

	history, err := svc.UsageHistory(30)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 25 {
		t.Fatalf("demo history points = %d, want a 30-day curve", len(history))
	}
	last := history[len(history)-1]
	if last.Bytes != summary.Bytes {
		t.Errorf("latest history point = %d, want today's total %d", last.Bytes, summary.Bytes)
	}

	if _, err := svc.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := svc.UsageHistory(30)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(history) {
		t.Errorf("second refresh changed history length: %d → %d", len(history), len(again))
	}
}

func TestBuckets_mergesLatestScan(t *testing.T) {
	svc, st := newService(t)
	ctx := context.Background()

	if err := st.UpsertBucketScan("prod-assets-media-cdn", 777, 8888, "Archive"); err != nil {
		t.Fatal(err)
	}
	buckets, err := svc.Buckets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found *models.Bucket
	for i := range buckets {
		if buckets[i].Name == "prod-assets-media-cdn" {
			found = &buckets[i]
		}
	}
	if found == nil {
		t.Fatal("bucket missing from listing")
	}
	if !found.Scanned || found.Objects != 777 || found.Bytes != 8888 || found.Class != "Archive" {
		t.Fatalf("merged bucket = %+v", *found)
	}
}

func TestEstimateMonthlyCostCents(t *testing.T) {
	if got := EstimateMonthlyCostCents(12 * 1000 * 1000 * 1000 * 1000); got != 7200 {
		t.Errorf("12 TB = %d cents, want 7200", got)
	}
	if got := EstimateMonthlyCostCents(0); got != 0 {
		t.Errorf("0 bytes = %d cents, want 0", got)
	}
}

func TestAudit_roundTrip(t *testing.T) {
	svc, _ := newService(t)

	if err := svc.Audit(models.AuditEvent{
		At: time.Now().UTC(), Actor: "qa@example.com", Action: "CreateBucket",
		Target: "cs://pending-uploads", Detail: "private", Status: 200,
	}); err != nil {
		t.Fatal(err)
	}

	events, err := svc.AuditEvents(store.AuditFilter{Action: "CreateBucket"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Actor != "qa@example.com" {
		t.Fatalf("events = %+v", events)
	}

	count, err := svc.AuditCount(store.AuditFilter{Action: "CreateBucket"})
	if err != nil || count != 1 {
		t.Fatalf("count = %d err=%v", count, err)
	}

	since := time.Now().UTC().Add(-time.Hour)
	daily, err := svc.AuditDailyCounts(since)
	if err != nil || len(daily) != 1 {
		t.Fatalf("daily = %+v err=%v", daily, err)
	}
	statuses, err := svc.AuditStatusCounts(since)
	if err != nil || len(statuses) != 1 || statuses[0].Status != 200 {
		t.Fatalf("statuses = %+v err=%v", statuses, err)
	}
}
