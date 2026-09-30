package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/devlog"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"
	caissqlite "github.com/puppe1990/amarra-cais/pkg/cais/sqlite"
	"github.com/puppe1990/amarra-cais/pkg/cais/sqllog"
	_ "modernc.org/sqlite"

	"github.com/cleat-cloud/cloudstore/internal/models"
)

var ErrEmailTaken = errors.New("email already registered")

// AuditFilter narrows audit log listings.
type AuditFilter struct {
	Action string
	Status int
	Query  string
	Limit  int
	Offset int
}

// AuditDayCount is one day of console activity (analytics sparkline).
type AuditDayCount struct {
	Day   time.Time
	Count int64
}

// AuditStatusCount aggregates audit events by HTTP-style status.
type AuditStatusCount struct {
	Status int
	Count  int64
}

// BucketUsage is the latest scan of one bucket.
type BucketUsage struct {
	Objects   int64
	Bytes     int64
	Class     string
	ScannedAt time.Time
}

type Store interface {
	FindUserByEmail(email string) (models.User, error)
	FindUserByID(id int64) (models.User, error)
	CreateUser(email, passwordHash string) (int64, error)
	CreatePasswordResetToken(userID int64) (string, error)
	ClearPasswordResetTokens(userID int64) error
	FindPasswordResetUserID(token string) (int64, bool)
	ResetPasswordWithToken(token, passwordHash string) error
	CreateStorageAccount(account models.StorageAccount, secret string) (int64, error)
	ListStorageAccounts() ([]models.StorageAccount, error)
	FindStorageAccountByID(id int64) (models.StorageAccount, error)
	ActiveStorageAccount() (models.StorageAccount, bool, error)
	StorageAccountSecret(id int64) (string, error)
	SetActiveStorageAccount(id int64) error
	DeleteStorageAccount(id int64) error
	InsertAuditEvent(event models.AuditEvent) (int64, error)
	ListAuditEvents(filter AuditFilter) ([]models.AuditEvent, error)
	CountAuditEvents(filter AuditFilter) (int64, error)
	AuditDailyCounts(since time.Time) ([]AuditDayCount, error)
	AuditStatusCounts(since time.Time) ([]AuditStatusCount, error)
	UpsertBucketScan(bucket string, objects, bytes int64, class string) error
	UpsertUsagePoint(day time.Time, bytes, objects int64) error
	BucketScans() (map[string]BucketUsage, error)
	UsageHistory(days int) ([]models.UsagePoint, error)
	Sessions() session.Store
	Ping() error
	DB() *sql.DB
	Close() error
}

type SQLiteStore struct {
	db *sqllog.DB
}

func NewSQLiteStore(dsn string, env string) (*SQLiteStore, error) {
	if dsn != ":memory:" {
		dir := filepath.Dir(dsn)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", caissqlite.DSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := caissqlite.Configure(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}

	if err := applyMigrations(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	cfg := sqllog.ConfigForEnv(env)
	if cfg.Enabled {
		cfg.Writer = devlog.MirrorDefault(os.Stdout)
	}
	wrapped := sqllog.Wrap(db, cfg)
	if err := seedAuthData(wrapped.Raw(), env); err != nil {
		_ = wrapped.Close()
		return nil, err
	}
	return &SQLiteStore{db: wrapped}, nil
}

func seedAuthData(db *sql.DB, env string) error {
	if env != "development" {
		return nil
	}
	if err := session.EnsureSQLiteSchema(db); err != nil {
		return err
	}
	hash, err := session.HashPassword("password")
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT OR IGNORE INTO users (email, password_hash) VALUES (?, ?)", "demo@example.com", hash)
	return err
}

func (s *SQLiteStore) FindUserByEmail(email string) (models.User, error) {
	var u models.User
	err := s.db.QueryRow(
		"SELECT id, email, password_hash, created_at FROM users WHERE email = ?",
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return models.User{}, fmt.Errorf("find user: %w", err)
	}
	return u, nil
}

func (s *SQLiteStore) CreateUser(email, passwordHash string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO users (email, password_hash) VALUES (?, ?)",
		email, passwordHash,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, ErrEmailTaken
		}
		return 0, fmt.Errorf("create user: %w", err)
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) Sessions() session.Store {
	return session.NewSQLiteStore(s.db.Raw())
}

func (s *SQLiteStore) Ping() error {
	return s.db.Raw().Ping()
}

func (s *SQLiteStore) DB() *sql.DB {
	return s.db.Raw()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
