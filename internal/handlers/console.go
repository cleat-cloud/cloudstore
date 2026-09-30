package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/cleat-cloud/cloudstore/internal/console"
	"github.com/cleat-cloud/cloudstore/internal/format"
	"github.com/cleat-cloud/cloudstore/internal/models"
	"github.com/cleat-cloud/cloudstore/internal/store"
)

// ConsoleDeps is everything a console page handler needs.
type ConsoleDeps struct {
	Views   *view.Renderer
	Store   store.Store
	Console *console.Service
	Site    meta.Site
	Catalog *i18n.Catalog
	Cfg     cais.Config
}

// consoleHandler carries the shared deps and the layout data assembly.
type consoleHandler struct {
	deps ConsoleDeps
}

func (h *consoleHandler) t(r *http.Request, key string, args ...any) string {
	return i18n.CatalogOr(r, h.deps.Catalog).T(key, args...)
}

func (h *consoleHandler) locale(r *http.Request) string {
	if catalog := i18n.CatalogFromRequest(r); catalog != nil {
		return catalog.Locale()
	}
	return "en"
}

// chromeAccount is the sidebar "project context" card.
type chromeAccount struct {
	Label       string
	Region      string
	StatusLabel string
	Healthy     bool
	Live        bool
}

// chromeQuota drives the sidebar storage gauge.
type chromeQuota struct {
	Percent      int
	PercentLabel string
	UsedLabel    string
	QuotaLabel   string
}

// navBucket powers bucket-scoped sidebar links (object browser, settings).
type navBucket struct {
	Name string
}

// consoleData assembles layout chrome: it degrades gracefully when the store
// is empty so pages render on a fresh install.
func (h *consoleHandler) consoleData(ctx context.Context, r *http.Request, extra map[string]any) map[string]any {
	data := amarraData(r, h.deps.Site, extra)

	if userID, ok := session.UserID(r); ok {
		if user, err := h.deps.Store.FindUserByID(userID); err == nil {
			data["UserEmail"] = user.Email
		}
	}

	if active, err := h.deps.Console.Active(ctx); err == nil {
		account := chromeAccount{
			Label:       h.t(r, "layout.demo_label"),
			Region:      h.t(r, "layout.demo_region"),
			StatusLabel: h.t(r, "layout.demo_status"),
			Healthy:     true,
		}
		if active.Live {
			account = chromeAccount{
				Label:       active.Account.Label,
				Region:      strings.ToUpper(active.Account.Provider) + " · " + active.Account.Region,
				StatusLabel: h.t(r, "layout.connected"),
				Healthy:     true,
				Live:        true,
			}
		}
		data["Account"] = account
	}

	if used, quota, err := h.deps.Console.Quota(); err == nil && quota > 0 {
		percent := int(used * 100 / quota)
		if percent > 100 {
			percent = 100
		}
		data["Quota"] = chromeQuota{
			Percent:      percent,
			PercentLabel: format.Ratio(used, quota),
			UsedLabel:    format.Bytes(used),
			QuotaLabel:   format.Bytes(quota),
		}
	}
	return data
}

func (h *consoleHandler) render(w http.ResponseWriter, r *http.Request, name string, extra map[string]any, status int) {
	writeView(w, r, h.deps.Views, h.deps.Cfg, "app", name, h.consoleData(r.Context(), r, extra), status)
}

// audit appends one console operation. Audit failures are logged, never
// surfaced: the user action already succeeded.
func (h *consoleHandler) audit(r *http.Request, action, target, detail string, status int) {
	actor := ""
	if userID, ok := session.UserID(r); ok {
		if user, err := h.deps.Store.FindUserByID(userID); err == nil {
			actor = user.Email
		}
	}
	err := h.deps.Console.Audit(models.AuditEvent{
		At: time.Now().UTC(), Actor: actor, Action: action, Target: target, Detail: detail, Status: status,
	})
	if err != nil {
		log.Printf("audit %s %s: %v", action, target, err)
	}
}

// intParam reads a positive int query param with a fallback.
func intParam(r *http.Request, name string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

// queryString rebuilds the query string without the named params, so page
// links keep filters without duplicating the page itself.
func queryString(r *http.Request, drop ...string) string {
	values := url.Values{}
	skip := map[string]bool{}
	for _, name := range drop {
		skip[name] = true
	}
	for name, list := range r.URL.Query() {
		if skip[name] || name == "" {
			continue
		}
		for _, value := range list {
			if value != "" {
				values.Add(name, value)
			}
		}
	}
	return values.Encode()
}

// providerError renders an inline alert message for provider failures.
func (h *consoleHandler) providerError(r *http.Request, err error) string {
	message := err.Error()
	if len(message) > 240 {
		message = message[:240] + "…"
	}
	return message
}

// auditActions lists the console operations for filter selects.
func auditActions() []string {
	actions := []string{
		"CreateBucket", "DeleteBucket", "ScanBuckets", "UploadObject", "DeleteObjects",
		"CreateFolder", "UpdateLifecycle", "UpdateCORS", "AddAccount", "ActivateAccount", "DeleteAccount",
	}
	sort.Strings(actions)
	return actions
}

// statusLabel renders an audit status as "200 OK" style text.
func statusLabel(status int) string {
	text := http.StatusText(status)
	if text == "" {
		return fmt.Sprintf("%d", status)
	}
	return fmt.Sprintf("%d %s", status, text)
}
