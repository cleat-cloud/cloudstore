package handlers

import (
	"net/http"
	"time"

	"github.com/cleat-cloud/cloudstore/internal/format"
	"github.com/cleat-cloud/cloudstore/internal/store"
)

type AnalyticsHandler struct{ consoleHandler }

func NewAnalyticsHandler(deps ConsoleDeps) *AnalyticsHandler {
	return &AnalyticsHandler{consoleHandler{deps: deps}}
}

type statusSlice struct {
	Label   string
	Status  int
	Percent string
	Count   int64
	Kind    string
}

type logRow struct {
	Job    string
	When   string
	Detail string
	Status string
	Kind   string
}

type analyticsVM struct {
	VolumeLabel  string
	ObjectsLabel string
	OpsLabel     string
	ErrorRate    string
	ErrorHint    string

	ChartLine    string
	ChartArea    string
	ChartSamples int
	ChartObjects string
	ChartFrom    string
	ChartTo      string

	StandardLabel string
	NearlineLabel string
	ArchiveLabel  string
	StandardPct   int
	NearlinePct   int
	ArchivePct    int

	StatusTotal int64
	OKPercent   string
	DonutDash   string
	Statuses    []statusSlice

	Tasks []logRow
	Logs  []logRow
	Error string
}

func (h *AnalyticsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locale := h.locale(r)
	vm := analyticsVM{OpsLabel: "0", ErrorRate: "0%"}

	if buckets, err := h.deps.Console.Buckets(ctx); err == nil {
		split := splitByClass(buckets)
		vm.VolumeLabel = format.Bytes(split.Total)
		vm.ObjectsLabel = format.Count(totalObjects(buckets))
		vm.StandardLabel = format.Bytes(split.Standard)
		vm.NearlineLabel = format.Bytes(split.Nearline)
		vm.ArchiveLabel = format.Bytes(split.Archive)
		vm.StandardPct = percentOf(split.Standard, split.Total)
		vm.NearlinePct = percentOf(split.Nearline, split.Total)
		vm.ArchivePct = percentOf(split.Archive, split.Total)
	} else {
		vm.Error = h.providerError(r, err)
	}

	if history, err := h.deps.Console.UsageHistory(30); err == nil && len(history) > 0 {
		values := usageValues(history)
		vm.ChartLine = linePath(values, 640, 180)
		vm.ChartArea = areaPath(values, 640, 180)
		vm.ChartSamples = len(history)
		last := history[len(history)-1]
		vm.ChartObjects = format.Count(last.Objects)
		vm.ChartFrom = format.Date(history[0].Day, locale)
		vm.ChartTo = format.Date(last.Day, locale)
	}

	since := time.Now().UTC().AddDate(0, 0, -30)
	if counts, err := h.deps.Console.AuditDailyCounts(since); err == nil {
		var total int64
		for _, count := range counts {
			total += count.Count
		}
		vm.OpsLabel = format.Count(total)
	}

	if statuses, err := h.deps.Console.AuditStatusCounts(since); err == nil {
		var total, ok int64
		for _, entry := range statuses {
			total += entry.Count
			if entry.Status >= 200 && entry.Status < 300 {
				ok += entry.Count
			}
		}
		vm.StatusTotal = total
		for _, entry := range statuses {
			vm.Statuses = append(vm.Statuses, statusSlice{
				Label:   statusLabel(entry.Status),
				Status:  entry.Status,
				Percent: format.Ratio(entry.Count, total),
				Count:   entry.Count,
				Kind:    statusKind(entry.Status),
			})
		}
		if total > 0 {
			vm.OKPercent = format.Ratio(ok, total)
			// Donut circumference for r=38 in the 100x100 viewBox is 238.76.
			vm.DonutDash = format.DonutDash(ok, total, 238.76)
		}
		failed := total - ok
		vm.ErrorRate = format.Ratio(failed, total)
		if failed == 0 {
			vm.ErrorHint = h.t(r, "analytics.kpi.err_ok")
		}
	}

	if events, err := h.deps.Console.AuditEvents(store.AuditFilter{Action: "ScanBuckets", Limit: 5}); err == nil {
		for _, event := range events {
			vm.Tasks = append(vm.Tasks, logRow{
				Job:    event.Action,
				When:   format.Relative(event.At, time.Now().UTC(), locale),
				Detail: event.Detail,
				Status: statusLabel(event.Status),
				Kind:   statusKind(event.Status),
			})
		}
	}

	if events, err := h.deps.Console.AuditEvents(store.AuditFilter{Limit: 8}); err == nil {
		for _, event := range events {
			detail := event.Target
			if event.Actor != "" {
				detail = event.Actor + " · " + event.Target
			}
			vm.Logs = append(vm.Logs, logRow{
				Job:    event.Action,
				When:   format.DateTime(event.At),
				Detail: detail,
				Status: statusLabel(event.Status),
				Kind:   statusKind(event.Status),
			})
		}
	}

	h.render(w, r, "analytics", map[string]any{
		"Title":     h.t(r, "analytics.title"),
		"ActiveNav": "analytics",
		"VM":        vm,
	}, 0)
}
