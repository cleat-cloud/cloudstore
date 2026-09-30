// Package console orchestrates the pieces the CloudStore UI needs: the active
// storage provider (B2 or demo), the SQLite store and the audit trail.
package console

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/cleat-cloud/cloudstore/internal/crypto"
	"github.com/cleat-cloud/cloudstore/internal/format"
	"github.com/cleat-cloud/cloudstore/internal/models"
	"github.com/cleat-cloud/cloudstore/internal/storage"
	"github.com/cleat-cloud/cloudstore/internal/store"
)

const (
	// DefaultQuotaBytes is the sidebar quota when none is configured (100 TB,
	// matching the design's storage gauge).
	DefaultQuotaBytes = 100 * 1000 * 1000 * 1000 * 1000
	// b2MonthlyCentsPerTB is B2's $6/TB/month storage price.
	b2MonthlyCentsPerTB = 600
	// scanObjectCap bounds one bucket scan so the console stays responsive on
	// accounts with millions of objects; the UI labels capped scans.
	scanObjectCap = 20000
)

type Options struct {
	QuotaBytes  int64
	Demo        storage.Provider
	NewProvider func(account models.StorageAccount, secret string) storage.Provider
}

type Service struct {
	store store.Store
	key   []byte
	opts  Options

	mu        sync.Mutex
	providers map[int64]storage.Provider
}

func New(st store.Store, encryptionKey []byte, opts Options) *Service {
	return &Service{store: st, key: encryptionKey, opts: opts, providers: map[int64]storage.Provider{}}
}

// Active is the storage context of the current console session.
type Active struct {
	Provider storage.Provider
	Account  models.StorageAccount
	Live     bool
}

// Active resolves the account in use. Without one the console runs against the
// demo provider so every screen works before credentials are configured.
func (s *Service) Active(ctx context.Context) (Active, error) {
	account, ok, err := s.store.ActiveStorageAccount()
	if err != nil {
		return Active{}, err
	}
	if !ok {
		if s.opts.Demo == nil {
			return Active{}, fmt.Errorf("no storage account configured")
		}
		return Active{Provider: s.opts.Demo, Live: false}, nil
	}
	provider, err := s.providerFor(account)
	if err != nil {
		return Active{}, err
	}
	return Active{Provider: provider, Account: account, Live: true}, nil
}

func (s *Service) providerFor(account models.StorageAccount) (storage.Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if provider, ok := s.providers[account.ID]; ok {
		return provider, nil
	}
	sealed, err := s.store.StorageAccountSecret(account.ID)
	if err != nil {
		return nil, err
	}
	secret, err := crypto.Decrypt(s.key, sealed)
	if err != nil {
		return nil, fmt.Errorf("decrypt account %d: %w", account.ID, err)
	}
	provider := s.opts.NewProvider(account, secret)
	s.providers[account.ID] = provider
	return provider, nil
}

func (s *Service) Accounts(ctx context.Context) ([]models.StorageAccount, error) {
	return s.store.ListStorageAccounts()
}

// AddAccount stores a provider key encrypted and returns the persisted row.
func (s *Service) AddAccount(ctx context.Context, label, keyID, appKey, region string) (models.StorageAccount, error) {
	sealed, err := crypto.Encrypt(s.key, appKey)
	if err != nil {
		return models.StorageAccount{}, err
	}
	id, err := s.store.CreateStorageAccount(models.StorageAccount{
		Label: label, Provider: "b2", KeyID: keyID, Region: region,
	}, sealed)
	if err != nil {
		return models.StorageAccount{}, err
	}
	return s.store.FindStorageAccountByID(id)
}

func (s *Service) ActivateAccount(ctx context.Context, id int64) error {
	return s.store.SetActiveStorageAccount(id)
}

func (s *Service) DeleteAccount(ctx context.Context, id int64) error {
	s.mu.Lock()
	delete(s.providers, id)
	s.mu.Unlock()
	return s.store.DeleteStorageAccount(id)
}

// Buckets merges the provider's live buckets with the latest stored scans.
func (s *Service) Buckets(ctx context.Context) ([]models.Bucket, error) {
	active, err := s.Active(ctx)
	if err != nil {
		return nil, err
	}
	buckets, err := active.Provider.ListBuckets(ctx)
	if err != nil {
		return nil, err
	}
	scans, err := s.store.BucketScans()
	if err != nil {
		return nil, err
	}
	for i := range buckets {
		usage, ok := scans[buckets[i].Name]
		if !ok {
			continue
		}
		buckets[i].Objects = usage.Objects
		buckets[i].Bytes = usage.Bytes
		buckets[i].Scanned = true
		if usage.Class != "" {
			buckets[i].Class = usage.Class
		}
	}
	return buckets, nil
}

// ScanSummary reports what one Refresh pass covered.
type ScanSummary struct {
	Buckets int
	Objects int64
	Bytes   int64
	Capped  bool
}

// Refresh re-lists every bucket (capped per bucket), stores per-bucket usage
// and folds the totals into today's analytics sample.
func (s *Service) Refresh(ctx context.Context) (ScanSummary, error) {
	active, err := s.Active(ctx)
	if err != nil {
		return ScanSummary{}, err
	}
	buckets, err := active.Provider.ListBuckets(ctx)
	if err != nil {
		return ScanSummary{}, err
	}

	var summary ScanSummary
	for _, bucket := range buckets {
		objects, bytes, capped, err := scanBucket(ctx, active.Provider, bucket.Name)
		if err != nil {
			if errors.Is(err, storage.ErrBucketNotFound) {
				continue
			}
			return summary, fmt.Errorf("scan %s: %w", bucket.Name, err)
		}
		class := bucket.Class
		if settings, err := active.Provider.BucketSettings(ctx, bucket.Name); err == nil {
			class = storage.ClassFromLifecycle(settings.Lifecycle)
		}
		if err := s.store.UpsertBucketScan(bucket.Name, objects, bytes, class); err != nil {
			return summary, err
		}
		summary.Buckets++
		summary.Objects += objects
		summary.Bytes += bytes
		summary.Capped = summary.Capped || capped
	}

	if !active.Live {
		s.seedDemoHistory()
	}
	return summary, nil
}

