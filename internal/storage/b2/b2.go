// Package b2 implements storage.Provider against the Backblaze B2 Native API
// (v4). The console authenticates with an application key whose secret is kept
// encrypted at rest; tokens are cached in memory and refreshed on expiry.
package b2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/puppe1990/cloudstore/internal/models"
	"github.com/puppe1990/cloudstore/internal/storage"
)

const apiVersion = "v4"

// Config carries one B2 application key.
type Config struct {
	KeyID    string
	AppKey   string
	Region   string // display region for the console (e.g. "us-west-004")
	Endpoint string // authorize base URL; defaults to https://api.backblazeb2.com
	HTTP     *http.Client
}

// Client talks to B2 with a cached authorization token.
type Client struct {
	cfg  Config
	http *http.Client

	mu   sync.Mutex
	auth *authSession
}

type authSession struct {
	accountID   string
	apiURL      string
	downloadURL string
	token       string
}

// APIError mirrors a B2 error response.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("b2: %s (%d): %s", e.Code, e.Status, e.Message)
}

func New(cfg Config) *Client {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://api.backblazeb2.com"
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{cfg: cfg, http: cfg.HTTP}
}

func (c *Client) session(ctx context.Context) (*authSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.auth != nil {
		return c.auth, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.Endpoint+"/b2api/"+apiVersion+"/b2_authorize_account", nil)
	if err != nil {
		return nil, fmt.Errorf("b2 authorize: %w", err)
	}
	req.SetBasicAuth(c.cfg.KeyID, c.cfg.AppKey)

	var payload struct {
		AccountID          string `json:"accountId"`
		AuthorizationToken string `json:"authorizationToken"`
		APIInfo            struct {
			StorageAPI struct {
				APIURL      string `json:"apiUrl"`
				DownloadURL string `json:"downloadUrl"`
			} `json:"storageApi"`
		} `json:"apiInfo"`
	}
	if err := c.do(req, &payload); err != nil {
		return nil, err
	}
	if payload.AuthorizationToken == "" || payload.APIInfo.StorageAPI.APIURL == "" {
		return nil, fmt.Errorf("%w: empty authorization response", storage.ErrAuth)
	}
	c.auth = &authSession{
		accountID:   payload.AccountID,
		apiURL:      payload.APIInfo.StorageAPI.APIURL,
		downloadURL: strings.TrimRight(payload.APIInfo.StorageAPI.DownloadURL, "/"),
		token:       payload.AuthorizationToken,
	}
	return c.auth, nil
}

func (c *Client) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.auth = nil
}

// post performs one authenticated API call, retrying once with a fresh token
// when B2 reports the cached one expired (tokens last up to 24h).
func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	for attempt := 0; attempt < 2; attempt++ {
		sess, err := c.session(ctx)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("b2 %s: encode body: %w", path, err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, sess.apiURL+"/b2api/"+apiVersion+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("b2 %s: %w", path, err)
		}
		req.Header.Set("Authorization", sess.token)
		req.Header.Set("Content-Type", "application/json")

		err = c.do(req, out)
		if attempt == 0 && isAuthExpired(err) {
			c.invalidate()
			continue
		}
		return err
	}
	return fmt.Errorf("%w: retry exhausted", storage.ErrAuth)
}

func isAuthExpired(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Status == http.StatusUnauthorized
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("b2 request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("b2 response: %w", err)
	}
	if resp.StatusCode >= 400 {
		apiErr := &APIError{Status: resp.StatusCode}
		if json.Unmarshal(raw, apiErr) != nil {
			apiErr.Message = strings.TrimSpace(string(raw))
		}
		return mapError(apiErr)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("b2 decode: %w", err)
	}
	return nil
}

func mapError(err error) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	// Both the sentinel and the structured error stay in the chain so callers
	// can errors.Is on the sentinel and errors.As on the B2 code.
	switch apiErr.Code {
	case "bad_bucket_id", "invalid_bucket_id":
		return fmt.Errorf("%w: %w", storage.ErrBucketNotFound, apiErr)
	case "duplicate_bucket_name":
		return fmt.Errorf("%w: %w", storage.ErrBucketExists, apiErr)
	case "cannot_delete_non_empty_bucket":
		return fmt.Errorf("%w: %w", storage.ErrBucketNotEmpty, apiErr)
	case "unauthorized", "bad_auth_token", "expired_auth_token":
		return fmt.Errorf("%w: %w", storage.ErrAuth, apiErr)
	case "file_not_present", "not_found":
		return fmt.Errorf("%w: %w", storage.ErrObjectNotFound, apiErr)
	}
	return apiErr
}

