package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"

	"github.com/puppe1990/cloudstore/internal/format"
	"github.com/puppe1990/cloudstore/internal/models"
	"github.com/puppe1990/cloudstore/internal/storage"
)

type ObjectsHandler struct{ consoleHandler }

func NewObjectsHandler(deps ConsoleDeps) *ObjectsHandler {
	return &ObjectsHandler{consoleHandler{deps: deps}}
}

// objectRowVM is one row of the object browser table.
type objectRowVM struct {
	Key      string
	Name     string
	Folder   bool
	Size     string
	Type     string
	Class    string
	Modified string
	Icon     string
	URI      string
}

type crumb struct {
	Label string
	Href  string
}

type metadataRow struct {
	Name  string
	Value string
}

type objectDetailVM struct {
	Key          string
	URI          string
	SizeLabel    string
	SizeBytes    string
	ETag         string
	ContentType  string
	CacheControl string
	Class        string
	Metadata     []metadataRow
	DownloadURL  string
	Media        string
}

type objectsVM struct {
	Bucket      string
	Prefix      string
	Breadcrumbs []crumb
	Rows        []objectRowVM
	Total       int
	VolumeLabel string
	HasMore     bool
	NextCursor  string

	Region     string
	Class      string
	Versioning string
	Encryption string
	IsPublic   bool

	Detail       *objectDetailVM
	PresignedURL string
	PresignTTL   int

	Error string
}

func (h *ObjectsHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locale := h.locale(r)
	bucket := r.PathValue("name")
	prefix := r.URL.Query().Get("prefix")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	key := r.URL.Query().Get("key")
	signMinutes := intParam(r, "sign", 0)

	active, err := h.deps.Console.Active(ctx)
	if err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}
	provider := active.Provider

	vm := objectsVM{Bucket: bucket, Prefix: prefix, Versioning: h.t(r, "objects.versioning_on")}
	settings, err := provider.BucketSettings(ctx, bucket)
	if err != nil {
		vm.Error = h.providerError(r, err)
	}
	vm.Encryption = settings.DefaultEncryption
	if settings.FileLockEnabled {
		vm.Versioning = h.t(r, "objects.versioning_worm")
	}
	vm.Class = storage.ClassFromLifecycle(settings.Lifecycle)

	if buckets, err := h.deps.Console.Buckets(ctx); err == nil {
		for _, info := range buckets {
			if info.Name == bucket {
				vm.Region = info.Region
				vm.IsPublic = info.Public
			}
		}
	}

	effectivePrefix := prefix + query
	page, err := provider.ListObjects(ctx, bucket, effectivePrefix, r.URL.Query().Get("cursor"), 50)
	if err != nil {
		vm.Error = h.providerError(r, err)
		page = models.ObjectPage{Prefix: effectivePrefix}
	}
	vm.Total = len(page.Folders) + len(page.Objects)
	vm.HasMore = page.HasMore
	vm.NextCursor = page.NextCursor
	var volume int64
	for _, folder := range page.Folders {
		name := strings.TrimPrefix(folder, effectivePrefix)
		vm.Rows = append(vm.Rows, objectRowVM{
			Key:    folder,
			Name:   name,
			Folder: true,
			Type:   h.t(r, "objects.folder"),
			Size:   "—",
			Icon:   "folder",
			URI:    "b2://" + bucket + "/" + folder,
		})
	}
	for _, object := range page.Objects {
		volume += object.Size
		vm.Rows = append(vm.Rows, objectRowVM{
			Key:      object.Key,
			Name:     strings.TrimPrefix(object.Key, effectivePrefix),
			Size:     format.Bytes(object.Size),
			Type:     object.ContentType,
			Class:    object.Class,
			Modified: format.Relative(object.UploadedAt, time.Now().UTC(), locale),
			Icon:     objectIcon(object.ContentType, object.Key),
			URI:      "b2://" + bucket + "/" + object.Key,
		})
	}
	vm.VolumeLabel = format.Bytes(volume)
	vm.Breadcrumbs = breadcrumbs(bucket, prefix)

	if key != "" {
		vm.Detail = h.objectDetail(r, provider, bucket, key, signMinutes, &vm)
	}

	h.render(w, r, "objects", map[string]any{
		"Title":       bucket + " · " + h.t(r, "objects.title"),
		"ActiveNav":   "objects",
		"Bucket":      navBucket{Name: bucket},
		"SearchQuery": query,
		"Query":       queryString(r),
		"VM":          vm,
	}, 0)
}

