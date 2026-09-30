package fakestore

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/puppe1990/cloudstore/internal/models"
	"github.com/puppe1990/cloudstore/internal/storage"
)

// Store is an in-memory storage.Provider backed by gofakeit data. It powers
// the demo mode (no B2 credentials configured) and handler tests, and its
// seed pins a reproducible dataset.
type Store struct {
	faker   *gofakeit.Faker
	mu      sync.Mutex
	buckets map[string]*bucketState
}

type bucketState struct {
	info     models.Bucket
	objects  map[string]models.ObjectInfo
	settings models.BucketSettings
}

type seedSpec struct {
	name      string
	region    string
	public    bool
	prefixes  []string
	lifecycle []models.LifecycleRule
}

// seedSpecs mirror the reference design's bucket rail with B2 regions.
var seedSpecs = []seedSpec{
	{"prod-assets-media-cdn", "us-west-004", false, []string{"videos/2024-campaigns/", "videos/", "images/banners/", "raw-footage/", "thumbnails-generated/"}, nil},
	{"analytics-lakehouse-raw", "us-west-004", false, []string{"2024/", "2025/", "temp/"}, []models.LifecycleRule{{Prefix: "temp/", HideAfterDays: 30, DeleteAfterDays: 90}}},
	{"customer-backups-glacier", "eu-central-003", false, []string{"db/", "configs/"}, []models.LifecycleRule{{Prefix: "", HideAfterDays: 30, DeleteAfterDays: 365}}},
	{"app-staging-temporary", "us-west-002", false, []string{"builds/", "tmp/"}, []models.LifecycleRule{{Prefix: "", HideAfterDays: 14}}},
	{"public-documentation-static", "us-west-002", true, []string{"docs/", "assets/"}, nil},
	{"logs-archive-2024", "eu-central-003", false, []string{"nginx/", "app/"}, []models.LifecycleRule{{Prefix: "", DeleteAfterDays: 180}}},
	{"media-transcode-queue", "us-west-004", false, []string{"incoming/", "output/"}, nil},
	{"dr-us-east-mirror", "us-east-005", false, []string{"mirror/"}, []models.LifecycleRule{{Prefix: "mirror/", HideAfterDays: 60}}},
}

// New returns a demo store with buckets and objects generated from seed.
func New(seed uint64) *Store {
	s := &Store{faker: gofakeit.New(seed), buckets: map[string]*bucketState{}}
	for _, spec := range seedSpecs {
		state := &bucketState{
			info: models.Bucket{
				ID:      fmt.Sprintf("%x", md5.Sum([]byte(spec.name)))[:12],
				Name:    spec.name,
				Region:  spec.region,
				Public:  spec.public,
				Created: s.pastTime(900),
			},
			objects: map[string]models.ObjectInfo{},
			settings: models.BucketSettings{
				Name:              spec.name,
				Public:            spec.public,
				Lifecycle:         spec.lifecycle,
				DefaultEncryption: "SSE-B2",
			},
		}
		state.info.Class = storage.ClassFromLifecycle(spec.lifecycle)
		for _, prefix := range spec.prefixes {
			for i := 0; i < s.faker.Number(3, 9); i++ {
				key := prefix + s.objectName()
				state.objects[key] = s.object(key, s.pastTime(420))
			}
		}
		s.buckets[spec.name] = state
	}
	return s
}

func (s *Store) objectName() string {
	exts := []string{"mp4", "webp", "json", "csv", "parquet", "vtt", "tar.gz", "log", "html"}
	ext := exts[s.faker.Number(0, len(exts)-1)]
	name := s.faker.Word() + "-" + s.faker.Word()
	if s.faker.Bool() {
		name += "-" + fmt.Sprint(s.faker.Number(2024, 2026))
	}
	return name + "." + ext
}

func (s *Store) pastTime(maxDays int) time.Time {
	days := s.faker.Number(0, maxDays)
	return time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour).Add(-time.Duration(s.faker.Number(0, 86400)) * time.Second)
}

func contentTypeFor(key string) string {
	switch {
	case strings.HasSuffix(key, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(key, ".webp"):
		return "image/webp"
	case strings.HasSuffix(key, ".json"):
		return "application/json"
	case strings.HasSuffix(key, ".csv"):
		return "text/csv"
	case strings.HasSuffix(key, ".parquet"):
		return "application/vnd.apache.parquet"
	case strings.HasSuffix(key, ".vtt"):
		return "text/vtt"
	case strings.HasSuffix(key, ".tar.gz"):
		return "application/gzip"
	case strings.HasSuffix(key, ".log"):
		return "text/plain"
	case strings.HasSuffix(key, ".html"):
		return "text/html"
	default:
		return "application/octet-stream"
	}
}

func (s *Store) object(key string, uploaded time.Time) models.ObjectInfo {
	return models.ObjectInfo{
		Key:          key,
		FileID:       fmt.Sprintf("%x", md5.Sum([]byte(key)))[:24],
		Size:         int64(s.faker.Number(10_000, 5_000_000_000)),
		ContentType:  contentTypeFor(key),
		UploadedAt:   uploaded,
		ETag:         fmt.Sprintf("%x", md5.Sum([]byte(key+uploaded.String()))),
		Class:        "Standard",
		CacheControl: "max-age=31536000, public",
		Metadata: map[string]string{
			"x-author":     s.faker.Username(),
			"x-encoded-by": "cloudstore-demo",
		},
	}
}

func (s *Store) ListBuckets(ctx context.Context) ([]models.Bucket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	buckets := make([]models.Bucket, 0, len(s.buckets))
	for _, state := range s.buckets {
		info := state.info
		info.Objects = int64(len(state.objects))
		for _, obj := range state.objects {
			info.Bytes += obj.Size
		}
		info.Scanned = true
		buckets = append(buckets, info)
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Name < buckets[j].Name })
	return buckets, nil
}

