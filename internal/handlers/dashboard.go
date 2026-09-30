package handlers

import (
	"net/http"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"

	"github.com/puppe1990/cloudstore/internal/store"
)

type DashboardHandler struct {
	views   *view.Renderer
	store   store.Store
	site    meta.Site
	catalog *i18n.Catalog
	cfg     cais.Config
}

func NewDashboardHandler(views *view.Renderer, s store.Store, site meta.Site, catalog *i18n.Catalog, cfg cais.Config) *DashboardHandler {
	return &DashboardHandler{views: views, store: s, site: site, catalog: catalog, cfg: cfg}
}

// t translates Go-side copy with the request locale (#211); h.catalog is the
// boot fallback for requests that skipped LocaleMiddleware (unit tests).
func (h *DashboardHandler) t(r *http.Request, key string, args ...any) string {
	return i18n.CatalogOr(r, h.catalog).T(key, args...)
}

func (h *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	count, err := h.store.CountContacts()
	if err != nil {
		httpx.ServerError(w, err, h.cfg)
		return
	}

	writeView(w, r, h.views, h.cfg, "app", "dashboard", amarraData(r, h.site, map[string]any{
		"Title":         h.t(r, "dashboard.title"),
		"ActiveNav":     "dashboard",
		"TotalContacts": count,
		"Env":           h.cfg.Env,
	}), 0)
}