// scanBucket walks the bucket tree (folders become new prefixes) so objects
// nested under prefixes are counted without listing them name-by-name.
func scanBucket(ctx context.Context, provider storage.Provider, bucket string) (int64, int64, bool, error) {
	var objects, bytes int64
	queue := []string{""}
	for len(queue) > 0 {
		prefix := queue[0]
		queue = queue[1:]
		cursor := ""
		for {
			page, err := provider.ListObjects(ctx, bucket, prefix, cursor, 1000)
			if err != nil {
				return 0, 0, false, err
			}
			for _, object := range page.Objects {
				objects++
				bytes += object.Size
			}
			queue = append(queue, page.Folders...)
			if objects >= scanObjectCap {
				return objects, bytes, true, nil
			}
			if !page.HasMore {
				break
			}
			cursor = page.NextCursor
		}
	}
	return objects, bytes, false, nil
}

// seedDemoHistory fabricates a 30-day growth curve once so the analytics chart
// has a story in demo mode; real accounts accumulate samples through Refresh.
// Today's point always keeps the real scanned totals.
func (s *Service) seedDemoHistory() {
	// Refresh already upserted today's real total, so "seeded" means the curve
	// exists (>3 points), not just a single row.
	existing, err := s.store.UsageHistory(2)
	if err != nil || len(existing) > 3 {
		return
	}
	scans, err := s.store.BucketScans()
	if err != nil || len(scans) == 0 {
		return
	}
	var used, objects int64
	for _, usage := range scans {
		used += usage.Bytes
		objects += usage.Objects
	}
	if used == 0 {
		return
	}

	faker := gofakeit.New(42)
	now := time.Now().UTC()
	for daysAgo := 29; daysAgo >= 0; daysAgo-- {
		day := now.AddDate(0, 0, -daysAgo)
		if daysAgo == 0 {
			_ = s.store.UpsertUsagePoint(day, used, objects)
			continue
		}
		progress := float64(30-daysAgo) / 30
		jitter := 0.97 + faker.Float64()*0.06
		bytes := int64(math.Min(float64(used), float64(used)*(0.62+0.38*progress)*jitter))
		items := int64(math.Min(float64(objects), float64(objects)*(0.62+0.38*progress)*jitter))
		_ = s.store.UpsertUsagePoint(day, bytes, items)
	}
}

// Quota reports sidebar usage against the configured quota.
func (s *Service) Quota() (used int64, quota int64, err error) {
	scans, err := s.store.BucketScans()
	if err != nil {
		return 0, 0, err
	}
	for _, usage := range scans {
		used += usage.Bytes
	}
	quota = s.opts.QuotaBytes
	if quota <= 0 {
		quota = DefaultQuotaBytes
	}
	return used, quota, nil
}

func (s *Service) UsageHistory(days int) ([]models.UsagePoint, error) {
	return s.store.UsageHistory(days)
}

// Scans exposes the latest per-bucket usage for KPI and table rendering.
func (s *Service) Scans() (map[string]store.BucketUsage, error) {
	return s.store.BucketScans()
}

// SeedDemo prepares demo mode before the first page view: one scan pass over
// the generated buckets, the 30-day history and the matching audit entry. It
// is a no-op for live accounts and when usage already exists, so the console
// never opens with empty KPIs, charts and audit trail.
func (s *Service) SeedDemo(ctx context.Context) error {
	active, err := s.Active(ctx)
	if err != nil || active.Live {
		return err
	}
	scans, err := s.store.BucketScans()
	if err != nil || len(scans) > 0 {
		return err
	}
	summary, err := s.Refresh(ctx)
	if err != nil {
		return fmt.Errorf("seed demo scan: %w", err)
	}
	return s.Audit(models.AuditEvent{
		At: time.Now().UTC(), Actor: "system", Action: "ScanBuckets", Target: "b2://*",
		Detail: fmt.Sprintf("%d buckets · %s", summary.Buckets, format.Bytes(summary.Bytes)), Status: 200,
	})
}

// Audit records one console operation for the audit trail.
func (s *Service) Audit(event models.AuditEvent) error {
	_, err := s.store.InsertAuditEvent(event)
	return err
}

func (s *Service) AuditEvents(filter store.AuditFilter) ([]models.AuditEvent, error) {
	return s.store.ListAuditEvents(filter)
}

func (s *Service) AuditCount(filter store.AuditFilter) (int64, error) {
	return s.store.CountAuditEvents(filter)
}

func (s *Service) AuditDailyCounts(since time.Time) ([]store.AuditDayCount, error) {
	return s.store.AuditDailyCounts(since)
}

func (s *Service) AuditStatusCounts(since time.Time) ([]store.AuditStatusCount, error) {
	return s.store.AuditStatusCounts(since)
}

// EstimateMonthlyCostCents estimates B2 storage cost from scanned bytes
// ($6/TB/month — egress and transactions are not included).
func EstimateMonthlyCostCents(bytes int64) int64 {
	if bytes <= 0 {
		return 0
	}
	return bytes * b2MonthlyCentsPerTB / (1000 * 1000 * 1000 * 1000)
}
