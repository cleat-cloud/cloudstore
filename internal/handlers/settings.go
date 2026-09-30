package handlers

import (
	"net/http"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"

	"github.com/puppe1990/cloudstore/internal/format"
	"github.com/puppe1990/cloudstore/internal/models"
	"github.com/puppe1990/cloudstore/internal/storage"
)

type SettingsHandler struct{ consoleHandler }

func NewSettingsHandler(deps ConsoleDeps) *SettingsHandler {
	return &SettingsHandler{consoleHandler{deps: deps}}
}

type accountVM struct {
	ID      int64
	Label   string
	KeyID   string
	Region  string
	Active  bool
	Healthy bool
	Verify  string
	Created string
}

type settingsVM struct {
	Accounts   []accountVM
	Demo       bool
	QuotaLabel string
	UsedLabel  string
	Regions    []string
	Error      string
	FormError  string
	FormLabel  string
	FormKeyID  string
	FormRegion string
	LiveVerify string
	VerifyOK   bool
}

// b2Regions are the B2 cluster regions the console can label accounts with.
var b2Regions = []string{
	"us-west-000", "us-west-001", "us-west-002", "us-west-003", "us-west-004",
	"us-east-005", "eu-central-001", "eu-central-002", "eu-central-003",
}

func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	h.renderSettings(w, r, "", 0)
}

func (h *SettingsHandler) renderSettings(w http.ResponseWriter, r *http.Request, formError string, status int) {
	ctx := r.Context()
	locale := h.locale(r)

	accounts, err := h.deps.Console.Accounts(ctx)
	vm := settingsVM{Regions: b2Regions, FormError: formError}
	if err != nil {
		vm.Error = err.Error()
	}
	for _, account := range accounts {
		vm.Accounts = append(vm.Accounts, accountVM{
			ID:      account.ID,
			Label:   account.Label,
			KeyID:   account.KeyID,
			Region:  account.Region,
			Active:  account.Active,
			Healthy: true,
			Created: format.Date(account.CreatedAt, locale),
		})
	}

	active, err := h.deps.Console.Active(ctx)
	vm.Demo = err != nil || !active.Live
	if err == nil && active.Live {
		if _, verifyErr := active.Provider.ListBuckets(ctx); verifyErr != nil {
			vm.VerifyOK = false
			vm.LiveVerify = h.t(r, "settings.verify_failed", verifyErr.Error())
			for i := range vm.Accounts {
				if vm.Accounts[i].Active {
					vm.Accounts[i].Healthy = false
					vm.Accounts[i].Verify = vm.LiveVerify
				}
			}
		} else {
			vm.VerifyOK = true
			vm.LiveVerify = h.t(r, "settings.verify_ok")
			for i := range vm.Accounts {
				if vm.Accounts[i].Active {
					vm.Accounts[i].Verify = vm.LiveVerify
				}
			}
		}
	}

	if used, quota, err := h.deps.Console.Quota(); err == nil {
		vm.UsedLabel = format.Bytes(used)
		vm.QuotaLabel = format.Bytes(quota)
	}

	h.render(w, r, "settings", map[string]any{
		"Title":     h.t(r, "settings.title"),
		"ActiveNav": "settings",
		"VM":        vm,
	}, status)
}

func (h *SettingsHandler) AddAccount(w http.ResponseWriter, r *http.Request) {
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	label := strings.TrimSpace(r.FormValue("label"))
	keyID := strings.TrimSpace(r.FormValue("key_id"))
	appKey := strings.TrimSpace(r.FormValue("app_key"))
	region := r.FormValue("region")

	var errs []string
	if label == "" {
		errs = append(errs, "label")
	}
	if keyID == "" {
		errs = append(errs, "key_id")
	}
	if appKey == "" {
		errs = append(errs, "app_key")
	}
	if len(errs) > 0 {
		h.renderSettings(w, r, "required fields: "+strings.Join(errs, ", "), http.StatusUnprocessableEntity)
		return
	}

	account, err := h.deps.Console.AddAccount(r.Context(), label, keyID, appKey, region)
	if err != nil {
		h.renderSettings(w, r, err.Error(), http.StatusBadGateway)
		return
	}
	h.audit(r, "AddAccount", "account:"+account.Label, account.KeyID, 200)
	flash.Set(w, "success", h.t(r, "settings.added_flash", account.Label), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *SettingsHandler) Activate(w http.ResponseWriter, r *http.Request) {
	id := int64(intParam(r, "id", 0))
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	account, err := h.deps.Store.FindStorageAccountByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.deps.Console.ActivateAccount(r.Context(), id); err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}
	h.audit(r, "ActivateAccount", "account:"+account.Label, account.KeyID, 200)
	flash.Set(w, "success", h.t(r, "settings.activated_flash", account.Label), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *SettingsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := int64(intParam(r, "id", 0))
	if id == 0 {
		http.NotFound(w, r)
		return
	}
	account, err := h.deps.Store.FindStorageAccountByID(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.deps.Console.DeleteAccount(r.Context(), id); err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}
	h.audit(r, "DeleteAccount", "account:"+account.Label, "", 200)
	flash.Set(w, "success", h.t(r, "settings.deleted_flash"), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

var (
	_ = models.StorageAccount{}
	_ = storage.ErrAuth
)
