package handlers

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"

	"github.com/cleat-cloud/cloudstore/internal/console"
	"github.com/cleat-cloud/cloudstore/internal/format"
	"github.com/cleat-cloud/cloudstore/internal/models"
	"github.com/cleat-cloud/cloudstore/internal/storage"
)

type BucketsHandler struct{ consoleHandler }

func NewBucketsHandler(deps ConsoleDeps) *BucketsHandler {
	return &BucketsHandler{consoleHandler{deps: deps}}
}

// bucketRowVM is one table/grid row of the overview.
type bucketRowVM struct {
	Name    string
	Region  string
	Class   string
	Objects string
	Bytes   string
	Created string
	Public  bool
	Scanned bool
}

// bucketsVM carries everything the overview template renders.
type bucketsVM struct {
	Rows       []bucketRowVM
	Filters    bucketFilters
	Regions    []string
	Pagination pagination
	TotalAll   int

	TotalBytesLabel string
	StandardLabel   string
	NearlineLabel   string
	ArchiveLabel    string
	StandardPct     int
	NearlinePct     int
	ArchivePct      int
	BucketsCount    int
	RegionCounts    []countLabel
	ObjectsLabel    string

	OpsLabel         string
	Sparkline        string
	CostLabel        string
	LastScan         string
	MissingLifecycle int
	OptimizerBucket  string
	NeedsFirstScan   bool

	Error     string
	FormError string
	FormName  string
}

func (h *BucketsHandler) List(w http.ResponseWriter, r *http.Request) {
	h.renderOverview(w, r, "", "")
}

func (h *BucketsHandler) renderOverview(w http.ResponseWriter, r *http.Request, formError, formName string) {
	ctx := r.Context()
	locale := h.locale(r)
	filters := bucketFiltersFrom(r)

	vm := bucketsVM{Filters: filters, FormError: formError, FormName: formName}
	buckets, err := h.deps.Console.Buckets(ctx)
	if err != nil {
		vm.Error = h.providerError(r, err)
		buckets = nil
	}

	vm.TotalBytesLabel = "0 B"
	vm.BucketsCount = len(buckets)
	vm.TotalAll = len(buckets)
	vm.Regions = regionOptions(buckets)

	filtered := filterBuckets(buckets, filters)
	pageRows, pagination := paginate(filtered, intParam(r, "page", 1), queryString(r, "page"))
	vm.Pagination = pagination
	for _, bucket := range pageRows {
		vm.Rows = append(vm.Rows, bucketRow(bucket, locale))
	}

	split := splitByClass(buckets)
	vm.TotalBytesLabel = format.Bytes(split.Total)
	vm.StandardLabel = format.Bytes(split.Standard)
	vm.NearlineLabel = format.Bytes(split.Nearline)
	vm.ArchiveLabel = format.Bytes(split.Archive)
	vm.StandardPct = percentOf(split.Standard, split.Total)
	vm.NearlinePct = percentOf(split.Nearline, split.Total)
	vm.ArchivePct = percentOf(split.Archive, split.Total)
	vm.RegionCounts = regionBreakdown(buckets, 3)
	vm.ObjectsLabel = format.Count(totalObjects(buckets))
	vm.CostLabel = format.MoneyUSD(console.EstimateMonthlyCostCents(split.Total))

	if counts, err := h.deps.Console.AuditDailyCounts(time.Now().UTC().AddDate(0, 0, -30)); err == nil {
		var total int64
		values := make([]int64, 0, len(counts))
		for _, count := range counts {
			total += count.Count
			values = append(values, count.Count)
		}
		vm.OpsLabel = format.Count(total)
		vm.Sparkline = linePath(values, 56, 25)
	} else {
		vm.OpsLabel = "0"
	}

	if scans, err := h.deps.Console.Scans(); err == nil {
		var lastScan time.Time
		if len(scans) == 0 && len(buckets) > 0 {
			vm.NeedsFirstScan = true
		}
		for _, bucket := range buckets {
			usage, ok := scans[bucket.Name]
			if !ok {
				continue
			}
			if usage.ScannedAt.After(lastScan) {
				lastScan = usage.ScannedAt
			}
			// Only a scanned bucket can tell whether it has lifecycle rules.
			if usage.Class == "Standard" {
				vm.MissingLifecycle++
				if vm.OptimizerBucket == "" {
					vm.OptimizerBucket = bucket.Name
				}
			}
		}
		if !lastScan.IsZero() {
			vm.LastScan = format.Relative(lastScan, time.Now().UTC(), locale)
		}
	}

	status := 0
	if formError != "" {
		status = http.StatusUnprocessableEntity
	}
	h.render(w, r, "buckets", map[string]any{
		"Title":       h.t(r, "buckets.title"),
		"ActiveNav":   "buckets",
		"SearchQuery": filters.Query,
		"Query":       queryString(r, "page"),
		"VM":          vm,
	}, status)
}

