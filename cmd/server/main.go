package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/boot"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"

	"github.com/cleat-cloud/cloudstore/internal/app"
	"github.com/cleat-cloud/cloudstore/internal/console"
	"github.com/cleat-cloud/cloudstore/internal/crypto"
	appi18n "github.com/cleat-cloud/cloudstore/internal/i18n"
	"github.com/cleat-cloud/cloudstore/internal/models"
	"github.com/cleat-cloud/cloudstore/internal/storage"
	"github.com/cleat-cloud/cloudstore/internal/storage/b2"
	"github.com/cleat-cloud/cloudstore/internal/storage/fakestore"
	"github.com/cleat-cloud/cloudstore/internal/store"
	"github.com/cleat-cloud/cloudstore/web"
)

// devAppSecret keeps `amarra-cais dev` frictionless; production must set
// APP_SECRET so stored application keys are not sealed with a public value.
const devAppSecret = "cloudstore-dev-only-change-me"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := cais.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}
	preferredPort := cfg.Port
	port, shifted, err := cais.ResolvePort(cfg.Port, cfg.Env)
	if err != nil {
		return err
	}
	cfg.Port = port

	a, err := bootstrapWithConfig(cfg)
	if err != nil {
		return err
	}

	shiftedFrom := ""
	if shifted {
		shiftedFrom = preferredPort
	}
	boot.Print(os.Stdout, boot.Options{
		AppName:         "CloudStore",
		Config:          cfg,
		Version:         boot.CaisVersion(),
		PortShiftedFrom: shiftedFrom,
	})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return a.RunContext(ctx)
}

func bootstrapWithConfig(cfg cais.Config) (*app.App, error) {
	tmplFS, err := fs.Sub(web.Templates, "templates")
	if err != nil {
		return nil, fmt.Errorf("templates: %w", err)
	}

	catalog := appi18n.NewCatalog(cfg.Locale)
	catalogs := map[string]*i18n.Catalog{
		"en": appi18n.NewCatalog("en"),
		"pt": appi18n.NewCatalog("pt"),
	}
	views, err := view.Load(tmplFS, catalog, catalogs["en"], catalogs["pt"])
	if err != nil {
		return nil, fmt.Errorf("views: %w", err)
	}

	s, err := store.NewSQLiteStore(cfg.DBPath, cfg.Env)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}

	consoleSvc, err := newConsoleService(cfg, s)
	if err != nil {
		_ = s.Close()
		return nil, err
	}

	staticDir, err := cais.ResolveWebDir("static", cfg.StaticDir)
	if err != nil {
		_ = s.Close()
		return nil, err
	}

	return app.New(cfg, app.Deps{
		Views:     views,
		Store:     s,
		Console:   consoleSvc,
		StaticDir: staticDir,
		Site:      meta.SiteFrom("CloudStore", cfg.AppURL),
		Catalog:   catalog,
		Catalogs:  catalogs,
	})
}

// newConsoleService wires the storage providers: B2 for live accounts, the
// gofakeit demo store when no account is active yet.
func newConsoleService(cfg cais.Config, s *store.SQLiteStore) (*console.Service, error) {
	key, err := resolveAppKey(cfg.Env)
	if err != nil {
		return nil, err
	}

	// Demo data is opt-in (CLOUDSTORE_DEMO=1): a real deployment connects a B2
	// account instead of serving generated buckets.
	var demo storage.Provider
	if os.Getenv("CLOUDSTORE_DEMO") == "1" {
		demo = fakestore.New(1)
	}

	service := console.New(s, key, console.Options{
		QuotaBytes: quotaBytes(),
		Demo:       demo,
		NewProvider: func(account models.StorageAccount, secret string) storage.Provider {
			return b2.New(b2.Config{KeyID: account.KeyID, AppKey: secret, Region: account.Region})
		},
	})

	if err := seedAccountFromEnv(service); err != nil {
		log.Printf("cloudstore: seed account: %v", err)
	}
	// Demo mode should look alive on the first visit (KPIs, charts, audit).
	if demo != nil {
		if err := service.SeedDemo(context.Background()); err != nil {
			log.Printf("cloudstore: seed demo data: %v", err)
		}
	}
	return service, nil
}

func resolveAppKey(env string) ([]byte, error) {
	secret := os.Getenv("APP_SECRET")
	if secret == "" {
		if env == "production" {
			return nil, errors.New("APP_SECRET is required when ENV=production")
		}
		log.Println("warning: APP_SECRET is empty; using the insecure development default")
		secret = devAppSecret
	}
	return crypto.DeriveKey(secret), nil
}

func quotaBytes() int64 {
	value, err := strconv.ParseInt(os.Getenv("CLOUDSTORE_QUOTA_BYTES"), 10, 64)
	if err != nil || value <= 0 {
		return console.DefaultQuotaBytes
	}
	return value
}

// seedAccountFromEnv provisions a B2 account from the environment when none
// exists yet, so `amarra-cais dev` can talk to a real account without UI steps.
func seedAccountFromEnv(service *console.Service) error {
	keyID, appKey := os.Getenv("B2_KEY_ID"), os.Getenv("B2_APP_KEY")
	if keyID == "" || appKey == "" {
		return nil
	}
	ctx := context.Background()
	accounts, err := service.Accounts(ctx)
	if err != nil {
		return err
	}
	if len(accounts) > 0 {
		return nil
	}
	label := os.Getenv("B2_ACCOUNT_LABEL")
	if label == "" {
		label = "Backblaze B2"
	}
	account, err := service.AddAccount(ctx, label, keyID, appKey, os.Getenv("B2_REGION"))
	if err != nil {
		return err
	}
	log.Printf("cloudstore: seeded storage account %q (%s)", account.Label, account.KeyID)
	return nil
}
