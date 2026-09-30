package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/cais/flash"

	"github.com/puppe1990/cloudstore/internal/format"
	"github.com/puppe1990/cloudstore/internal/models"
	"github.com/puppe1990/cloudstore/internal/storage"
)

type PoliciesHandler struct{ consoleHandler }

func NewPoliciesHandler(deps ConsoleDeps) *PoliciesHandler {
	return &PoliciesHandler{consoleHandler{deps: deps}}
}

type policyRuleVM struct {
	Prefix      string
	HideDays    string
	DeleteDays  string
	HideLabel   string
	DeleteLabel string
}

type policiesVM struct {
	Bucket string
	Tab    string
	Region string
	Class  string
	Size   string
	Public bool

	Versioning  string
	Encryption  string
	FileLock    bool
	Retention   string
	RetentionOn bool

	Lifecycle []policyRuleVM
	CORSJSON  string
	HasCORS   bool

	Error     string
	FormError string
}

func (h *PoliciesHandler) Get(w http.ResponseWriter, r *http.Request) {
	h.renderPolicies(w, r, "", 0)
}

func (h *PoliciesHandler) renderPolicies(w http.ResponseWriter, r *http.Request, formError string, status int) {
	ctx := r.Context()
	bucket := r.PathValue("name")
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "lifecycle"
	}

	active, err := h.deps.Console.Active(ctx)
	if err != nil {
		h.render(w, r, "policies", map[string]any{
			"Title":     h.t(r, "policies.title"),
			"ActiveNav": "policies",
			"Bucket":    navBucket{Name: bucket},
			"VM":        policiesVM{Bucket: bucket, Tab: tab, FormError: formError, Error: err.Error()},
		}, status)
		return
	}

	vm := policiesVM{Bucket: bucket, Tab: tab, FormError: formError, Encryption: "SSE-B2"}
	settings, err := active.Provider.BucketSettings(ctx, bucket)
	if err != nil {
		vm.Error = h.providerError(r, err)
	} else {
		vm.Public = settings.Public
		vm.Encryption = settings.DefaultEncryption
		vm.FileLock = settings.FileLockEnabled
		vm.Versioning = h.t(r, "objects.versioning_on")
		if settings.FileLockEnabled {
			vm.Versioning = h.t(r, "objects.versioning_worm")
			vm.RetentionOn = true
		}
		vm.Retention = h.t(r, "policies.encryption.none")
		if settings.RetentionDays > 0 {
			vm.Retention = h.t(r, "policies.encryption.days", settings.RetentionDays)
		}
		for _, rule := range settings.Lifecycle {
			vm.Lifecycle = append(vm.Lifecycle, policyRuleVM{
				Prefix:      rule.Prefix,
				HideDays:    dayValue(rule.HideAfterDays),
				DeleteDays:  dayValue(rule.DeleteAfterDays),
				HideLabel:   hideLabel(h.t, r, rule.HideAfterDays),
				DeleteLabel: deleteLabel(h.t, r, rule.DeleteAfterDays),
			})
		}
		if encoded, err := json.MarshalIndent(settings.CORS, "", "  "); err == nil && len(settings.CORS) > 0 {
			vm.CORSJSON = string(encoded)
			vm.HasCORS = true
		}
	}
	vm.Class = storage.ClassFromLifecycle(settings.Lifecycle)

	if buckets, err := h.deps.Console.Buckets(ctx); err == nil {
		for _, info := range buckets {
			if info.Name == bucket {
				vm.Region = info.Region
				vm.Public = info.Public
				vm.Size = format.Bytes(info.Bytes)
			}
		}
	}

	h.render(w, r, "policies", map[string]any{
		"Title":     bucket + " · " + h.t(r, "policies.title"),
		"ActiveNav": "policies",
		"Bucket":    navBucket{Name: bucket},
		"VM":        vm,
	}, status)
}

type translateFunc func(r *http.Request, key string, args ...any) string

func hideLabel(t translateFunc, r *http.Request, days int) string {
	if days <= 0 {
		return "—"
	}
	return t(r, "policies.lifecycle.hide_label", days)
}

func deleteLabel(t translateFunc, r *http.Request, days int) string {
	if days <= 0 {
		return "—"
	}
	return t(r, "policies.lifecycle.delete_label", days)
}

func dayValue(days int) string {
	if days <= 0 {
		return ""
	}
	return strconv.Itoa(days)
}