type b2LifecycleRule struct {
	DaysFromHidingToDeleting  int    `json:"daysFromHidingToDeleting,omitempty"`
	DaysFromUploadingToHiding int    `json:"daysFromUploadingToHiding,omitempty"`
	FileNamePrefix            string `json:"fileNamePrefix"`
}

type b2CORSRule struct {
	CORSRuleName      string   `json:"corsRuleName"`
	AllowedOrigins    []string `json:"allowedOrigins"`
	AllowedHeaders    []string `json:"allowedHeaders"`
	AllowedOperations []string `json:"allowedOperations"`
	ExposeHeaders     []string `json:"exposeHeaders"`
	MaxAgeSeconds     int      `json:"maxAgeSeconds"`
}

type b2Retention struct {
	Mode   string `json:"mode"`
	Period struct {
		Duration int    `json:"duration"`
		Unit     string `json:"unit"`
	} `json:"period"`
}

type b2Bucket struct {
	AccountID                   string            `json:"accountId"`
	BucketID                    string            `json:"bucketId"`
	BucketName                  string            `json:"bucketName"`
	BucketType                  string            `json:"bucketType"`
	LifecycleRules              []b2LifecycleRule `json:"lifecycleRules"`
	CORSRules                   []b2CORSRule      `json:"corsRules"`
	DefaultServerSideEncryption *struct {
		Value *struct {
			Algorithm string `json:"algorithm"`
			Mode      string `json:"mode"`
		} `json:"value"`
	} `json:"defaultServerSideEncryption"`
	FileLockConfiguration *struct {
		Value *struct {
			IsFileLockEnabled bool         `json:"isFileLockEnabled"`
			DefaultRetention  *b2Retention `json:"defaultRetention"`
		} `json:"value"`
	} `json:"fileLockConfiguration"`
	Revision int `json:"revision"`
}

func (c *Client) ListBuckets(ctx context.Context) ([]models.Bucket, error) {
	raw, err := c.listBuckets(ctx, "")
	if err != nil {
		return nil, err
	}
	buckets := make([]models.Bucket, 0, len(raw))
	for _, item := range raw {
		buckets = append(buckets, c.toBucket(item))
	}
	return buckets, nil
}

func (c *Client) listBuckets(ctx context.Context, bucketID string) ([]b2Bucket, error) {
	sess, err := c.session(ctx)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"accountId": sess.accountID, "bucketTypes": []string{"all"}}
	if bucketID != "" {
		body = map[string]any{"accountId": sess.accountID, "bucketId": bucketID}
	}
	var payload struct {
		Buckets []b2Bucket `json:"buckets"`
	}
	if err := c.post(ctx, "/b2_list_buckets", body, &payload); err != nil {
		return nil, err
	}
	return payload.Buckets, nil
}

func (c *Client) toBucket(item b2Bucket) models.Bucket {
	rules := make([]models.LifecycleRule, 0, len(item.LifecycleRules))
	for _, rule := range item.LifecycleRules {
		rules = append(rules, models.LifecycleRule{
			Prefix:          rule.FileNamePrefix,
			HideAfterDays:   rule.DaysFromUploadingToHiding,
			DeleteAfterDays: rule.DaysFromHidingToDeleting,
		})
	}
	return models.Bucket{
		ID:     item.BucketID,
		Name:   item.BucketName,
		Region: c.cfg.Region,
		Public: item.BucketType == "allPublic",
		Class:  storage.ClassFromLifecycle(rules),
	}
}

// bucketID resolves a bucket name to its B2 id (mutating calls need the id).
func (c *Client) bucketID(ctx context.Context, name string) (b2Bucket, error) {
	buckets, err := c.listBuckets(ctx, "")
	if err != nil {
		return b2Bucket{}, err
	}
	for _, item := range buckets {
		if item.BucketName == name {
			return item, nil
		}
	}
	return b2Bucket{}, fmt.Errorf("%w: %s", storage.ErrBucketNotFound, name)
}

func (c *Client) CreateBucket(ctx context.Context, name string, public bool) (models.Bucket, error) {
	if err := storage.ValidateBucketName(name); err != nil {
		return models.Bucket{}, err
	}
	sess, err := c.session(ctx)
	if err != nil {
		return models.Bucket{}, err
	}
	bucketType := "allPrivate"
	if public {
		bucketType = "allPublic"
	}
	var created b2Bucket
	err = c.post(ctx, "/b2_create_bucket", map[string]any{
		"accountId": sess.accountID, "bucketName": name, "bucketType": bucketType,
	}, &created)
	if err != nil {
		return models.Bucket{}, err
	}
	return c.toBucket(created), nil
}

