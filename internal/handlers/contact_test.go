package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/flash"

	appi18n "github.com/puppe1990/cloudstore/internal/i18n"
	"github.com/puppe1990/cloudstore/internal/store"
)

func newContactHandler(t *testing.T) (*ContactHandler, store.Store) {
	t.Helper()
	s := setupTestStore(t)
	h := NewContactHandler(setupTestViews(t), s, testSite(), appi18n.DefaultCatalog(), cais.Config{})
	return h, s
}

func TestContactHandler_Get_RendersForm(t *testing.T) {
	h, _ := newContactHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/contact", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), `name="email"`) {
		t.Errorf("missing email field, got: %s", rr.Body.String())
	}
}

func TestContactHandler_Post_MalformedEmail_RendersError(t *testing.T) {
	h, _ := newContactHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/contact", strings.NewReader("name=Alice&email=not-an-email"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Post(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Enter a valid email") {
		t.Errorf("missing email error, got: %s", rr.Body.String())
	}
}

func TestContactHandler_Post_MissingName_RendersError(t *testing.T) {
	h, _ := newContactHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/contact", strings.NewReader("name=&email=alice@example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Post(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Name is required") {
		t.Errorf("missing name error, got: %s", rr.Body.String())
	}
}

func TestContactHandler_Post_InvalidEmail_RendersError(t *testing.T) {
	h, _ := newContactHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/contact", strings.NewReader("name=Alice&email="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Post(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Email is required") {
		t.Errorf("missing email error, got: %s", rr.Body.String())
	}
}

func TestContactHandler_Get_RendersFlash(t *testing.T) {
	h, _ := newContactHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/contact", nil)
	req = flash.WithMessage(req, flash.Message{Kind: "success", Message: "Message sent successfully."})
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if !strings.Contains(rr.Body.String(), "Message sent successfully.") {
		t.Errorf("missing flash, got: %s", rr.Body.String())
	}
}

func TestContactHandler_Post_Valid_Redirects(t *testing.T) {
	h, s := newContactHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/contact", strings.NewReader("name=Alice&email=alice@example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Post(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rr.Code)
	}
	if rr.Header().Get("Location") != "/contact" {
		t.Errorf("Location = %q, want /contact", rr.Header().Get("Location"))
	}
	count, err := s.CountContacts()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("contact count = %d, want 1", count)
	}
}