// SaveLifecycle replaces the bucket's lifecycle rules from the submitted rows.
func (h *PoliciesHandler) SaveLifecycle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bucket := r.PathValue("name")

	var rules []models.LifecycleRule
	prefixes := r.Form["rule_prefix"]
	hides := r.Form["rule_hide"]
	deletes := r.Form["rule_delete"]
	for i := range prefixes {
		hide := parseDays(dayAt(hides, i))
		del := parseDays(dayAt(deletes, i))
		prefix := strings.TrimSpace(prefixes[i])
		if hide == 0 && del == 0 {
			continue
		}
		rules = append(rules, models.LifecycleRule{Prefix: prefix, HideAfterDays: hide, DeleteAfterDays: del})
	}

	active, err := h.deps.Console.Active(r.Context())
	if err != nil {
		h.renderPolicies(w, r, err.Error(), http.StatusBadGateway)
		return
	}
	if err := active.Provider.UpdateBucketSettings(r.Context(), bucket, models.BucketSettingsUpdate{Lifecycle: rules}); err != nil {
		h.audit(r, "UpdateLifecycle", "b2://"+bucket, err.Error(), statusForError(err))
		h.renderPolicies(w, r, h.providerError(r, err), http.StatusBadGateway)
		return
	}

	h.audit(r, "UpdateLifecycle", "b2://"+bucket, strconv.Itoa(len(rules))+" rules", 200)
	flash.Set(w, "success", h.t(r, "policies.lifecycle.saved"), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, "/buckets/"+bucket+"/settings?tab=lifecycle", http.StatusSeeOther)
}

// SaveCORS validates the JSON editor payload and pushes the rules to B2.
func (h *PoliciesHandler) SaveCORS(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bucket := r.PathValue("name")
	raw := strings.TrimSpace(r.FormValue("cors_json"))

	var rules []models.CORSRule
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &rules); err != nil {
			h.renderCORS(w, r, h.t(r, "policies.cors.invalid", err.Error()), raw)
			return
		}
		for _, rule := range rules {
			if len(rule.Origins) == 0 || len(rule.Operations) == 0 {
				h.renderCORS(w, r, h.t(r, "policies.cors.invalid", "origins and allowedOperations are required"), raw)
				return
			}
		}
	}

	active, err := h.deps.Console.Active(r.Context())
	if err != nil {
		h.renderCORS(w, r, err.Error(), raw)
		return
	}
	if rules == nil {
		rules = []models.CORSRule{}
	}
	if err := active.Provider.UpdateBucketSettings(r.Context(), bucket, models.BucketSettingsUpdate{CORS: rules}); err != nil {
		h.audit(r, "UpdateCORS", "b2://"+bucket, err.Error(), statusForError(err))
		h.renderCORS(w, r, h.providerError(r, err), raw)
		return
	}

	h.audit(r, "UpdateCORS", "b2://"+bucket, strconv.Itoa(len(rules))+" rules", 200)
	flash.Set(w, "success", h.t(r, "policies.cors.saved"), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, "/buckets/"+bucket+"/settings?tab=cors", http.StatusSeeOther)
}

// renderCORS re-renders the CORS tab with the submitted (possibly invalid) JSON.
func (h *PoliciesHandler) renderCORS(w http.ResponseWriter, r *http.Request, message, raw string) {
	request := r.Clone(r.Context())
	query := request.URL.Query()
	query.Set("tab", "cors")
	request.URL.RawQuery = query.Encode()
	// The editor keeps the submitted payload so the user can fix it.
	h.renderPoliciesWithCORS(w, request, message, raw)
}

func (h *PoliciesHandler) renderPoliciesWithCORS(w http.ResponseWriter, r *http.Request, message, raw string) {
	ctx := r.Context()
	bucket := r.PathValue("name")

	active, err := h.deps.Console.Active(ctx)
	vm := policiesVM{Bucket: bucket, Tab: "cors", FormError: message, CORSJSON: raw, HasCORS: raw != "", Encryption: "SSE-B2"}
	if err == nil {
		if settings, err := active.Provider.BucketSettings(ctx, bucket); err == nil {
			vm.Public = settings.Public
			vm.Encryption = settings.DefaultEncryption
			vm.FileLock = settings.FileLockEnabled
		}
	}
	h.render(w, r, "policies", map[string]any{
		"Title":     bucket + " · " + h.t(r, "policies.title"),
		"ActiveNav": "policies",
		"Bucket":    navBucket{Name: bucket},
		"VM":        vm,
	}, http.StatusUnprocessableEntity)
}

func dayAt(values []string, index int) string {
	if index < len(values) {
		return values[index]
	}
	return ""
}

func parseDays(raw string) int {
	days, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || days < 0 {
		return 0
	}
	return days
}
