package fakestore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/cleat-cloud/cloudstore/internal/models"
	"github.com/cleat-cloud/cloudstore/internal/storage"
)

// Version support keeps the demo provider honest without a full version log:
// each stored key has one synthesized "upload" version, and hide/copy/
// retention mutate the log for the operations the console performs.

type versionKey struct {
	bucket string
	key    string
}

func (s *Store) versionsFor(bucket, key string) []models.FileVersion {
	if s.versions == nil {
		s.versions = map[versionKey][]models.FileVersion{}
	}
	id := versionKey{bucket, key}
	if log, ok := s.versions[id]; ok {
		return log
	}
	info := s.buckets[bucket].objects[key]
	return []models.FileVersion{{
		Key:         key,
		FileID:      info.FileID,
		Action:      "upload",
		Size:        info.Size,
		ContentType: info.ContentType,
		UploadedAt:  info.UploadedAt,
		SHA1:        info.ETag,
		MD5:         info.ETag,
		Encryption:  "SSE-B2",
	}}
}

func (s *Store) appendVersion(bucket, key string, version models.FileVersion) {
	if s.versions == nil {
		s.versions = map[versionKey][]models.FileVersion{}
	}
	id := versionKey{bucket, key}
	s.versions[id] = append([]models.FileVersion{version}, s.versions[id]...)
}

func (s *Store) ListFileVersions(ctx context.Context, bucket, prefix, startName, startID string, limit int) (models.FileVersionPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[bucket]
	if !ok {
		return models.FileVersionPage{}, storage.ErrBucketNotFound
	}
	if limit <= 0 {
		limit = 100
	}

	var keys []string
	for key := range state.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	page := models.FileVersionPage{}
	for _, key := range keys {
		if startName != "" && key < startName {
			continue
		}
		for _, version := range s.versionsFor(bucket, key) {
			if len(page.Versions) == limit {
				page.HasMore = true
				page.NextName = version.Key
				page.NextID = version.FileID
				return page, nil
			}
			if version.Key == startName && version.FileID == startID {
				continue
			}
			page.Versions = append(page.Versions, version)
		}
	}
	return page, nil
}

func (s *Store) HideFile(ctx context.Context, bucket, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[bucket]
	if !ok {
		return storage.ErrBucketNotFound
	}
	if _, exists := state.objects[key]; !exists {
		return storage.ErrObjectNotFound
	}
	s.appendVersion(bucket, key, models.FileVersion{
		Key: key, FileID: fmt.Sprintf("%x", time.Now().UnixNano()), Action: "hide",
		UploadedAt: time.Now().UTC(), ContentType: "application/x-bz-hide-marker",
	})
	delete(state.objects, key)
	return nil
}

func (s *Store) CopyFile(ctx context.Context, bucket, sourceKey, destKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[bucket]
	if !ok {
		return storage.ErrBucketNotFound
	}
	source, exists := state.objects[sourceKey]
	if !exists {
		return storage.ErrObjectNotFound
	}
	copied := source
	copied.Key = destKey
	copied.UploadedAt = time.Now().UTC()
	state.objects[destKey] = copied
	s.appendVersion(bucket, destKey, models.FileVersion{
		Key: destKey, FileID: copied.FileID, Action: "upload", Size: copied.Size,
		ContentType: copied.ContentType, UploadedAt: copied.UploadedAt, SHA1: copied.ETag, MD5: copied.ETag,
		Encryption: "SSE-B2",
	})
	return nil
}

func (s *Store) DeleteFileVersion(ctx context.Context, bucket, key, fileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[bucket]
	if !ok {
		return storage.ErrBucketNotFound
	}
	if _, exists := state.objects[key]; !exists && fileID == "" {
		return storage.ErrObjectNotFound
	}
	delete(state.objects, key)
	if s.versions != nil {
		delete(s.versions, versionKey{bucket, key})
	}
	return nil
}

func (s *Store) FileInfo(ctx context.Context, bucket, key, fileID string) (models.FileVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.buckets[bucket]; !ok {
		return models.FileVersion{}, storage.ErrBucketNotFound
	}
	for _, version := range s.versionsFor(bucket, key) {
		if fileID == "" || version.FileID == fileID {
			return version, nil
		}
	}
	return models.FileVersion{}, storage.ErrObjectNotFound
}

func (s *Store) SetFileRetention(ctx context.Context, bucket, key, fileID, mode string, days int, bypassGovernance bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.buckets[bucket]; !ok {
		return storage.ErrBucketNotFound
	}
	log := s.versionsFor(bucket, key)
	for i := range log {
		if fileID == "" || log[i].FileID == fileID {
			log[i].Retention = mode
			s.versions[versionKey{bucket, key}] = log
			return nil
		}
	}
	return storage.ErrObjectNotFound
}

func (s *Store) SetFileLegalHold(ctx context.Context, bucket, key, fileID string, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.buckets[bucket]; !ok {
		return storage.ErrBucketNotFound
	}
	log := s.versionsFor(bucket, key)
	for i := range log {
		if fileID == "" || log[i].FileID == fileID {
			log[i].LegalHold = on
			s.versions[versionKey{bucket, key}] = log
			return nil
		}
	}
	return storage.ErrObjectNotFound
}

// Download returns a deterministic payload derived from the key so tests and
// the demo mode exercise the real streaming path.
func (s *Store) Download(ctx context.Context, bucket, key, fileID string, w io.Writer) (string, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.buckets[bucket]
	if !ok {
		return "", 0, storage.ErrBucketNotFound
	}
	info, exists := state.objects[key]
	if !exists {
		return "", 0, storage.ErrObjectNotFound
	}
	payload := []byte("demo:" + key)
	if _, err := io.Copy(w, bytes.NewReader(payload)); err != nil {
		return "", 0, fmt.Errorf("write download: %w", err)
	}
	return info.ContentType, int64(len(payload)), nil
}
