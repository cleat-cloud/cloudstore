package handlers

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/cleat-cloud/cloudstore/internal/console"
	"github.com/cleat-cloud/cloudstore/internal/crypto"
	appi18n "github.com/cleat-cloud/cloudstore/internal/i18n"
	"github.com/cleat-cloud/cloudstore/internal/models"
	"github.com/cleat-cloud/cloudstore/internal/storage"
	"github.com/cleat-cloud/cloudstore/internal/storage/fakestore"
	"github.com/cleat-cloud/cloudstore/internal/store"
)

type consoleFixture struct {
	deps ConsoleDeps
	demo *fakestore.Store
}

func newConsoleFixture(t *testing.T) consoleFixture {
	t.Helper()
	st := setupTestStore(t)
	demo := fakestore.New(42)
	deps := ConsoleDeps{
		Views: setupTestViews(t),
		Store: st,
		Console: console.New(st, crypto.DeriveKey("test-secret"), console.Options{
			QuotaBytes: 100 * 1000 * 1000 * 1000,
			Demo:       demo,
			NewProvider: func(models.StorageAccount, string) storage.Provider {
				return demo
			},
		}),
		Site:    testSite(),
		Catalog: appi18n.DefaultCatalog(),
		Cfg:     cais.Config{},
	}
	return consoleFixture{deps: deps, demo: demo}
}

// authedRequest is a request with a session user, so the layout renders the
// console chrome (rail, quota, user menu).
func authedRequest(method, target string, body *strings.Reader) *http.Request {
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return session.WithUserID(req, 1)
}

func TestBucketsHandler_List_rendersTable(t *testing.T) {
	f := newConsoleFixture(t)

	rr := httptest.NewRecorder()
	NewBucketsHandler(f.deps).List(rr, authedRequest(http.MethodGet, "/buckets", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"Cloud Storage Buckets", "prod-assets-media-cdn", "customer-backups-glacier", "Showing"} {
		if !strings.Contains(body, want) {
			t.Errorf("buckets page missing %q", want)
		}
	}
}

func TestBucketsHandler_List_filtersByName(t *testing.T) {
	f := newConsoleFixture(t)

	rr := httptest.NewRecorder()
	NewBucketsHandler(f.deps).List(rr, authedRequest(http.MethodGet, "/buckets?q=glacier", nil))

	body := rr.Body.String()
	if !strings.Contains(body, "customer-backups-glacier") {
		t.Error("filtered listing lost the matching bucket")
	}
	if strings.Contains(body, "analytics-lakehouse-raw</a>") {
		t.Error("filtered listing kept a non-matching bucket")
	}
}

func TestBucketsHandler_Create_and_Delete(t *testing.T) {
	f := newConsoleFixture(t)
	handler := NewBucketsHandler(f.deps)

	rr := httptest.NewRecorder()
	handler.Create(rr, authedRequest(http.MethodPost, "/buckets", strings.NewReader("name=pending-uploads&type=private")))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d, want 303, body: %s", rr.Code, rr.Body.String())
	}
	if _, err := f.demo.ObjectDetail(context.Background(), "pending-uploads", "anything"); err == nil {
		t.Fatal("unexpected object")
	}
	buckets, err := f.demo.ListBuckets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, bucket := range buckets {
		if bucket.Name == "pending-uploads" {
			found = true
		}
	}
	if !found {
		t.Fatal("created bucket missing from the provider")
	}

	events, err := f.deps.Console.AuditEvents(store.AuditFilter{Action: "CreateBucket"})
	if err != nil || len(events) != 1 {
		t.Fatalf("audit events = %d err=%v", len(events), err)
	}

	req := authedRequest(http.MethodPost, "/buckets/pending-uploads/delete", nil)
	req.SetPathValue("name", "pending-uploads")
	rr = httptest.NewRecorder()
	handler.Delete(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("delete status = %d, want 303", rr.Code)
	}
	buckets, _ = f.demo.ListBuckets(context.Background())
	for _, bucket := range buckets {
		if bucket.Name == "pending-uploads" {
			t.Fatal("bucket still present after delete")
		}
	}
}

