package b2

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cleat-cloud/cloudstore/internal/models"
	"github.com/cleat-cloud/cloudstore/internal/storage"
)

func modelUpdate() models.BucketSettingsUpdate {
	return models.BucketSettingsUpdate{
		Lifecycle: []models.LifecycleRule{{Prefix: "tmp/", HideAfterDays: 30, DeleteAfterDays: 90}},
		CORS: []models.CORSRule{{
			Name: "web", Origins: []string{"https://app.example.com"},
			Operations: []string{"b2_download_file_by_name"}, Headers: []string{"authorization"}, MaxAge: 3600,
		}},
	}
}

// stub is a fake B2 service: every handler records the request so tests can
// assert the wire contract without touching Backblaze.
type stub struct {
	t              *testing.T
	server         *httptest.Server
	authorizeCalls int
	expireNext     bool

	listBucketsBody  map[string]any
	listFilesBody    map[string]any
	updateBucketBody map[string]any
	deleteFileBody   map[string]any
	createBucketBody map[string]any
	deleteBucketBody map[string]any
	uploadURLBody    map[string]any
	uploadHeaders    http.Header
	uploadBody       string
}

func newStub(t *testing.T) *stub {
	t.Helper()
	s := &stub{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc("/b2api/v4/b2_authorize_account", s.handleAuthorize)
	mux.HandleFunc("/b2api/v4/b2_list_buckets", s.handleListBuckets)
	mux.HandleFunc("/b2api/v4/b2_list_file_names", s.handleListFiles)
	mux.HandleFunc("/b2api/v4/b2_get_upload_url", s.handleUploadURL)
	mux.HandleFunc("/b2api/v4/b2_delete_file_version", s.handleDeleteFile)
	mux.HandleFunc("/b2api/v4/b2_update_bucket", s.handleUpdateBucket)
	mux.HandleFunc("/b2api/v4/b2_create_bucket", s.handleCreateBucket)
	mux.HandleFunc("/b2api/v4/b2_delete_bucket", s.handleDeleteBucket)
	mux.HandleFunc("/b2api/v4/b2_get_download_authorization", s.handleDownloadAuthorization)
	mux.HandleFunc("/upload", s.handleUpload)
	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)
	return s
}

func (s *stub) client() *Client {
	return New(Config{
		KeyID:    "key-id",
		AppKey:   "app-key",
		Region:   "us-west-004",
		Endpoint: s.server.URL,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *stub) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	s.authorizeCalls++
	user, pass, ok := r.BasicAuth()
	if !ok || user != "key-id" || pass != "app-key" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"status": 401, "code": "unauthorized", "message": "bad key"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accountId":          "acct-1",
		"authorizationToken": "token-1",
		"apiInfo": map[string]any{
			"storageApi": map[string]any{
				"apiUrl":      s.server.URL,
				"downloadUrl": s.server.URL + "/download",
				"allowed":     map[string]any{"capabilities": []string{"listBuckets", "writeBuckets", "listFiles"}},
			},
		},
	})
}

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode body %q: %v", raw, err)
	}
	return body
}

func (s *stub) requireToken(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") != "token-1" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"status": 401, "code": "bad_auth_token", "message": "stale token"})
		return false
	}
	return true
}

func mediaBucket() map[string]any {
	return map[string]any{
		"accountId": "acct-1", "bucketId": "id-media", "bucketName": "prod-assets-media-cdn",
		"bucketType": "allPrivate", "bucketInfo": map[string]any{}, "lifecycleRules": []any{},
		"corsRules":                   []any{},
		"defaultServerSideEncryption": map[string]any{"isClientAuthorizedToRead": true, "value": map[string]any{"algorithm": "AES256", "mode": "SSE-B2"}},
		"fileLockConfiguration":       map[string]any{"isClientAuthorizedToRead": true, "value": map[string]any{"isFileLockEnabled": false, "defaultRetention": map[string]any{"mode": nil, "period": nil}}},
		"options":                     []string{"s3"}, "revision": 3,
	}
}

