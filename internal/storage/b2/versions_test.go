package b2

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// versionStub serves the version/copy/hide/retention/download endpoints and
// records the bodies so the tests assert the wire contract.
type versionStub struct {
	t      *testing.T
	server *httptest.Server
	body   map[string]map[string]any
}

func newVersionStub(t *testing.T) *versionStub {
	t.Helper()
	s := &versionStub{t: t, body: map[string]map[string]any{}}
	var server *httptest.Server
	mux := http.NewServeMux()
	record := func(name string, payload any) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			s.body[name] = decodeBody(t, r)
			writeJSON(w, http.StatusOK, payload)
		}
	}
	bucket := map[string]any{"bucketId": "id-media", "bucketName": "prod-assets-media-cdn", "bucketType": "allPrivate"}

	mux.HandleFunc("/b2api/v4/b2_authorize_account", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"accountId": "acct-1", "authorizationToken": "token-1",
			"apiInfo": map[string]any{"storageApi": map[string]any{"apiUrl": server.URL, "downloadUrl": server.URL + "/download"}},
		})
	})
	mux.HandleFunc("/b2api/v4/b2_list_buckets", func(w http.ResponseWriter, r *http.Request) {
		s.body["list_buckets"] = decodeBody(t, r)
		writeJSON(w, http.StatusOK, map[string]any{"buckets": []any{bucket}})
	})
	mux.HandleFunc("/b2api/v4/b2_list_file_names", record("list_files", map[string]any{
		"files": []any{map[string]any{"action": "upload", "fileName": "videos/hero.mp4", "fileId": "fid-1", "contentLength": 12, "uploadTimestamp": 1740597120000}},
	}))
	mux.HandleFunc("/b2api/v4/b2_list_file_versions", record("list_versions", map[string]any{
		"files": []any{
			map[string]any{
				"action": "upload", "fileName": "videos/hero.mp4", "fileId": "fid-2", "contentLength": 20,
				"contentSha1": "sha1-2", "contentMd5": "md5-2", "contentType": "video/mp4", "uploadTimestamp": 1740597120000,
				"fileRetention":        map[string]any{"value": map[string]any{"mode": "governance", "retainUntilTimestamp": 1790000000000}},
				"legalHold":            map[string]any{"value": "on"},
				"serverSideEncryption": map[string]any{"algorithm": "AES256", "mode": "SSE-B2"},
			},
			map[string]any{"action": "hide", "fileName": "videos/hero.mp4", "fileId": "fid-hide", "contentLength": 0, "uploadTimestamp": 1740597000000},
		},
		"nextFileName": "videos/hero.mp4",
		"nextFileId":   "fid-hide",
	}))
	mux.HandleFunc("/b2api/v4/b2_hide_file", record("hide", map[string]any{"fileId": "fid-hide"}))
	mux.HandleFunc("/b2api/v4/b2_copy_file", record("copy", map[string]any{"fileId": "fid-copy"}))
	mux.HandleFunc("/b2api/v4/b2_delete_file_version", record("delete_version", map[string]any{"fileId": "fid-2"}))
	mux.HandleFunc("/b2api/v4/b2_get_file_info", record("file_info", map[string]any{
		"action": "upload", "fileName": "videos/hero.mp4", "fileId": "fid-2", "contentLength": 20,
		"contentSha1": "sha1-2", "contentMd5": "md5-2", "contentType": "video/mp4", "uploadTimestamp": 1740597120000,
	}))
	mux.HandleFunc("/b2api/v4/b2_update_file_retention", record("retention", map[string]any{"fileId": "fid-2"}))
	mux.HandleFunc("/b2api/v4/b2_update_file_legal_hold", record("legal_hold", map[string]any{"fileId": "fid-2"}))
	mux.HandleFunc("/download/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("payload-bytes"))
	})
	mux.HandleFunc("/b2api/v4/b2_download_file_by_id", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fileId") == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "code": "bad_request", "message": "missing fileId"})
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("payload-bytes"))
	})

	server = httptest.NewServer(mux)
	s.server = server
	t.Cleanup(server.Close)
	return s
}

func (s *versionStub) client() *Client {
	return New(Config{KeyID: "key-id", AppKey: "app-key", Endpoint: s.server.URL})
}

