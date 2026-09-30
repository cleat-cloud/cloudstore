package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais"
)

// With CLOUDSTORE_SIGNUP unset the signup routes answer like the login page
// (feature flag off), for GET and POST alike.
func TestSignupDisabled_redirectsToLogin(t *testing.T) {
	handler := SignupDisabled(cais.Config{})

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(method, "/signup", nil))
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("%s status = %d, want 303", method, rr.Code)
		}
		if got := rr.Header().Get("Location"); got != "/login" {
			t.Errorf("%s location = %q, want /login", method, got)
		}
	}
}
