package fakestore

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cleat-cloud/cloudstore/internal/models"
	"github.com/cleat-cloud/cloudstore/internal/storage"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	return New(42)
}

func TestNew_isDeterministicAndValid(t *testing.T) {
	first, err := newStore(t).ListBuckets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(42).ListBuckets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first) < 6 {
		t.Fatalf("demo store has %d buckets, want at least 6", len(first))
	}
	for i := range first {
		if first[i].Name != second[i].Name {
			t.Fatalf("New(42) is not reproducible: %q vs %q", first[i].Name, second[i].Name)
		}
		if err := storage.ValidateBucketName(first[i].Name); err != nil {
			t.Errorf("generated bucket %q is invalid: %v", first[i].Name, err)
		}
	}
}

func TestListBuckets_reportsScannedUsage(t *testing.T) {
	buckets, err := newStore(t).ListBuckets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seenObjects := false
	for _, b := range buckets {
		if b.Scanned && b.Objects > 0 && b.Bytes > 0 {
			seenObjects = true
		}
	}
	if !seenObjects {
		t.Error("demo buckets should carry scanned object counts and byte totals")
	}
}

func TestCreateBucket_validatesAndPersists(t *testing.T) {
	s := newStore(t)
	if _, err := s.CreateBucket(context.Background(), "Bad Name", true); !errors.Is(err, storage.ErrInvalidName) {
		t.Fatalf("invalid name error = %v, want ErrInvalidName", err)
	}

	created, err := s.CreateBucket(context.Background(), "pending-uploads", false)
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "pending-uploads" || created.Public {
		t.Fatalf("created bucket = %+v", created)
	}

	buckets, err := s.ListBuckets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range buckets {
		if b.Name == "pending-uploads" {
			found = true
		}
	}
	if !found {
		t.Error("created bucket missing from ListBuckets")
	}
}

func TestDeleteBucket_guardrails(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.DeleteBucket(ctx, "missing-bucket"); !errors.Is(err, storage.ErrBucketNotFound) {
		t.Fatalf("delete missing = %v, want ErrBucketNotFound", err)
	}

	if _, err := s.CreateBucket(ctx, "to-delete", true); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteBucket(ctx, "to-delete"); err != nil {
		t.Fatalf("delete empty bucket: %v", err)
	}
}

func TestListObjects_foldersPrefixAndPagination(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	buckets, _ := s.ListBuckets(ctx)
	bucket := ""
	for _, b := range buckets {
		if b.Objects > 3 {
			bucket = b.Name
		}
	}
	if bucket == "" {
		t.Skip("demo store has no populated bucket")
	}

	page, err := s.ListObjects(ctx, bucket, "", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Objects) > 5 {
		t.Fatalf("limit ignored: %d objects", len(page.Objects))
	}
	if len(page.Folders) == 0 {
		t.Error("expected synthesized folder rows at the root")
	}

	next, err := s.ListObjects(ctx, bucket, "", page.NextCursor, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Objects) > 0 && len(page.Objects) > 0 && next.Objects[0].Key == page.Objects[0].Key {
		t.Error("cursor pagination returned the first page again")
	}
}

func TestPutDeleteObject_roundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.CreateBucket(ctx, "uploads-bucket", false); err != nil {
		t.Fatal(err)
	}
	body := bytes.NewReader([]byte("hello world"))
	if err := s.PutObject(ctx, "uploads-bucket", "reports/q1.csv", body, 11, "text/csv"); err != nil {
		t.Fatal(err)
	}

	detail, err := s.ObjectDetail(ctx, "uploads-bucket", "reports/q1.csv")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Size != 11 || detail.ContentType != "text/csv" || detail.ETag == "" {
		t.Fatalf("detail = %+v", detail)
	}

	page, err := s.ListObjects(ctx, "uploads-bucket", "reports/", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range page.Objects {
		if o.Key == "reports/q1.csv" {
			found = true
		}
	}
	if !found {
		t.Error("uploaded object missing from prefix listing")
	}

	if err := s.DeleteObject(ctx, "uploads-bucket", "reports/q1.csv"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ObjectDetail(ctx, "uploads-bucket", "reports/q1.csv"); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("detail after delete = %v, want ErrObjectNotFound", err)
	}
}

func TestDownloadURL_mentionsObjectAndTTL(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	page, err := s.ListObjects(ctx, "prod-assets-media-cdn", "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Objects) == 0 {
		t.Skip("seeded bucket has no root objects")
	}
	key := page.Objects[0].Key

	url, err := s.DownloadURL(ctx, "prod-assets-media-cdn", key, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(url, "prod-assets-media-cdn") || !strings.Contains(url, key) {
		t.Errorf("DownloadURL = %q, want bucket and key", url)
	}
	if !strings.Contains(url, "900") {
		t.Errorf("DownloadURL = %q, want the 15 min TTL in seconds", url)
	}
}

func TestBucketSettings_updateAndClass(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.CreateBucket(ctx, "policy-bucket", false); err != nil {
		t.Fatal(err)
	}
	settings, err := s.BucketSettings(ctx, "policy-bucket")
	if err != nil {
		t.Fatal(err)
	}
	if got := storage.ClassFromLifecycle(settings.Lifecycle); got != "Standard" {
		t.Fatalf("fresh bucket class = %q", got)
	}

	update := models.BucketSettingsUpdate{
		Lifecycle: []models.LifecycleRule{{Prefix: "tmp/", HideAfterDays: 30, DeleteAfterDays: 180}},
	}
	if err := s.UpdateBucketSettings(ctx, "policy-bucket", update); err != nil {
		t.Fatal(err)
	}
	settings, err = s.BucketSettings(ctx, "policy-bucket")
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Lifecycle) != 1 || settings.Lifecycle[0].HideAfterDays != 30 {
		t.Fatalf("settings after update = %+v", settings.Lifecycle)
	}
}
