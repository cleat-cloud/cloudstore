package b2

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cleat-cloud/cloudstore/internal/models"
	"github.com/cleat-cloud/cloudstore/internal/storage"
)

// b2VersionFile is the b2_list_file_versions / b2_get_file_info row shape,
// including the Object Lock and encryption blocks the console surfaces.
type b2VersionFile struct {
	Action          string            `json:"action"`
	ContentLength   int64             `json:"contentLength"`
	ContentSha1     string            `json:"contentSha1"`
	ContentMD5      string            `json:"contentMd5"`
	ContentType     string            `json:"contentType"`
	FileID          string            `json:"fileId"`
	FileInfo        map[string]string `json:"fileInfo"`
	FileName        string            `json:"fileName"`
	UploadTimestamp int64             `json:"uploadTimestamp"`
	FileRetention   *struct {
		Value *struct {
			Mode                 string `json:"mode"`
			RetainUntilTimestamp int64  `json:"retainUntilTimestamp"`
		} `json:"value"`
	} `json:"fileRetention"`
	LegalHold *struct {
		Value *string `json:"value"`
	} `json:"legalHold"`
	ServerSideEncryption *struct {
		Algorithm string `json:"algorithm"`
		Mode      string `json:"mode"`
	} `json:"serverSideEncryption"`
}

func (f b2VersionFile) toVersion() models.FileVersion {
	version := models.FileVersion{
		Key:         f.FileName,
		FileID:      f.FileID,
		Action:      f.Action,
		Size:        f.ContentLength,
		ContentType: f.ContentType,
		SHA1:        f.ContentSha1,
		MD5:         f.ContentMD5,
	}
	if f.UploadTimestamp > 0 {
		version.UploadedAt = time.UnixMilli(f.UploadTimestamp).UTC()
	}
	if f.FileRetention != nil && f.FileRetention.Value != nil {
		version.Retention = f.FileRetention.Value.Mode
	}
	if f.LegalHold != nil && f.LegalHold.Value != nil {
		version.LegalHold = *f.LegalHold.Value == "on"
	}
	if f.ServerSideEncryption != nil {
		version.Encryption = f.ServerSideEncryption.Mode
	}
	return version
}

// ListFileVersions walks the bucket's version history (folders are skipped:
// the version screens work per object).
func (c *Client) ListFileVersions(ctx context.Context, bucket, prefix, startName, startID string, limit int) (models.FileVersionPage, error) {
	item, err := c.bucketID(ctx, bucket)
	if err != nil {
		return models.FileVersionPage{}, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	body := map[string]any{"bucketId": item.BucketID, "prefix": prefix, "maxFileCount": limit}
	if startName != "" {
		body["startFileName"] = startName
	}
	if startID != "" {
		body["startFileId"] = startID
	}

	var payload struct {
		Files        []b2VersionFile `json:"files"`
		NextFileName *string         `json:"nextFileName"`
		NextFileID   *string         `json:"nextFileId"`
	}
	if err := c.post(ctx, "/b2_list_file_versions", body, &payload); err != nil {
		return models.FileVersionPage{}, err
	}

	page := models.FileVersionPage{}
	for _, file := range payload.Files {
		if file.Action == "folder" {
			continue
		}
		page.Versions = append(page.Versions, file.toVersion())
	}
	if payload.NextFileName != nil && *payload.NextFileName != "" {
		page.HasMore = true
		page.NextName = *payload.NextFileName
		if payload.NextFileID != nil {
			page.NextID = *payload.NextFileID
		}
	}
	return page, nil
}

// HideFile writes a hide marker, keeping the version history intact.
func (c *Client) HideFile(ctx context.Context, bucket, key string) error {
	item, err := c.bucketID(ctx, bucket)
	if err != nil {
		return err
	}
	return c.post(ctx, "/b2_hide_file", map[string]any{"bucketId": item.BucketID, "fileName": key}, nil)
}

// CopyFile duplicates a file server-side (also used to "restore" a version).
func (c *Client) CopyFile(ctx context.Context, bucket, sourceKey, destKey string) error {
	item, err := c.bucketID(ctx, bucket)
	if err != nil {
		return err
	}
	source, err := c.findFile(ctx, bucket, sourceKey)
	if err != nil {
		return err
	}
	return c.post(ctx, "/b2_copy_file", map[string]any{
		"sourceFileId":        source.FileID,
		"fileName":            destKey,
		"destinationBucketId": item.BucketID,
	}, nil)
}

func (c *Client) DeleteFileVersion(ctx context.Context, bucket, key, fileID string) error {
	if fileID == "" {
		file, err := c.findFile(ctx, bucket, key)
		if err != nil {
			return err
		}
		fileID = file.FileID
	}
	return c.post(ctx, "/b2_delete_file_version", map[string]any{"fileName": key, "fileId": fileID}, nil)
}

// FileInfo returns the version metadata B2 exposes (sha1/md5, SSE, retention,
// legal hold); by id when the name moved, by name otherwise.
func (c *Client) FileInfo(ctx context.Context, bucket, key, fileID string) (models.FileVersion, error) {
	if fileID == "" {
		file, err := c.findFile(ctx, bucket, key)
		if err != nil {
			return models.FileVersion{}, err
		}
		fileID = file.FileID
	}
	var payload b2VersionFile
	if err := c.post(ctx, "/b2_get_file_info", map[string]any{"fileId": fileID}, &payload); err != nil {
		return models.FileVersion{}, err
	}
	return payload.toVersion(), nil
}

// SetFileRetention applies Object Lock retention to one version. Compliance
// windows cannot be shortened; governance ones need bypassGovernance.
func (c *Client) SetFileRetention(ctx context.Context, bucket, key, fileID, mode string, days int, bypassGovernance bool) error {
	if mode != "governance" && mode != "compliance" {
		return fmt.Errorf("retention mode %q: %w", mode, storage.ErrInvalidName)
	}
	body := map[string]any{
		"fileName": key,
		"fileId":   fileID,
		"fileRetention": map[string]any{
			"mode":                 mode,
			"retainUntilTimestamp": time.Now().UTC().AddDate(0, 0, days).UnixMilli(),
		},
	}
	if bypassGovernance {
		body["bypassGovernance"] = "true"
	}
	return c.post(ctx, "/b2_update_file_retention", body, nil)
}

func (c *Client) SetFileLegalHold(ctx context.Context, bucket, key, fileID string, on bool) error {
	hold := "off"
	if on {
		hold = "on"
	}
	return c.post(ctx, "/b2_update_file_legal_hold", map[string]any{
		"fileName": key, "fileId": fileID, "legalHold": hold,
	}, nil)
}

// Download streams one version through the console: by name through the
// download host, by id through the API when the name has moved.
func (c *Client) Download(ctx context.Context, bucket, key, fileID string, w io.Writer) (string, int64, error) {
	sess, err := c.session(ctx)
	if err != nil {
		return "", 0, err
	}

	url := fmt.Sprintf("%s/file/%s/%s", sess.downloadURL, bucket, encodeKey(key))
	if fileID != "" {
		url = fmt.Sprintf("%s/b2api/%s/b2_download_file_by_id?fileId=%s", sess.apiURL, apiVersion, fileID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, fmt.Errorf("b2 download: %w", err)
	}
	req.Header.Set("Authorization", sess.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("b2 download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", 0, mapError(&APIError{Status: resp.StatusCode, Code: "download_failed", Message: resp.Status})
	}

	written, err := io.Copy(w, resp.Body)
	if err != nil {
		return "", written, fmt.Errorf("b2 download stream: %w", err)
	}
	return resp.Header.Get("Content-Type"), written, nil
}
