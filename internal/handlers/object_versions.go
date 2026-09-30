package handlers

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/middleware"
)

// RegisterFileRoutes wires the real file operations on top of the objects
// screen: download through the console, server-side copy, per-version delete
// and restore (B2: download_file_by_name/_by_id, copy_file,
// delete_file_version).
func RegisterFileRoutes(r *cais.Router, consoleDeps ConsoleDeps) {
	handler := NewObjectsHandler(consoleDeps)
	authN := func(next http.HandlerFunc) http.HandlerFunc {
		return middleware.RequireAuthFunc("/login", next)
	}

	r.Get("/buckets/{name}/objects/download", authN(handler.Download))
	r.Post("/buckets/{name}/objects/copy", authN(handler.Copy))
	r.Post("/buckets/{name}/versions/delete", authN(handler.DeleteVersion))
	r.Post("/buckets/{name}/versions/restore", authN(handler.RestoreVersion))
}

// Download streams one version through the console, so private buckets can be
// downloaded without exposing the application key to the browser.
func (h *ObjectsHandler) Download(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bucket := r.PathValue("name")
	key := r.URL.Query().Get("key")
	fileID := r.URL.Query().Get("fileId")
	if key == "" {
		http.NotFound(w, r)
		return
	}

	active, err := h.deps.Console.Active(ctx)
	if err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}

	// Metadata first: headers must be set before the body starts streaming.
	info, err := active.Provider.FileInfo(ctx, bucket, key, fileID)
	if err != nil {
		h.audit(r, "DownloadObject", "b2://"+bucket+"/"+key, err.Error(), statusForError(err))
		flash.Set(w, "error", h.providerError(r, err), h.deps.Cfg.CookieSecure())
		http.Redirect(w, r, objectsPath(bucket, r.URL.Query().Get("prefix")), http.StatusSeeOther)
		return
	}

	contentType := info.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if info.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(key)))

	if _, _, err := active.Provider.Download(ctx, bucket, key, fileID, w); err != nil {
		h.audit(r, "DownloadObject", "b2://"+bucket+"/"+key, err.Error(), 502)
		return
	}
	h.audit(r, "DownloadObject", "b2://"+bucket+"/"+key, fileID, 200)
}

// Copy duplicates an object server-side (b2_copy_file).
func (h *ObjectsHandler) Copy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bucket := r.PathValue("name")
	prefix := r.URL.Query().Get("prefix")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(r.FormValue("key"))
	dest := strings.TrimSpace(r.FormValue("dest"))
	if key == "" || dest == "" {
		flash.Set(w, "error", h.t(r, "versions.copy_missing"), h.deps.Cfg.CookieSecure())
		http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
		return
	}

	active, err := h.deps.Console.Active(ctx)
	if err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}
	if err := active.Provider.CopyFile(ctx, bucket, key, dest); err != nil {
		h.audit(r, "CopyObject", "b2://"+bucket+"/"+key, err.Error(), statusForError(err))
		flash.Set(w, "error", h.providerError(r, err), h.deps.Cfg.CookieSecure())
		http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
		return
	}
	h.audit(r, "CopyObject", "b2://"+bucket+"/"+key, "→ "+dest, 200)
	flash.Set(w, "success", h.t(r, "versions.copied_flash", dest), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
}

// DeleteVersion removes one version by id (b2_delete_file_version): the way to
// reclaim storage from old versions without touching the current one.
func (h *ObjectsHandler) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bucket := r.PathValue("name")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(r.FormValue("key"))
	fileID := strings.TrimSpace(r.FormValue("fileId"))

	active, err := h.deps.Console.Active(ctx)
	if err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}
	if err := active.Provider.DeleteFileVersion(ctx, bucket, key, fileID); err != nil {
		h.audit(r, "DeleteFileVersion", "b2://"+bucket+"/"+key, err.Error(), statusForError(err))
		flash.Set(w, "error", h.providerError(r, err), h.deps.Cfg.CookieSecure())
		http.Redirect(w, r, objectsPath(bucket, r.URL.Query().Get("prefix")), http.StatusSeeOther)
		return
	}
	h.audit(r, "DeleteFileVersion", "b2://"+bucket+"/"+key, fileID, 200)
	flash.Set(w, "success", h.t(r, "versions.deleted_flash"), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, objectsPath(bucket, r.URL.Query().Get("prefix")), http.StatusSeeOther)
}

// RestoreVersion copies a chosen version back onto the object's current name
// (B2 has no in-place restore: copy_file over the same key does it).
func (h *ObjectsHandler) RestoreVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bucket := r.PathValue("name")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(r.FormValue("key"))

	active, err := h.deps.Console.Active(ctx)
	if err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}
	if err := active.Provider.CopyFile(ctx, bucket, key, key); err != nil {
		h.audit(r, "RestoreVersion", "b2://"+bucket+"/"+key, err.Error(), statusForError(err))
		flash.Set(w, "error", h.providerError(r, err), h.deps.Cfg.CookieSecure())
		http.Redirect(w, r, objectsPath(bucket, r.URL.Query().Get("prefix")), http.StatusSeeOther)
		return
	}
	h.audit(r, "RestoreVersion", "b2://"+bucket+"/"+key, "copy_file over the same key", 200)
	flash.Set(w, "success", h.t(r, "versions.restored_flash", key), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, objectsPath(bucket, r.URL.Query().Get("prefix")), http.StatusSeeOther)
}

// retentionLabel summarises an Object Lock window for the detail panel.
