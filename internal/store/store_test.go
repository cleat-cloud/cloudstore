package store

import (
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/cleat-cloud/cloudstore/internal/models"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStore_Migrations_storageTables(t *testing.T) {
	s := newTestStore(t)

	for _, table := range []string{"storage_accounts", "audit_events", "bucket_scans", "usage_daily"} {
		var name string
		if err := s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name = ?", table).Scan(&name); err != nil {
			t.Fatalf("table %s not found: %v", table, err)
		}
	}
}

func TestStorageAccounts_createActivateAndSecret(t *testing.T) {
	s := newTestStore(t)
	faker := gofakeit.New(11)

	firstID, err := s.CreateStorageAccount(models.StorageAccount{
		Label:    faker.Word() + " account",
		Provider: "b2",
		KeyID:    faker.UUID(),
		Region:   "us-west-004",
	}, "encrypted-secret")
	if err != nil {
		t.Fatal(err)
	}

	active, ok, err := s.ActiveStorageAccount()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || active.ID != firstID {
		t.Fatalf("first account should become active: ok=%v id=%d", ok, active.ID)
	}

	secret, err := s.StorageAccountSecret(firstID)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "encrypted-secret" {
		t.Fatalf("secret = %q", secret)
	}

	secondID, err := s.CreateStorageAccount(models.StorageAccount{
		Label:    faker.Word() + " backup",
		Provider: "b2",
		KeyID:    faker.UUID(),
		Region:   "eu-central-003",
	}, "another-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetActiveStorageAccount(secondID); err != nil {
		t.Fatal(err)
	}
	active, ok, err = s.ActiveStorageAccount()
	if err != nil || !ok || active.ID != secondID {
		t.Fatalf("active after switch = %+v ok=%v err=%v", active, ok, err)
	}

	if err := s.DeleteStorageAccount(secondID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.ActiveStorageAccount(); err != nil || ok {
		t.Fatalf("no account should stay active after delete: ok=%v err=%v", ok, err)
	}
	accounts, err := s.ListStorageAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 {
		t.Fatalf("accounts after delete = %d, want 1", len(accounts))
	}
}

func TestAuditEvents_insertListAndFilters(t *testing.T) {
	s := newTestStore(t)
	faker := gofakeit.New(12)
	now := time.Now().UTC()

	for i := 0; i < 5; i++ {
		_, err := s.InsertAuditEvent(models.AuditEvent{
			At:     now.Add(-time.Duration(i) * time.Hour),
			Actor:  "ops@example.com",
			Action: "CreateBucket",
			Target: "cs://" + faker.Word(),
			Status: 200,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.InsertAuditEvent(models.AuditEvent{
		At: now, Actor: "ops@example.com", Action: "DeleteObject", Target: "cs://alpha-cdn/tmp.bin", Status: 403,
	}); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListAuditEvents(AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 6 {
		t.Fatalf("events = %d, want 6", len(all))
	}
	if all[0].Status != 403 {
		t.Errorf("listing should be newest first, got status %d", all[0].Status)
	}

	created, err := s.ListAuditEvents(AuditFilter{Action: "CreateBucket"})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 5 {
		t.Errorf("action filter = %d, want 5", len(created))
	}

	failed, err := s.ListAuditEvents(AuditFilter{Status: 403})
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 {
		t.Errorf("status filter = %d, want 1", len(failed))
	}

	searched, err := s.ListAuditEvents(AuditFilter{Query: "alpha-cdn"})
	if err != nil {
		t.Fatal(err)
	}
	if len(searched) != 1 {
		t.Errorf("query filter = %d, want 1", len(searched))
	}

	count, err := s.CountAuditEvents(AuditFilter{Action: "CreateBucket"})
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}

func TestAuditCounts_dailyAndStatus(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC().Truncate(time.Minute)

	events := []struct {
		offset time.Duration
		status int
	}{
		{0, 200}, {-2 * time.Hour, 200}, {-30 * time.Hour, 404}, {-26 * time.Hour, 200},
	}
	for _, e := range events {
		if _, err := s.InsertAuditEvent(models.AuditEvent{At: now.Add(e.offset), Action: "ListObjects", Status: e.status}); err != nil {
			t.Fatal(err)
		}
	}

	days, err := s.AuditDailyCounts(now.Add(-48 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(days) < 2 {
		t.Fatalf("days = %d, want at least 2", len(days))
	}
	var total int64
	for _, d := range days {
		total += d.Count
	}
	if total != 4 {
		t.Errorf("daily total = %d, want 4", total)
	}

	statuses, err := s.AuditStatusCounts(now.Add(-48 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	byStatus := map[int]int64{}
	for _, sc := range statuses {
		byStatus[sc.Status] = sc.Count
	}
	if byStatus[200] != 3 || byStatus[404] != 1 {
		t.Errorf("status counts = %+v", byStatus)
	}
}

func TestBucketScans_upsertAndUsageHistory(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertBucketScan("alpha-cdn", 100, 5000, "Standard"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertBucketScan("beta-raw", 5, 900, "Archive"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertBucketScan("alpha-cdn", 120, 6000, "Nearline"); err != nil {
		t.Fatal(err)
	}

	scans, err := s.BucketScans()
	if err != nil {
		t.Fatal(err)
	}
	if len(scans) != 2 {
		t.Fatalf("scans = %d, want 2", len(scans))
	}
	if got := scans["alpha-cdn"]; got.Objects != 120 || got.Bytes != 6000 || got.Class != "Nearline" {
		t.Errorf("alpha-cdn scan = %+v", got)
	}

	history, err := s.UsageHistory(30)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) == 0 {
		t.Fatal("usage history is empty")
	}
	last := history[len(history)-1]
	if last.Bytes != 6900 || last.Objects != 125 {
		t.Errorf("latest usage point = %+v, want bytes 6900 objects 125", last)
	}
}