func (h *ObjectsHandler) objectDetail(r *http.Request, provider storage.Provider, bucket, key string, signMinutes int, vm *objectsVM) *objectDetailVM {
	ctx := r.Context()
	locale := h.locale(r)
	info, err := provider.ObjectDetail(ctx, bucket, key)
	if err != nil {
		vm.Error = h.providerError(r, err)
		return nil
	}
	detail := &objectDetailVM{
		Key:          info.Key,
		URI:          "b2://" + bucket + "/" + info.Key,
		SizeLabel:    format.Bytes(info.Size),
		SizeBytes:    format.GroupDigits(info.Size, locale),
		ETag:         info.ETag,
		ContentType:  info.ContentType,
		CacheControl: info.CacheControl,
		Class:        info.Class,
	}
	for name, value := range info.Metadata {
		detail.Metadata = append(detail.Metadata, metadataRow{Name: name, Value: value})
	}
	if strings.HasPrefix(info.ContentType, "video/") {
		detail.Media = "video"
	}
	if strings.HasPrefix(info.ContentType, "image/") {
		detail.Media = "image"
	}

	if signMinutes > 0 {
		ttl := time.Duration(signMinutes) * time.Minute
		url, err := provider.DownloadURL(ctx, bucket, key, ttl)
		if err != nil {
			vm.Error = h.providerError(r, err)
		} else {
			vm.PresignedURL = url
			vm.PresignTTL = signMinutes
			detail.DownloadURL = url
		}
	}
	if vm.IsPublic {
		detail.DownloadURL = "https://" + bucket + ".b2.cloudstore.local/" + info.Key
	}
	return detail
}

func breadcrumbs(bucket, prefix string) []crumb {
	crumbs := []crumb{{Label: "Buckets", Href: "/buckets"}}
	crumbs = append(crumbs, crumb{Label: bucket, Href: "/buckets/" + bucket + "/objects"})
	if prefix == "" {
		crumbs[len(crumbs)-1].Href = ""
		return crumbs
	}
	segments := strings.Split(strings.TrimSuffix(prefix, "/"), "/")
	path := ""
	kept := crumbs[:0:0]
	kept = append(kept, crumbs...)
	for i, segment := range segments {
		if segment == "" {
			continue
		}
		path += segment + "/"
		href := "/buckets/" + bucket + "/objects?prefix=" + path
		if i == len(segments)-1 {
			href = ""
		}
		kept = append(kept, crumb{Label: segment + "/", Href: href})
	}
	return kept
}

func (h *ObjectsHandler) Upload(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("name")
	prefix := r.URL.Query().Get("prefix")

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		flash.Set(w, "error", err.Error(), h.deps.Cfg.CookieSecure())
		http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		flash.Set(w, "error", err.Error(), h.deps.Cfg.CookieSecure())
		http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
		return
	}
	defer func() { _ = file.Close() }()

	active, err := h.deps.Console.Active(r.Context())
	if err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}
	key := prefix + strings.TrimSpace(header.Filename)
	contentType := header.Header.Get("Content-Type")
	if err := active.Provider.PutObject(r.Context(), bucket, key, file, header.Size, contentType); err != nil {
		h.audit(r, "UploadObject", "b2://"+bucket+"/"+key, err.Error(), statusForError(err))
		flash.Set(w, "error", h.providerError(r, err), h.deps.Cfg.CookieSecure())
		http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
		return
	}
	h.audit(r, "UploadObject", "b2://"+bucket+"/"+key, format.Bytes(header.Size), 200)
	flash.Set(w, "success", h.t(r, "objects.uploaded_flash", key), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
}

func (h *ObjectsHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("name")
	prefix := r.URL.Query().Get("prefix")
	name := strings.Trim(strings.TrimSpace(r.FormValue("folder")), "/")
	if name == "" {
		http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
		return
	}
	key := prefix + name + "/"

	active, err := h.deps.Console.Active(r.Context())
	if err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}
	if err := active.Provider.PutObject(r.Context(), bucket, key, strings.NewReader(""), 0, "application/x-directory"); err != nil {
		h.audit(r, "CreateFolder", "b2://"+bucket+"/"+key, err.Error(), statusForError(err))
		flash.Set(w, "error", h.providerError(r, err), h.deps.Cfg.CookieSecure())
		http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
		return
	}
	h.audit(r, "CreateFolder", "b2://"+bucket+"/"+key, "", 200)
	flash.Set(w, "success", h.t(r, "objects.folder_flash", key), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
}

func (h *ObjectsHandler) DeleteSelected(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("name")
	prefix := r.URL.Query().Get("prefix")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	keys := r.Form["key"]
	if len(keys) == 0 {
		http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
		return
	}

	active, err := h.deps.Console.Active(r.Context())
	if err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}
	deleted := 0
	for _, key := range keys {
		err := active.Provider.DeleteObject(r.Context(), bucket, key)
		status := statusForError(err)
		if err != nil && !errors.Is(err, storage.ErrObjectNotFound) {
			h.audit(r, "DeleteObjects", "b2://"+bucket+"/"+key, err.Error(), status)
			flash.Set(w, "error", h.providerError(r, err), h.deps.Cfg.CookieSecure())
			http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
			return
		}
		h.audit(r, "DeleteObjects", "b2://"+bucket+"/"+key, "", 200)
		deleted++
	}
	flash.Set(w, "success", h.t(r, "objects.deleted_flash", deleted), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, objectsPath(bucket, prefix), http.StatusSeeOther)
}

func objectsPath(bucket, prefix string) string {
	path := "/buckets/" + bucket + "/objects"
	if prefix != "" {
		path += "?prefix=" + prefix
	}
	return path
}
