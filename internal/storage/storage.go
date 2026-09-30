package storage

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/puppe1990/cloudstore/internal/models"
)

var (
	ErrBucketNotFound = errors.New("bucket not found")
	ErrBucketExists   = errors.New("bucket already exists")
	ErrBucketNotEmpty = errors.New("bucket not empty")
	ErrObjectNotFound = errors.New("object not found")
	ErrInvalidName    = errors.New("invalid bucket name")
)

// Provider is one object-storage backend. B2 Cloud Storage is the first
// implementation; the interface keeps the console provider-agnostic.
type Provider interface {
	ListBuckets(ctx context.Context) ([]models.Bucket, error)
	CreateBucket(ctx context.Context, name string, public bool) (models.Bucket, error)
	DeleteBucket(ctx context.Context, name string) error
	ListObjects(ctx context.Context, bucket, prefix, cursor string, limit int) (models.ObjectPage, error)
	PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error
	DeleteObject(ctx context.Context, bucket, key string) error
	ObjectDetail(ctx context.Context, bucket, key string) (models.ObjectInfo, error)
	BucketSettings(ctx context.Context, bucket string) (models.BucketSettings, error)
	UpdateBucketSettings(ctx context.Context, bucket string, update models.BucketSettingsUpdate) error
	DownloadURL(ctx context.Context, bucket, key string, ttl time.Duration) (string, error)
}

// ValidateBucketName enforces the B2 bucket name contract: 6-50 characters,
// lowercase letters, digits and hyphens, starting with a letter.
func ValidateBucketName(name string) error {
	if len(name) < 6 || len(name) > 50 {
		return ErrInvalidName
	}
	if name[0] < 'a' || name[0] > 'z' {
		return ErrInvalidName
	}
	if name[len(name)-1] == '-' {
		return ErrInvalidName
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return ErrInvalidName
	}
	return nil
}

// ClassFromLifecycle derives the console's storage class column from the
// bucket's lifecycle rules: B2 has no tiers, so the class is the retention
// intent a rule encodes.
func ClassFromLifecycle(rules []models.LifecycleRule) string {
	class := "Standard"
	for _, rule := range rules {
		if rule.DeleteAfterDays > 0 {
			return "Archive"
		}
		if rule.HideAfterDays > 0 {
			class = "Nearline"
		}
	}
	return class
}
