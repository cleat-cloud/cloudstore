package handlers

import (
	"net/http"
	"strconv"

	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"

	"github.com/cleat-cloud/cloudstore/internal/format"
	"github.com/cleat-cloud/cloudstore/internal/store"
)

type AuditHandler struct{ consoleHandler }

func NewAuditHandler(deps ConsoleDeps) *AuditHandler {
	return &AuditHandler{consoleHandler{deps: deps}}
}

type auditRowVM struct {
	At     string
	Actor  string
	Action string
	Target string
	Detail string
	Status string
	Kind   string
}

type auditVM struct {
	Rows       []auditRowVM
	Actions    []string
	Statuses   []int
	Filters    store.AuditFilter
	Pagination pagination
}

func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	filter := store.AuditFilter{Action: query.Get("action"), Query: query.Get("q"), Limit: 1000}
	if filter.Action == "all" {
		filter.Action = ""
	}
	if raw := query.Get("status"); raw != "" && raw != "all" {
		if status, err := strconv.Atoi(raw); err == nil && status >= 100 && status <= 599 {
			filter.Status = status
		}
	}

	events, err := h.deps.Console.AuditEvents(filter)
	if err != nil {
		httpx.ServerError(w, err, h.deps.Cfg)
		return
	}

	vm := auditVM{
		Actions:  auditActions(),
		Statuses: []int{200, 202, 403, 404, 409, 502},
		Filters:  filter,
	}
	rows, pagination := paginate(events, intParam(r, "page", 1), queryString(r, "page"))
	vm.Pagination = pagination
	for _, event := range rows {
		vm.Rows = append(vm.Rows, auditRowVM{
			At:     format.DateTime(event.At),
			Actor:  event.Actor,
			Action: event.Action,
			Target: event.Target,
			Detail: event.Detail,
			Status: statusLabel(event.Status),
			Kind:   statusKind(event.Status),
		})
	}

	h.render(w, r, "audit", map[string]any{
		"Title":     h.t(r, "audit.title"),
		"ActiveNav": "audit",
		"Query":     queryString(r, "page"),
		"VM":        vm,
	}, 0)
}
