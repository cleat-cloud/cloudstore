package handlers

import (
	"net/http"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"
	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/cloudstore/internal/models"
	"github.com/puppe1990/cloudstore/internal/store"
)

type ContactHandler struct {
	views   *view.Renderer
	store   store.Store
	site    meta.Site
	catalog *i18n.Catalog
	cfg     cais.Config
}

func NewContactHandler(views *view.Renderer, s store.Store, site meta.Site, catalog *i18n.Catalog, cfg cais.Config) *ContactHandler {
	return &ContactHandler{views: views, store: s, site: site, catalog: catalog, cfg: cfg}
}

// t translates Go-side copy with the request locale (#211); h.catalog is the
// boot fallback for requests that skipped LocaleMiddleware (unit tests).
func (h *ContactHandler) t(r *http.Request, key string, args ...any) string {
	return i18n.CatalogOr(r, h.catalog).T(key, args...)
}

func (h *ContactHandler) Get(w http.ResponseWriter, r *http.Request) {
	writeView(w, r, h.views, h.cfg, "app", "contact", amarraData(r, h.site, map[string]any{
		"Title":     h.t(r, "contact.title"),
		"ActiveNav": "contact",
	}), 0)
}

func (h *ContactHandler) Post(w http.ResponseWriter, r *http.Request) {
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))

	var errs validate.FieldErrors
	if name == "" {
		errs.Add("name", h.t(r, "contact.name_required"))
	}
	if err := validate.Email(email); err != nil {
		msg := h.t(r, "contact.email_required")
		if email != "" {
			msg = h.t(r, "contact.email_invalid")
		}
		errs.Add("email", msg)
	}
	if errs.Any() {
		writeView(w, r, h.views, h.cfg, "app", "contact", amarraData(r, h.site, map[string]any{
			"Title":     h.t(r, "contact.title"),
			"ActiveNav": "contact",
			"Errors":    errs,
			"Name":      name,
			"Email":     email,
		}), http.StatusUnprocessableEntity)
		return
	}

	if _, err := h.store.InsertContact(models.Contact{Name: name, Email: email}); err != nil {
		httpx.ServerError(w, err, h.cfg)
		return
	}

	flash.Set(w, "success", h.t(r, "contact.success"), h.cfg.CookieSecure())
	http.Redirect(w, r, "/contact", http.StatusSeeOther)
}