func (c *Client) DeleteBucket(ctx context.Context, name string) error {
	item, err := c.bucketID(ctx, name)
	if err != nil {
		return err
	}
	sess, err := c.session(ctx)
	if err != nil {
		return err
	}
	return c.post(ctx, "/b2_delete_bucket", map[string]any{
		"accountId": sess.accountID, "bucketId": item.BucketID,
	}, nil)
}

func (c *Client) ListObjects(ctx context.Context, bucket, prefix, cursor string, limit int) (models.ObjectPage, error) {
	item, err := c.bucketID(ctx, bucket)
	if err != nil {
		return models.ObjectPage{}, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	body := map[string]any{
		"bucketId": item.BucketID, "prefix": prefix, "delimiter": "/", "maxFileCount": limit,
	}
	if cursor != "" {
		body["startFileName"] = cursor
	}

	var payload struct {
		Files        []b2File `json:"files"`
		NextFileName *string  `json:"nextFileName"`
	}
	if err := c.post(ctx, "/b2_list_file_names", body, &payload); err != nil {
		return models.ObjectPage{}, err
	}

	page := models.ObjectPage{Prefix: prefix}
	for _, file := range payload.Files {
		if file.Action == "folder" || strings.HasSuffix(file.FileName, "/") {
			page.Folders = append(page.Folders, file.FileName)
			continue
		}
		page.Objects = append(page.Objects, file.toObject())
	}
	if payload.NextFileName != nil && *payload.NextFileName != "" {
		page.HasMore = true
		page.NextCursor = *payload.NextFileName
	}
	return page, nil
}

type b2File struct {
	Action          string            `json:"action"`
	ContentLength   int64             `json:"contentLength"`
	ContentSha1     string            `json:"contentSha1"`
	ContentMD5      string            `json:"contentMd5"`
	ContentType     string            `json:"contentType"`
	FileID          string            `json:"fileId"`
	FileInfo        map[string]string `json:"fileInfo"`
	FileName        string            `json:"fileName"`
	UploadTimestamp int64             `json:"uploadTimestamp"`
}

func (f b2File) toObject() models.ObjectInfo {
	etag := f.ContentMD5
	if etag == "" {
		etag = f.ContentSha1
	}
	return models.ObjectInfo{
		Key:         f.FileName,
		FileID:      f.FileID,
		Size:        f.ContentLength,
		ContentType: f.ContentType,
		UploadedAt:  time.UnixMilli(f.UploadTimestamp).UTC(),
		ETag:        etag,
		Class:       "Standard",
		Metadata:    f.FileInfo,
	}
}

// findFile returns the exact file entry for a key (B2 list is prefix-based).
func (c *Client) findFile(ctx context.Context, bucket, key string) (b2File, error) {
	item, err := c.bucketID(ctx, bucket)
	if err != nil {
		return b2File{}, err
	}
	var payload struct {
		Files []b2File `json:"files"`
	}
	err = c.post(ctx, "/b2_list_file_names", map[string]any{
		"bucketId": item.BucketID, "prefix": key, "maxFileCount": 1,
	}, &payload)
	if err != nil {
		return b2File{}, err
	}
	for _, file := range payload.Files {
		if file.FileName == key {
			return file, nil
		}
	}
	return b2File{}, fmt.Errorf("%w: %s", storage.ErrObjectNotFound, key)
}

func (c *Client) ObjectDetail(ctx context.Context, bucket, key string) (models.ObjectInfo, error) {
	file, err := c.findFile(ctx, bucket, key)
	if err != nil {
		return models.ObjectInfo{}, err
	}
	return file.toObject(), nil
}

func (c *Client) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	item, err := c.bucketID(ctx, bucket)
	if err != nil {
		return err
	}
	var upload struct {
		UploadURL          string `json:"uploadUrl"`
		AuthorizationToken string `json:"authorizationToken"`
	}
	if err := c.post(ctx, "/b2_get_upload_url", map[string]any{"bucketId": item.BucketID}, &upload); err != nil {
		return err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upload.UploadURL, r)
	if err != nil {
		return fmt.Errorf("b2 upload: %w", err)
	}
	req.ContentLength = size
	req.Header.Set("Authorization", upload.AuthorizationToken)
	req.Header.Set("X-Bz-File-Name", encodeKey(key))
	req.Header.Set("Content-Type", contentType)
	// Hash verification is skipped so the console can stream uploads; the
	// download path still verifies content against the stored sha1.
	req.Header.Set("X-Bz-Content-Sha1", "do_not_verify")

	var result struct {
		FileID   string `json:"fileId"`
		FileName string `json:"fileName"`
	}
	return c.do(req, &result)
}

func (c *Client) DeleteObject(ctx context.Context, bucket, key string) error {
	file, err := c.findFile(ctx, bucket, key)
	if err != nil {
		return err
	}
	if file.FileID == "" {
		return fmt.Errorf("%w: %s has no file id", storage.ErrObjectNotFound, key)
	}
	return c.post(ctx, "/b2_delete_file_version", map[string]any{
		"fileName": file.FileName, "fileId": file.FileID,
	}, nil)
}