func TestListFileVersions_mapsHistory(t *testing.T) {
	s := newVersionStub(t)
	page, err := s.client().ListFileVersions(context.Background(), "prod-assets-media-cdn", "videos/", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Versions) != 2 {
		t.Fatalf("versions = %d, want 2 (upload + hide)", len(page.Versions))
	}
	current, hidden := page.Versions[0], page.Versions[1]
	if current.SHA1 != "sha1-2" || current.MD5 != "md5-2" || current.Retention != "governance" || !current.LegalHold {
		t.Fatalf("current version = %+v", current)
	}
	if current.Encryption != "SSE-B2" {
		t.Errorf("encryption = %q", current.Encryption)
	}
	if !hidden.Hidden() {
		t.Error("hide marker not detected")
	}
	if !page.HasMore || page.NextName != "videos/hero.mp4" || page.NextID != "fid-hide" {
		t.Errorf("pagination = %+v", page)
	}
}

func TestHideCopyAndDeleteVersion_sendContract(t *testing.T) {
	s := newVersionStub(t)
	client := s.client()
	ctx := context.Background()

	if err := client.HideFile(ctx, "prod-assets-media-cdn", "videos/hero.mp4"); err != nil {
		t.Fatal(err)
	}
	if s.body["hide"]["fileName"] != "videos/hero.mp4" || s.body["hide"]["bucketId"] != "id-media" {
		t.Errorf("hide body = %+v", s.body["hide"])
	}

	if err := client.CopyFile(ctx, "prod-assets-media-cdn", "videos/hero.mp4", "videos/hero-copy.mp4"); err != nil {
		t.Fatal(err)
	}
	if s.body["copy"]["sourceFileId"] != "fid-1" || s.body["copy"]["fileName"] != "videos/hero-copy.mp4" {
		t.Errorf("copy body = %+v", s.body["copy"])
	}

	if err := client.DeleteFileVersion(ctx, "prod-assets-media-cdn", "videos/hero.mp4", "fid-2"); err != nil {
		t.Fatal(err)
	}
	if s.body["delete_version"]["fileId"] != "fid-2" || s.body["delete_version"]["fileName"] != "videos/hero.mp4" {
		t.Errorf("delete body = %+v", s.body["delete_version"])
	}
}

func TestFileRetentionAndLegalHold(t *testing.T) {
	s := newVersionStub(t)
	client := s.client()
	ctx := context.Background()

	if err := client.SetFileRetention(ctx, "b", "videos/hero.mp4", "fid-2", "governance", 30, true); err != nil {
		t.Fatal(err)
	}
	retention, ok := s.body["retention"]["fileRetention"].(map[string]any)
	if !ok || retention["mode"] != "governance" {
		t.Fatalf("retention body = %+v", s.body["retention"])
	}
	if until, ok := retention["retainUntilTimestamp"].(float64); !ok || until <= float64(time.Now().UnixMilli()) {
		t.Errorf("retainUntilTimestamp = %v, want a future timestamp", retention["retainUntilTimestamp"])
	}
	if s.body["retention"]["bypassGovernance"] != "true" {
		t.Error("bypassGovernance not forwarded")
	}
	if err := client.SetFileRetention(ctx, "b", "k", "fid", "forever", 1, false); err == nil {
		t.Error("invalid retention mode accepted")
	}

	if err := client.SetFileLegalHold(ctx, "b", "videos/hero.mp4", "fid-2", true); err != nil {
		t.Fatal(err)
	}
	if s.body["legal_hold"]["legalHold"] != "on" {
		t.Errorf("legal hold body = %+v", s.body["legal_hold"])
	}
}

func TestDownload_streamsByNameAndID(t *testing.T) {
	s := newVersionStub(t)
	client := s.client()
	ctx := context.Background()

	var byName bytes.Buffer
	contentType, size, err := client.Download(ctx, "prod-assets-media-cdn", "videos/hero.mp4", "", &byName)
	if err != nil {
		t.Fatal(err)
	}
	if byName.String() != "payload-bytes" || size != int64(len("payload-bytes")) {
		t.Fatalf("download = %q (%d bytes)", byName.String(), size)
	}
	if !strings.HasPrefix(contentType, "video/mp4") {
		t.Errorf("content type = %q", contentType)
	}

	var byID bytes.Buffer
	if _, _, err := client.Download(ctx, "prod-assets-media-cdn", "videos/hero.mp4", "fid-9", &byID); err != nil {
		t.Fatal(err)
	}
	if byID.String() != "payload-bytes" {
		t.Errorf("download by id = %q", byID.String())
	}
}
