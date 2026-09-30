package handlers

import (
	"net/http"

	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
)

// SignupDisabled answers the /signup routes while the feature flag is off:
// the console is invite/owner-only until CLOUDSTORE_SIGNUP=1 turns it back on.
func SignupDisabled(cfg cais.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flash.Set(w, "error", "O cadastro está desabilitado neste ambiente. Entre com uma conta existente.", cfg.CookieSecure())
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}