func TestBucketsHandler_Create_invalidNameRenders422(t *testing.T) {
	f := newConsoleFixture(t)

	rr := httptest.NewRecorder()
	NewBucketsHandler(f.deps).Create(rr, authedRequest(http.MethodPost, "/buckets", strings.NewReader("name=Bad&type=private")))

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "6–50") {
		t.Errorf("missing validation copy, body: %s", rr.Body.String())
	}
}

func TestBucketsHandler_Refresh_storesScans(t *testing.T) {
	f := newConsoleFixture(t)

	rr := httptest.NewRecorder()
	NewBucketsHandler(f.deps).Refresh(rr, authedRequest(http.MethodPost, "/buckets/refresh", nil))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303, body: %s", rr.Code, rr.Body.String())
	}

	scans, err := f.deps.Console.Scans()
	if err != nil || len(scans) == 0 {
		t.Fatalf("scans = %d err=%v", len(scans), err)
	}
	events, err := f.deps.Console.AuditEvents(store.AuditFilter{Action: "ScanBuckets"})
	if err != nil || len(events) != 1 {
		t.Fatalf("audit events = %d err=%v", len(events), err)
	}

	rr = httptest.NewRecorder()
	NewBucketsHandler(f.deps).List(rr, authedRequest(http.MethodGet, "/buckets", nil))
	if !strings.Contains(rr.Body.String(), "Total Storage") {
		t.Error("overview missing the storage KPI")
	}
}

func TestObjectsHandler_List_detailAndPresign(t *testing.T) {
	f := newConsoleFixture(t)
	handler := NewObjectsHandler(f.deps)

	rr := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/buckets/prod-assets-media-cdn/objects", nil)
	req.SetPathValue("name", "prod-assets-media-cdn")
	handler.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Object Browser") || !strings.Contains(body, "videos/") {
		t.Errorf("objects page missing folder rail: %s", body[:200])
	}

	// Object detail with a presigned link.
	page, err := f.demo.ListObjects(context.Background(), "prod-assets-media-cdn", "videos/", "", 5)
	if err != nil || len(page.Objects) == 0 {
		t.Fatalf("seed has no videos: %v", err)
	}
	key := page.Objects[0].Key
	rr = httptest.NewRecorder()
	req = authedRequest(http.MethodGet, "/buckets/prod-assets-media-cdn/objects?prefix=videos/&key="+url.QueryEscape(key)+"&sign=15", nil)
	req.SetPathValue("name", "prod-assets-media-cdn")
	handler.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", rr.Code)
	}
	body = rr.Body.String()
	if !strings.Contains(body, "Object Details") || !strings.Contains(body, "Authorization=demo-900") {
		t.Error("detail panel missing the presigned URL")
	}
}

