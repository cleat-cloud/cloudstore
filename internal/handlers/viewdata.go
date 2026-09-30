package handlers

import (
	"net/http"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"
)

func amarraData(r *http.Request, site meta.Site, extra map[string]any) map[string]any {
	s := meta.ForRequest(site, r)
	// #208: the layout gates auth chrome (rail, logout) on this flag, so a
	// visitor without a session does not see navigation that bounces them.
	_, s.LoggedIn = session.UserID(r)
	data := map[string]any{
		"Site":      s,
		"CSRFToken": s.CSRFToken,
		"Flash":     s.Flash,
	}
	if cat := i18n.CatalogFromRequest(r); cat != nil {
		data["Locale"] = cat.Locale()
	}
	for k, v := range extra {
		data[k] = v
	}
	return data
}

// writeView names the layout at the call site: an app with a second layout (a
// marketing "landing") must not render public pages inside the app chrome (#66).
func writeView(w http.ResponseWriter, r *http.Request, views *view.Renderer, cfg cais.Config, layout, name string, data any, status int) {
	view.Write(w, r, views, view.Page{Layout: layout, Name: name, Data: data, Status: status}, cfg)
}