func backupsBucket() map[string]any {
	return map[string]any{
		"accountId": "acct-1", "bucketId": "id-backups", "bucketName": "customer-backups-glacier",
		"bucketType": "allPublic", "bucketInfo": map[string]any{},
		"lifecycleRules": []any{map[string]any{"daysFromHidingToDeleting": 180, "daysFromUploadingToHiding": 30, "fileNamePrefix": "db/"}},
		"corsRules": []any{map[string]any{
			"corsRuleName": "web", "allowedOrigins": []string{"https://app.example.com"},
			"allowedHeaders": []string{"authorization"}, "allowedOperations": []string{"b2_download_file_by_name"},
			"exposeHeaders": []string{"x-bz-content-sha1"}, "maxAgeSeconds": 3600,
		}},
		"defaultServerSideEncryption": map[string]any{"isClientAuthorizedToRead": true, "value": map[string]any{"algorithm": "AES256", "mode": "SSE-B2"}},
		"fileLockConfiguration":       map[string]any{"isClientAuthorizedToRead": true, "value": map[string]any{"isFileLockEnabled": true, "defaultRetention": map[string]any{"mode": "governance", "period": map[string]any{"duration": 7, "unit": "days"}}}},
		"options":                     []string{}, "revision": 1,
	}
}

func (s *stub) handleListBuckets(w http.ResponseWriter, r *http.Request) {
	if !s.requireToken(w, r) {
		return
	}
	s.listBucketsBody = decodeBody(s.t, r)
	if s.expireNext {
		s.expireNext = false
		writeJSON(w, http.StatusUnauthorized, map[string]any{"status": 401, "code": "expired_auth_token", "message": "expired"})
		return
	}
	buckets := []any{mediaBucket(), backupsBucket()}
	if id, ok := s.listBucketsBody["bucketId"].(string); ok {
		filtered := []any{}
		for _, b := range buckets {
			if b.(map[string]any)["bucketId"] == id {
				filtered = append(filtered, b)
			}
		}
		buckets = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"buckets": buckets})
}

func (s *stub) handleListFiles(w http.ResponseWriter, r *http.Request) {
	if !s.requireToken(w, r) {
		return
	}
	s.listFilesBody = decodeBody(s.t, r)
	start, _ := s.listFilesBody["startFileName"].(string)
	prefix, _ := s.listFilesBody["prefix"].(string)

	switch {
	case prefix == "reports/q1.csv":
		writeJSON(w, http.StatusOK, map[string]any{
			"files":        []any{uploadFile("reports/q1.csv", 1234)},
			"nextFileName": nil,
		})
	case start == "reports/q1.csv":
		writeJSON(w, http.StatusOK, map[string]any{
			"files":        []any{uploadFile("reports/q1.csv", 1234), uploadFile("reports/q2.csv", 2048)},
			"nextFileName": nil,
		})
	default:
		writeJSON(w, http.StatusOK, map[string]any{
			"files": []any{
				uploadFile("hero-launch-4k.mp4", 1424891020),
				folderRow("images/"),
				folderRow("reports/"),
			},
			"nextFileName": "reports/q1.csv",
		})
	}
}

func uploadFile(name string, size int64) map[string]any {
	return map[string]any{
		"accountId": "acct-1", "action": "upload", "bucketId": "id-media",
		"contentLength": size, "contentSha1": "dc724af18fbdd4e59189f5fe768a5f8311527050",
		"contentMd5": "9b105d4c387c4b0d8745318ef5abcf5e", "contentType": "video/mp4",
		"fileId": "file-" + name, "fileInfo": map[string]any{"x-author": "marketing-team"},
		"fileName": name, "uploadTimestamp": 1740597120000,
	}
}

func folderRow(name string) map[string]any {
	return map[string]any{"accountId": "acct-1", "action": "folder", "bucketId": "id-media", "contentLength": 0, "fileName": name, "uploadTimestamp": 0}
}

func (s *stub) handleUploadURL(w http.ResponseWriter, r *http.Request) {
	if !s.requireToken(w, r) {
		return
	}
	s.uploadURLBody = decodeBody(s.t, r)
	writeJSON(w, http.StatusOK, map[string]any{"uploadUrl": s.server.URL + "/upload", "authorizationToken": "upload-token"})
}