func (s *Store) CreateBucket(ctx context.Context, name string, public bool) (models.Bucket, error) {
	if err := storage.ValidateBucketName(name); err != nil {
		return models.Bucket{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.buckets[name]; exists {
		return models.Bucket{}, storage.ErrBucketExists
	}
	state := &bucketState{
		info: models.Bucket{
			ID:      fmt.Sprintf("%x", md5.Sum([]byte(name)))[:12],
			Name:    name,
			Region:  s.faker.RandomString([]string{"us-west-004", "us-west-002", "eu-central-003", "us-east-005"}),
			Public:  public,
			Class:   "Standard",
			Created: time.Now().UTC(),
		},
		objects:  map[string]models.ObjectInfo{},
		settings: models.BucketSettings{Name: name, Public: public, DefaultEncryption: "SSE-B2"},
	}
	s.buckets[name] = state
	return state.info, nil
}

func (s *Store) DeleteBucket(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[name]
	if !ok {
		return storage.ErrBucketNotFound
	}
	if len(state.objects) > 0 {
		return storage.ErrBucketNotEmpty
	}
	delete(s.buckets, name)
	return nil
}

func (s *Store) ListObjects(ctx context.Context, bucket, prefix, cursor string, limit int) (models.ObjectPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[bucket]
	if !ok {
		return models.ObjectPage{}, storage.ErrBucketNotFound
	}
	if limit <= 0 {
		limit = 50
	}

	var keys []string
	for key := range state.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	folderSet := map[string]bool{}
	var files []string
	for _, key := range keys {
		rest := strings.TrimPrefix(key, prefix)
		if idx := strings.Index(rest, "/"); idx >= 0 {
			folderSet[prefix+rest[:idx+1]] = true
			continue
		}
		files = append(files, key)
	}
	folders := make([]string, 0, len(folderSet))
	for folder := range folderSet {
		folders = append(folders, folder)
	}
	sort.Strings(folders)

	start, _ := strconv.Atoi(cursor)
	if start < 0 || start > len(folders)+len(files) {
		start = 0
	}
	page := models.ObjectPage{Prefix: prefix}
	end := start + limit
	if end > len(folders)+len(files) {
		end = len(folders) + len(files)
	}
	for i := start; i < end; i++ {
		if i < len(folders) {
			page.Folders = append(page.Folders, folders[i])
			continue
		}
		page.Objects = append(page.Objects, state.objects[files[i-len(folders)]])
	}
	if end < len(folders)+len(files) {
		page.HasMore = true
		page.NextCursor = strconv.Itoa(end)
	}
	return page, nil
}

func (s *Store) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[bucket]
	if !ok {
		return storage.ErrBucketNotFound
	}
	if _, err := io.Copy(io.Discard, r); err != nil {
		return fmt.Errorf("read upload body: %w", err)
	}
	if contentType == "" {
		contentType = contentTypeFor(key)
	}
	state.objects[key] = models.ObjectInfo{
		Key:         key,
		FileID:      fmt.Sprintf("%x", md5.Sum([]byte(key+time.Now().String())))[:24],
		Size:        size,
		ContentType: contentType,
		UploadedAt:  time.Now().UTC(),
		ETag:        fmt.Sprintf("%x", md5.Sum([]byte(key))),
		Class:       "Standard",
	}
	return nil
}

func (s *Store) DeleteObject(ctx context.Context, bucket, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[bucket]
	if !ok {
		return storage.ErrBucketNotFound
	}
	if _, exists := state.objects[key]; !exists {
		return storage.ErrObjectNotFound
	}
	delete(state.objects, key)
	return nil
}

func (s *Store) ObjectDetail(ctx context.Context, bucket, key string) (models.ObjectInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[bucket]
	if !ok {
		return models.ObjectInfo{}, storage.ErrBucketNotFound
	}
	info, exists := state.objects[key]
	if !exists {
		return models.ObjectInfo{}, storage.ErrObjectNotFound
	}
	return info, nil
}

func (s *Store) BucketSettings(ctx context.Context, bucket string) (models.BucketSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[bucket]
	if !ok {
		return models.BucketSettings{}, storage.ErrBucketNotFound
	}
	settings := state.settings
	settings.VersionCount = int64(len(state.objects))
	return settings, nil
}

func (s *Store) UpdateBucketSettings(ctx context.Context, bucket string, update models.BucketSettingsUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[bucket]
	if !ok {
		return storage.ErrBucketNotFound
	}
	for _, rule := range update.Lifecycle {
		if rule.HideAfterDays <= 0 && rule.DeleteAfterDays <= 0 {
			return fmt.Errorf("lifecycle rule for %q needs hide or delete days: %w", rule.Prefix, storage.ErrInvalidName)
		}
		if strings.HasPrefix(rule.Prefix, "/") {
			return fmt.Errorf("lifecycle prefix %q: %w", rule.Prefix, storage.ErrInvalidName)
		}
	}
	state.settings.Lifecycle = update.Lifecycle
	state.settings.CORS = update.CORS
	state.info.Class = storage.ClassFromLifecycle(update.Lifecycle)
	return nil
}

func (s *Store) DownloadURL(ctx context.Context, bucket, key string, ttl time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[bucket]
	if !ok {
		return "", storage.ErrBucketNotFound
	}
	if _, exists := state.objects[key]; !exists {
		return "", storage.ErrObjectNotFound
	}
	return fmt.Sprintf("https://demo-b2.cloudstore.local/file/%s/%s?Authorization=demo-%d", bucket, key, int(ttl.Seconds())), nil
}