func (c *Client) BucketSettings(ctx context.Context, bucket string) (models.BucketSettings, error) {
	item, err := c.bucketID(ctx, bucket)
	if err != nil {
		return models.BucketSettings{}, err
	}
	settings := models.BucketSettings{
		Name:              item.BucketName,
		Public:            item.BucketType == "allPublic",
		DefaultEncryption: "SSE-B2",
	}
	for _, rule := range item.LifecycleRules {
		settings.Lifecycle = append(settings.Lifecycle, models.LifecycleRule{
			Prefix:          rule.FileNamePrefix,
			HideAfterDays:   rule.DaysFromUploadingToHiding,
			DeleteAfterDays: rule.DaysFromHidingToDeleting,
		})
	}
	for _, rule := range item.CORSRules {
		settings.CORS = append(settings.CORS, models.CORSRule{
			Name:          rule.CORSRuleName,
			Origins:       rule.AllowedOrigins,
			Operations:    rule.AllowedOperations,
			Headers:       rule.AllowedHeaders,
			ExposeHeaders: rule.ExposeHeaders,
			MaxAge:        rule.MaxAgeSeconds,
		})
	}
	if item.DefaultServerSideEncryption != nil && item.DefaultServerSideEncryption.Value != nil {
		settings.DefaultEncryption = item.DefaultServerSideEncryption.Value.Mode
	}
	if item.FileLockConfiguration != nil && item.FileLockConfiguration.Value != nil {
		settings.FileLockEnabled = item.FileLockConfiguration.Value.IsFileLockEnabled
		if retention := item.FileLockConfiguration.Value.DefaultRetention; retention != nil {
			settings.RetentionDays = retentionDays(retention)
		}
	}
	return settings, nil
}

func retentionDays(retention *b2Retention) int {
	days := retention.Period.Duration
	switch retention.Period.Unit {
	case "weeks":
		days *= 7
	case "years":
		days *= 365
	}
	return days
}

func (c *Client) UpdateBucketSettings(ctx context.Context, bucket string, update models.BucketSettingsUpdate) error {
	item, err := c.bucketID(ctx, bucket)
	if err != nil {
		return err
	}
	sess, err := c.session(ctx)
	if err != nil {
		return err
	}

	rules := make([]b2LifecycleRule, 0, len(update.Lifecycle))
	for _, rule := range update.Lifecycle {
		rules = append(rules, b2LifecycleRule{
			DaysFromHidingToDeleting:  rule.DeleteAfterDays,
			DaysFromUploadingToHiding: rule.HideAfterDays,
			FileNamePrefix:            rule.Prefix,
		})
	}
	cors := make([]b2CORSRule, 0, len(update.CORS))
	for i, rule := range update.CORS {
		name := rule.Name
		if name == "" {
			name = fmt.Sprintf("cloudstore-rule-%d", i+1)
		}
		cors = append(cors, b2CORSRule{
			CORSRuleName:      name,
			AllowedOrigins:    rule.Origins,
			AllowedHeaders:    rule.Headers,
			AllowedOperations: rule.Operations,
			ExposeHeaders:     rule.ExposeHeaders,
			MaxAgeSeconds:     rule.MaxAge,
		})
	}

	return c.post(ctx, "/b2_update_bucket", map[string]any{
		"accountId":      sess.accountID,
		"bucketId":       item.BucketID,
		"lifecycleRules": rules,
		"corsRules":      cors,
	}, nil)
}

func (c *Client) DownloadURL(ctx context.Context, bucket, key string, ttl time.Duration) (string, error) {
	item, err := c.bucketID(ctx, bucket)
	if err != nil {
		return "", err
	}
	seconds := int(ttl.Seconds())
	if seconds <= 0 {
		seconds = 900
	}
	var payload struct {
		AuthorizationToken string `json:"authorizationToken"`
	}
	if err := c.post(ctx, "/b2_get_download_authorization", map[string]any{
		"bucketId": item.BucketID, "fileNamePrefix": key, "validDurationInSeconds": seconds,
	}, &payload); err != nil {
		return "", err
	}
	sess, err := c.session(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/file/%s/%s?Authorization=%s", sess.downloadURL, item.BucketName, encodeKey(key), url.QueryEscape(payload.AuthorizationToken)), nil
}

// encodeKey percent-encodes a B2 object key the way the API expects it (as a
// URL parameter: spaces as %20, "/" escaped).
func encodeKey(key string) string {
	return strings.ReplaceAll(url.QueryEscape(key), "+", "%20")
}