func (s *stub) handleUpload(w http.ResponseWriter, r *http.Request) {
	s.uploadHeaders = r.Header.Clone()
	raw, _ := io.ReadAll(r.Body)
	s.uploadBody = string(raw)
	writeJSON(w, http.StatusOK, map[string]any{"fileId": "file-1", "fileName": r.Header.Get("X-Bz-File-Name")})
}

func (s *stub) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	if !s.requireToken(w, r) {
		return
	}
	s.deleteFileBody = decodeBody(s.t, r)
	writeJSON(w, http.StatusOK, map[string]any{"fileId": s.deleteFileBody["fileId"], "fileName": s.deleteFileBody["fileName"]})
}

func (s *stub) handleUpdateBucket(w http.ResponseWriter, r *http.Request) {
	if !s.requireToken(w, r) {
		return
	}
	s.updateBucketBody = decodeBody(s.t, r)
	writeJSON(w, http.StatusOK, backupsBucket())
}

func (s *stub) handleCreateBucket(w http.ResponseWriter, r *http.Request) {
	if !s.requireToken(w, r) {
		return
	}
	s.createBucketBody = decodeBody(s.t, r)
	writeJSON(w, http.StatusOK, map[string]any{"accountId": "acct-1", "bucketId": "id-new", "bucketName": s.createBucketBody["bucketName"], "bucketType": s.createBucketBody["bucketType"], "lifecycleRules": []any{}, "options": []string{}})
}

func (s *stub) handleDeleteBucket(w http.ResponseWriter, r *http.Request) {
	if !s.requireToken(w, r) {
		return
	}
	s.deleteBucketBody = decodeBody(s.t, r)
	writeJSON(w, http.StatusOK, map[string]any{"bucketId": s.deleteBucketBody["bucketId"]})
}