func bucketRow(bucket models.Bucket, locale string) bucketRowVM {
	row := bucketRowVM{
		Name:    bucket.Name,
		Region:  bucket.Region,
		Class:   bucket.Class,
		Public:  bucket.Public,
		Scanned: bucket.Scanned,
		Objects: "—",
		Bytes:   "—",
		Created: "—",
	}
	if bucket.Scanned {
		row.Objects = format.Count(bucket.Objects)
		row.Bytes = format.Bytes(bucket.Bytes)
	}
	if !bucket.Created.IsZero() {
		row.Created = format.Date(bucket.Created, locale)
	}
	return row
}

func (h *BucketsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	public := r.FormValue("type") == "public"

	if err := storage.ValidateBucketName(name); err != nil {
		h.renderOverview(w, r, h.t(r, "buckets.error.invalid"), name)
		return
	}

	active, err := h.deps.Console.Active(r.Context())
	if err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}
	if _, err := active.Provider.CreateBucket(r.Context(), name, public); err != nil {
		message := h.providerError(r, err)
		if errors.Is(err, storage.ErrBucketExists) {
			message = h.t(r, "buckets.error.exists")
		}
		h.audit(r, "CreateBucket", "b2://"+name, err.Error(), statusForError(err))
		h.renderOverview(w, r, message, name)
		return
	}

	h.audit(r, "CreateBucket", "b2://"+name, accessLabel(public), 200)
	flash.Set(w, "success", h.t(r, "buckets.created_flash", name), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, "/buckets", http.StatusSeeOther)
}

func (h *BucketsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	active, err := h.deps.Console.Active(r.Context())
	if err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}

	if err := active.Provider.DeleteBucket(r.Context(), name); err != nil {
		h.audit(r, "DeleteBucket", "b2://"+name, err.Error(), statusForError(err))
		message := h.providerError(r, err)
		if errors.Is(err, storage.ErrBucketNotEmpty) {
			message = h.t(r, "buckets.error.not_empty", name)
		}
		flash.Set(w, "error", message, h.deps.Cfg.CookieSecure())
		http.Redirect(w, r, "/buckets", http.StatusSeeOther)
		return
	}

	h.audit(r, "DeleteBucket", "b2://"+name, "", 200)
	flash.Set(w, "success", h.t(r, "buckets.deleted_flash", name), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, "/buckets", http.StatusSeeOther)
}

func (h *BucketsHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	summary, err := h.deps.Console.Refresh(r.Context())
	if err != nil {
		h.audit(r, "ScanBuckets", "b2://*", err.Error(), 502)
		flash.Set(w, "error", h.providerError(r, err), h.deps.Cfg.CookieSecure())
		http.Redirect(w, r, "/buckets", http.StatusSeeOther)
		return
	}
	h.audit(r, "ScanBuckets", "b2://*", fmt.Sprintf("%d buckets · %s", summary.Buckets, format.Bytes(summary.Bytes)), 200)
	flash.Set(w, "success", h.t(r, "buckets.refreshed_flash", summary.Buckets, format.Bytes(summary.Bytes)), h.deps.Cfg.CookieSecure())
	http.Redirect(w, r, "/buckets", http.StatusSeeOther)
}

// ExportCSV streams the filtered buckets as a spreadsheet-friendly snapshot.
func (h *BucketsHandler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	buckets, err := h.deps.Console.Buckets(r.Context())
	if err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}
	buckets = filterBuckets(buckets, bucketFiltersFrom(r))

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="cloudstore-buckets.csv"`)
	writer := csv.NewWriter(w)
	defer writer.Flush()

	_ = writer.Write([]string{"name", "region", "class", "access", "objects", "bytes", "scanned", "created"})
	for _, bucket := range buckets {
		created := ""
		if !bucket.Created.IsZero() {
			created = bucket.Created.UTC().Format("2006-01-02")
		}
		_ = writer.Write([]string{
			bucket.Name, bucket.Region, bucket.Class, accessLabel(bucket.Public),
			format.Count(bucket.Objects), format.Bytes(bucket.Bytes),
			fmt.Sprintf("%t", bucket.Scanned), created,
		})
	}
	h.audit(r, "ExportCSV", "b2://*", fmt.Sprintf("%d buckets", len(buckets)), 200)
}

func accessLabel(public bool) string {
	if public {
		return "public"
	}
	return "private"
}

func statusForError(err error) int {
	switch {
	case errors.Is(err, storage.ErrBucketNotFound), errors.Is(err, storage.ErrObjectNotFound):
		return http.StatusNotFound
	case errors.Is(err, storage.ErrBucketNotEmpty), errors.Is(err, storage.ErrBucketExists), errors.Is(err, storage.ErrInvalidName):
		return http.StatusConflict
	case errors.Is(err, storage.ErrAuth):
		return http.StatusUnauthorized
	default:
		return http.StatusBadGateway
	}
}
