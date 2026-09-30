package storage

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/cleat-cloud/cloudstore/internal/models"
)

var (
	ErrBucketNotFound = errors.New("bucket not found")
	ErrBucketExists   = errors.New("bucket already exists")
	ErrBucketNotEmpty = errors.New("bucket not empty")
	ErrObjectNotFound = errors.New("object not found")
	ErrInvalidName    = errors.New("invalid bucket name")
	ErrAuth           = errors.New("provider authentication failed")
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

	// Versions and file-level governance (B2 native API).
	ListFileVersions(ctx context.Context, bucket, prefix, startName, startID string, limit int) (models.FileVersionPage, error)
	HideFile(ctx context.Context, bucket, key string) error
	CopyFile(ctx context.Context, bucket, sourceKey, destKey string) error
	DeleteFileVersion(ctx context.Context, bucket, key, fileID string) error
	FileInfo(ctx context.Context, bucket, key, fileID string) (models.FileVersion, error)
	SetFileRetention(ctx context.Context, bucket, key, fileID, mode string, days int, bypassGovernance bool) error
	SetFileLegalHold(ctx context.Context, bucket, key, fileID string, on bool) error

	// Download streams one file version into w (by name, or by id when the
	// name has moved) and reports what it wrote.
	Download(ctx context.Context, bucket, key, fileID string, w io.Writer) (contentType string, size int64, err error)
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
