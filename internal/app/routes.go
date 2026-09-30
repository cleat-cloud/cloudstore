package app

import (
	"net/http"

	"github.com/puppe1990/amarra-cais/pkg/amarra/live"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/middleware"

	"github.com/cleat-cloud/cloudstore/internal/handlers"
)

func registerRoutes(r *cais.Router, deps Deps, cfg cais.Config) {
	consoleDeps := handlers.ConsoleDeps{
		Views:   deps.Views,
		Store:   deps.Store,
		Console: deps.Console,
		Site:    deps.Site,
		Catalog: deps.Catalog,
		Cfg:     cfg,
	}

	home := handlers.NewHomeHandler(deps.Views, deps.Site, deps.Catalog, cfg)
	auth := handlers.NewAuthHandler(deps.Views, deps.Store, deps.Site, deps.Store.Sessions(), cfg, deps.Catalog)
	buckets := handlers.NewBucketsHandler(consoleDeps)
	objects := handlers.NewObjectsHandler(consoleDeps)
	policies := handlers.NewPoliciesHandler(consoleDeps)
	analytics := handlers.NewAnalyticsHandler(consoleDeps)
	audit := handlers.NewAuditHandler(consoleDeps)
	settings := handlers.NewSettingsHandler(consoleDeps)

	loginLimit := middleware.NewRateLimiter(10, cfg)
	resetLimit := middleware.NewRateLimiter(10, cfg)

	r.Get("/", home.ServeHTTP)
	r.Get("/login", auth.Login)
	r.Post("/login", loginLimit.Middleware(http.HandlerFunc(auth.LoginPost)).ServeHTTP)
	r.Get("/signup", auth.SignUp)
	r.Post("/signup", loginLimit.Middleware(http.HandlerFunc(auth.SignUpPost)).ServeHTTP)
	r.Get("/forgot-password", auth.ForgotPassword)
	r.Post("/forgot-password", resetLimit.Middleware(http.HandlerFunc(auth.ForgotPasswordPost)).ServeHTTP)
	r.Get("/reset-password", auth.ResetPassword)
	r.Post("/reset-password", resetLimit.Middleware(http.HandlerFunc(auth.ResetPasswordPost)).ServeHTTP)
	r.Post("/logout", auth.LogoutPost)
	r.Post("/locale", handlers.PostLocale(cfg))

	authN := func(next http.HandlerFunc) http.HandlerFunc {
		return middleware.RequireAuthFunc("/login", next)
	}

	r.Get("/buckets", authN(buckets.List))
	r.Get("/buckets.csv", authN(buckets.ExportCSV))
	r.Post("/buckets", authN(buckets.Create))
	r.Post("/buckets/refresh", authN(buckets.Refresh))
	r.Post("/buckets/{name}/delete", authN(buckets.Delete))

	r.Get("/buckets/{name}/objects", authN(objects.List))
	r.Post("/buckets/{name}/objects", authN(objects.Upload))
	r.Post("/buckets/{name}/objects/delete", authN(objects.DeleteSelected))
	r.Post("/buckets/{name}/objects/folder", authN(objects.CreateFolder))

	r.Get("/buckets/{name}/settings", authN(policies.Get))
	r.Post("/buckets/{name}/settings/lifecycle", authN(policies.SaveLifecycle))
	r.Post("/buckets/{name}/settings/cors", authN(policies.SaveCORS))

	r.Get("/analytics", authN(analytics.ServeHTTP))
	r.Get("/audit", authN(audit.List))
	r.Get("/settings", authN(settings.Get))
	r.Post("/settings/accounts", authN(settings.AddAccount))
	r.Post("/settings/accounts/{id}/activate", authN(settings.Activate))
	r.Post("/settings/accounts/{id}/delete", authN(settings.Delete))
}

func registerLiveViews(hub *live.Hub, deps Deps) {
	_ = hub
	_ = deps
	// cais:live-views
}