func TestObjectsHandler_UploadAndDelete(t *testing.T) {
	f := newConsoleFixture(t)
	handler := NewObjectsHandler(f.deps)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "report.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("a,b\n1,2\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/buckets/app-staging-temporary/objects?prefix=builds/", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = session.WithUserID(req, 1)
	req.SetPathValue("name", "app-staging-temporary")

	rr := httptest.NewRecorder()
	handler.Upload(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("upload status = %d, want 303, body: %s", rr.Code, rr.Body.String())
	}
	if _, err := f.demo.ObjectDetail(context.Background(), "app-staging-temporary", "builds/report.csv"); err != nil {
		t.Fatalf("uploaded object missing: %v", err)
	}

	deleteForm := url.Values{"key": {"builds/report.csv"}}
	req = httptest.NewRequest(http.MethodPost, "/buckets/app-staging-temporary/objects/delete", strings.NewReader(deleteForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = session.WithUserID(req, 1)
	req.SetPathValue("name", "app-staging-temporary")
	rr = httptest.NewRecorder()
	handler.DeleteSelected(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("delete status = %d, want 303", rr.Code)
	}
	if _, err := f.demo.ObjectDetail(context.Background(), "app-staging-temporary", "builds/report.csv"); err == nil {
		t.Fatal("object still present after delete")
	}
}

func TestPoliciesHandler_tabsAndLifecycleSave(t *testing.T) {
	f := newConsoleFixture(t)
	handler := NewPoliciesHandler(f.deps)

	for _, tab := range []string{"lifecycle", "cors", "encryption", "access"} {
		rr := httptest.NewRecorder()
		req := authedRequest(http.MethodGet, "/buckets/customer-backups-glacier/settings?tab="+tab, nil)
		req.SetPathValue("name", "customer-backups-glacier")
		handler.Get(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("tab %s status = %d, body: %s", tab, rr.Code, rr.Body.String())
		}
	}

	form := url.Values{
		"rule_prefix": {"tmp/"},
		"rule_hide":   {"30"},
		"rule_delete": {"90"},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/buckets/app-staging-temporary/settings/lifecycle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = session.WithUserID(req, 1)
	req.SetPathValue("name", "app-staging-temporary")
	handler.SaveLifecycle(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("save status = %d, want 303, body: %s", rr.Code, rr.Body.String())
	}

	settings, err := f.demo.BucketSettings(context.Background(), "app-staging-temporary")
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Lifecycle) != 1 || settings.Lifecycle[0].HideAfterDays != 30 || settings.Lifecycle[0].Prefix != "tmp/" {
		t.Fatalf("lifecycle = %+v", settings.Lifecycle)
	}
}

func TestPoliciesHandler_SaveCORS_invalidJSONRenders422(t *testing.T) {
	f := newConsoleFixture(t)

	form := url.Values{"cors_json": {"{not json"}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/buckets/customer-backups-glacier/settings/cors", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = session.WithUserID(req, 1)
	req.SetPathValue("name", "customer-backups-glacier")
	NewPoliciesHandler(f.deps).SaveCORS(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Invalid JSON") {
		t.Errorf("missing JSON error copy, body: %.200s", rr.Body.String())
	}
}

func TestPoliciesHandler_SaveCORS_validJSON(t *testing.T) {
	f := newConsoleFixture(t)
	payload := `[{"corsRuleName":"web","allowedOrigins":["https://app.example.com"],"allowedOperations":["b2_download_file_by_name"],"maxAgeSeconds":3600}]`

	form := url.Values{"cors_json": {payload}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/buckets/customer-backups-glacier/settings/cors", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = session.WithUserID(req, 1)
	req.SetPathValue("name", "customer-backups-glacier")
	NewPoliciesHandler(f.deps).SaveCORS(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303, body: %s", rr.Code, rr.Body.String())
	}
	settings, err := f.demo.BucketSettings(context.Background(), "customer-backups-glacier")
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.CORS) != 1 || settings.CORS[0].MaxAge != 3600 {
		t.Fatalf("cors = %+v", settings.CORS)
	}
}

func TestAnalyticsHandler_rendersAfterScan(t *testing.T) {
	f := newConsoleFixture(t)
	if _, err := f.deps.Console.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	NewAnalyticsHandler(f.deps).ServeHTTP(rr, authedRequest(http.MethodGet, "/analytics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Storage Volume Over Time") && !strings.Contains(body, "Volume de Armazenamento") {
		t.Error("analytics missing the volume chart")
	}
}

func TestAuditHandler_List_showsEvents(t *testing.T) {
	f := newConsoleFixture(t)
	if _, err := f.deps.Console.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	NewAuditHandler(f.deps).List(rr, authedRequest(http.MethodGet, "/audit?action=ScanBuckets", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "ScanBuckets") {
		t.Error("audit page missing the scan event")
	}
}

func TestSettingsHandler_accountLifecycle(t *testing.T) {
	f := newConsoleFixture(t)
	handler := NewSettingsHandler(f.deps)

	rr := httptest.NewRecorder()
	handler.Get(rr, authedRequest(http.MethodGet, "/settings", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Modo demonstração") && !strings.Contains(rr.Body.String(), "Demo mode") {
		t.Error("settings page missing the demo notice")
	}

	form := url.Values{
		"label":   {"CloudStore EU"},
		"key_id":  {"key-id-1"},
		"app_key": {"super-secret"},
		"region":  {"eu-central-003"},
	}
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/settings/accounts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = session.WithUserID(req, 1)
	handler.AddAccount(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("add status = %d, want 303, body: %s", rr.Code, rr.Body.String())
	}

	accounts, err := f.deps.Store.ListStorageAccounts()
	if err != nil || len(accounts) != 1 || !accounts[0].Active {
		t.Fatalf("accounts = %+v err=%v", accounts, err)
	}
	sealed, err := f.deps.Store.StorageAccountSecret(accounts[0].ID)
	if err != nil || sealed == "super-secret" {
		t.Fatalf("secret not encrypted: %q err=%v", sealed, err)
	}
}