func (s *stub) handleDownloadAuthorization(w http.ResponseWriter, r *http.Request) {
	if !s.requireToken(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bucketId": "id-media", "fileNamePrefix": "", "authorizationToken": "dl-token"})
}

func TestListBuckets_mapsFieldsAndSendsAccount(t *testing.T) {
	s := newStub(t)
	buckets, err := s.client().ListBuckets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 2 {
		t.Fatalf("buckets = %d, want 2", len(buckets))
	}
	if s.listBucketsBody["accountId"] != "acct-1" {
		t.Errorf("accountId sent = %v", s.listBucketsBody["accountId"])
	}
	media, backups := buckets[0], buckets[1]
	if media.Name != "prod-assets-media-cdn" || media.ID != "id-media" || media.Public {
		t.Errorf("media bucket = %+v", media)
	}
	if media.Region != "us-west-004" {
		t.Errorf("region = %q, want the configured region", media.Region)
	}
	if media.Class != "Standard" {
		t.Errorf("media class = %q, want Standard", media.Class)
	}
	if !backups.Public || backups.Class != "Archive" {
		t.Errorf("backups bucket = %+v, want public Archive", backups)
	}
}

func TestListBuckets_reauthorizesAfterExpiredToken(t *testing.T) {
	s := newStub(t)
	client := s.client()
	if _, err := client.ListBuckets(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.expireNext = true
	if _, err := client.ListBuckets(context.Background()); err != nil {
		t.Fatalf("retry after expiry failed: %v", err)
	}
	if s.authorizeCalls != 2 {
		t.Errorf("authorize calls = %d, want 2", s.authorizeCalls)
	}
}

func TestListObjects_splitsFoldersAndPaginates(t *testing.T) {
	s := newStub(t)
	client := s.client()

	page, err := client.ListObjects(context.Background(), "prod-assets-media-cdn", "", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Folders) != 2 || page.Folders[0] != "images/" || page.Folders[1] != "reports/" {
		t.Errorf("folders = %v", page.Folders)
	}
	if len(page.Objects) != 1 || page.Objects[0].Key != "hero-launch-4k.mp4" {
		t.Errorf("objects = %+v", page.Objects)
	}
	if !page.HasMore || page.NextCursor != "reports/q1.csv" {
		t.Errorf("pagination = %+v", page)
	}
	if s.listFilesBody["delimiter"] != "/" || s.listFilesBody["maxFileCount"].(float64) != 3 {
		t.Errorf("list body = %+v", s.listFilesBody)
	}
	if _, sent := s.listFilesBody["startFileName"]; sent {
		t.Errorf("first page must omit startFileName: %+v", s.listFilesBody)
	}

	next, err := client.ListObjects(context.Background(), "prod-assets-media-cdn", "", page.NextCursor, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Objects) != 2 || next.HasMore {
		t.Errorf("second page = %+v", next)
	}
	if s.listFilesBody["startFileName"] != "reports/q1.csv" {
		t.Errorf("startFileName = %v", s.listFilesBody["startFileName"])
	}
}

func TestPutObject_requestsUploadURLAndStreams(t *testing.T) {
	s := newStub(t)
	client := s.client()
	err := client.PutObject(context.Background(), "prod-assets-media-cdn", "videos/2024-campaigns/hero.mp4", strings.NewReader("payload"), 7, "video/mp4")
	if err != nil {
		t.Fatal(err)
	}
	if s.uploadURLBody["bucketId"] != "id-media" {
		t.Errorf("get_upload_url body = %+v", s.uploadURLBody)
	}
	if got := s.uploadHeaders.Get("X-Bz-File-Name"); got != "videos%2F2024-campaigns%2Fhero.mp4" {
		t.Errorf("X-Bz-File-Name = %q", got)
	}
	if got := s.uploadHeaders.Get("X-Bz-Content-Sha1"); got != "do_not_verify" {
		t.Errorf("X-Bz-Content-Sha1 = %q", got)
	}
	if got := s.uploadHeaders.Get("Content-Type"); got != "video/mp4" {
		t.Errorf("Content-Type = %q", got)
	}
	if s.uploadBody != "payload" {
		t.Errorf("upload body = %q", s.uploadBody)
	}
}

func TestDeleteObject_resolvesFileIDThenDeletes(t *testing.T) {
	s := newStub(t)
	err := s.client().DeleteObject(context.Background(), "prod-assets-media-cdn", "reports/q1.csv")
	if err != nil {
		t.Fatal(err)
	}
	if s.deleteFileBody["fileId"] != "file-reports/q1.csv" || s.deleteFileBody["fileName"] != "reports/q1.csv" {
		t.Errorf("delete body = %+v", s.deleteFileBody)
	}
}

func TestObjectDetail_returnsMetadata(t *testing.T) {
	s := newStub(t)
	detail, err := s.client().ObjectDetail(context.Background(), "prod-assets-media-cdn", "hero-launch-4k.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Size != 1424891020 || detail.ContentType != "video/mp4" {
		t.Errorf("detail = %+v", detail)
	}
	if detail.ETag != "9b105d4c387c4b0d8745318ef5abcf5e" {
		t.Errorf("etag = %q, want contentMd5", detail.ETag)
	}
	if detail.Metadata["x-author"] != "marketing-team" {
		t.Errorf("metadata = %v", detail.Metadata)
	}
	if detail.UploadedAt.IsZero() {
		t.Error("uploadedAt not parsed")
	}
}

func TestObjectDetail_missingReturnsObjectNotFound(t *testing.T) {
	s := newStub(t)
	_, err := s.client().ObjectDetail(context.Background(), "prod-assets-media-cdn", "reports/q2.csv")
	if !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("err = %v, want ErrObjectNotFound", err)
	}
}

func TestBucketSettings_mapsGovernance(t *testing.T) {
	s := newStub(t)
	settings, err := s.client().BucketSettings(context.Background(), "customer-backups-glacier")
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Public || !settings.FileLockEnabled {
		t.Errorf("settings = %+v", settings)
	}
	if settings.DefaultEncryption != "SSE-B2" {
		t.Errorf("encryption = %q", settings.DefaultEncryption)
	}
	if len(settings.Lifecycle) != 1 || settings.Lifecycle[0].HideAfterDays != 30 || settings.Lifecycle[0].DeleteAfterDays != 180 {
		t.Errorf("lifecycle = %+v", settings.Lifecycle)
	}
	if len(settings.CORS) != 1 || settings.CORS[0].MaxAge != 3600 {
		t.Errorf("cors = %+v", settings.CORS)
	}
	if settings.RetentionDays != 7 {
		t.Errorf("retention days = %d, want 7", settings.RetentionDays)
	}
}

func TestUpdateBucketSettings_sendsLifecycleAndCORS(t *testing.T) {
	s := newStub(t)
	err := s.client().UpdateBucketSettings(context.Background(), "customer-backups-glacier", modelUpdate())
	if err != nil {
		t.Fatal(err)
	}
	if s.updateBucketBody["accountId"] != "acct-1" || s.updateBucketBody["bucketId"] != "id-backups" {
		t.Errorf("update body = %+v", s.updateBucketBody)
	}
	rules, ok := s.updateBucketBody["lifecycleRules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("lifecycleRules = %v", s.updateBucketBody["lifecycleRules"])
	}
	rule := rules[0].(map[string]any)
	if rule["fileNamePrefix"] != "tmp/" || rule["daysFromUploadingToHiding"].(float64) != 30 || rule["daysFromHidingToDeleting"].(float64) != 90 {
		t.Errorf("rule = %+v", rule)
	}
	cors, ok := s.updateBucketBody["corsRules"].([]any)
	if !ok || len(cors) != 1 {
		t.Fatalf("corsRules = %v", s.updateBucketBody["corsRules"])
	}
}

func TestCreateAndDeleteBucket(t *testing.T) {
	s := newStub(t)
	client := s.client()
	created, err := client.CreateBucket(context.Background(), "pending-uploads", true)
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "pending-uploads" || !created.Public {
		t.Errorf("created = %+v", created)
	}
	if s.createBucketBody["bucketType"] != "allPublic" {
		t.Errorf("bucketType = %v", s.createBucketBody["bucketType"])
	}

	if err := client.DeleteBucket(context.Background(), "prod-assets-media-cdn"); err != nil {
		t.Fatal(err)
	}
	if s.deleteBucketBody["bucketId"] != "id-media" {
		t.Errorf("delete bucket body = %+v", s.deleteBucketBody)
	}
}

func TestDownloadURL_usesDownloadAuthorization(t *testing.T) {
	s := newStub(t)
	url, err := s.client().DownloadURL(context.Background(), "prod-assets-media-cdn", "videos/hero.mp4", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, s.server.URL+"/download/file/prod-assets-media-cdn/videos%2Fhero.mp4") {
		t.Errorf("url = %q", url)
	}
	if !strings.Contains(url, "Authorization=dl-token") {
		t.Errorf("url = %q, want the authorization token", url)
	}
}

// authResponse builds an authorize payload pinning the API to apiURL. Broken
// stubs need their own URL, so tests declare the server before the handler.
func authResponse(apiURL string) map[string]any {
	return map[string]any{
		"accountId":          "acct-1",
		"authorizationToken": "token-1",
		"apiInfo": map[string]any{
			"storageApi": map[string]any{
				"apiUrl":      apiURL,
				"downloadUrl": apiURL + "/download",
			},
		},
	}
}

func TestErrorMapping(t *testing.T) {
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/b2api/v4/b2_authorize_account", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, authResponse(server.URL))
	})
	mux.HandleFunc("/b2api/v4/b2_list_buckets", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "code": "bad_bucket_id", "message": "no such bucket"})
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	client := New(Config{KeyID: "key-id", AppKey: "app-key", Endpoint: server.URL})
	_, err := client.ListBuckets(context.Background())
	if !errors.Is(err, storage.ErrBucketNotFound) {
		t.Fatalf("err = %v, want ErrBucketNotFound", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "bad_bucket_id" {
		t.Fatalf("err = %v, want APIError{Code: bad_bucket_id}", err)
	}
}

func TestCreateBucket_mapsDuplicateName(t *testing.T) {
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/b2api/v4/b2_authorize_account", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, authResponse(server.URL))
	})
	mux.HandleFunc("/b2api/v4/b2_create_bucket", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "code": "duplicate_bucket_name", "message": "already exists"})
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	client := New(Config{KeyID: "key-id", AppKey: "app-key", Endpoint: server.URL})
	_, err := client.CreateBucket(context.Background(), "prod-assets-media-cdn", false)
	if !errors.Is(err, storage.ErrBucketExists) {
		t.Fatalf("err = %v, want ErrBucketExists", err)
	}
}
