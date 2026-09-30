package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/puppe1990/cloudstore/internal/models"
)

const sqliteTimeLayout = "2006-01-02 15:04:05"

func (s *SQLiteStore) FindUserByID(id int64) (models.User, error) {
	var u models.User
	err := s.db.QueryRow(
		"SELECT id, email, password_hash, created_at FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return models.User{}, fmt.Errorf("find user %d: %w", id, err)
	}
	return u, nil
}

// CreateStorageAccount stores the provider key; the secret arrives encrypted
// (the store never sees the plaintext application key). The first account
// becomes active so a fresh install has a working context.
func (s *SQLiteStore) CreateStorageAccount(account models.StorageAccount, secret string) (int64, error) {
	tx, err := s.db.Raw().Begin()
	if err != nil {
		return 0, fmt.Errorf("begin create account: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM storage_accounts").Scan(&count); err != nil {
		return 0, fmt.Errorf("count accounts: %w", err)
	}
	active := 0
	if count == 0 {
		active = 1
	}
	if account.Provider == "" {
		account.Provider = "b2"
	}
	if account.Status == "" {
		account.Status = "connected"
	}
	result, err := tx.Exec(
		`INSERT INTO storage_accounts (label, provider, key_id, secret_encrypted, region, status, active)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		account.Label, account.Provider, account.KeyID, secret, account.Region, account.Status, active,
	)
	if err != nil {
		return 0, fmt.Errorf("insert account: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit account: %w", err)
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) ListStorageAccounts() ([]models.StorageAccount, error) {
	rows, err := s.db.Query(
		`SELECT id, label, provider, key_id, region, status, active, created_at, updated_at
		 FROM storage_accounts ORDER BY created_at, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var accounts []models.StorageAccount
	for rows.Next() {
		var a models.StorageAccount
		var active int
		if err := rows.Scan(&a.ID, &a.Label, &a.Provider, &a.KeyID, &a.Region, &a.Status, &active, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		a.Active = active == 1
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *SQLiteStore) FindStorageAccountByID(id int64) (models.StorageAccount, error) {
	var a models.StorageAccount
	var active int
	err := s.db.QueryRow(
		`SELECT id, label, provider, key_id, region, status, active, created_at, updated_at
		 FROM storage_accounts WHERE id = ?`,
		id,
	).Scan(&a.ID, &a.Label, &a.Provider, &a.KeyID, &a.Region, &a.Status, &active, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return models.StorageAccount{}, fmt.Errorf("find account %d: %w", id, err)
	}
	a.Active = active == 1
	return a, nil
}

func (s *SQLiteStore) ActiveStorageAccount() (models.StorageAccount, bool, error) {
	var a models.StorageAccount
	var active int
	err := s.db.QueryRow(
		`SELECT id, label, provider, key_id, region, status, active, created_at, updated_at
		 FROM storage_accounts WHERE active = 1 ORDER BY id LIMIT 1`,
	).Scan(&a.ID, &a.Label, &a.Provider, &a.KeyID, &a.Region, &a.Status, &active, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return models.StorageAccount{}, false, nil
	}
	if err != nil {
		return models.StorageAccount{}, false, fmt.Errorf("active account: %w", err)
	}
	a.Active = active == 1
	return a, true, nil
}

func (s *SQLiteStore) StorageAccountSecret(id int64) (string, error) {
	var secret string
	err := s.db.QueryRow("SELECT secret_encrypted FROM storage_accounts WHERE id = ?", id).Scan(&secret)
	if err != nil {
		return "", fmt.Errorf("account secret %d: %w", id, err)
	}
	return secret, nil
}

func (s *SQLiteStore) SetActiveStorageAccount(id int64) error {
	tx, err := s.db.Raw().Begin()
	if err != nil {
		return fmt.Errorf("begin activate account: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("UPDATE storage_accounts SET active = 0"); err != nil {
		return fmt.Errorf("clear active accounts: %w", err)
	}
	if _, err := tx.Exec("UPDATE storage_accounts SET active = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?", id); err != nil {
		return fmt.Errorf("activate account: %w", err)
	}
	return tx.Commit()
}

// DeleteStorageAccount removes the row. If it was active the console falls
// back to "no account" (demo mode) instead of silently switching providers.
func (s *SQLiteStore) DeleteStorageAccount(id int64) error {
	if _, err := s.db.Exec("DELETE FROM storage_accounts WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	return nil
}

func (s *SQLiteStore) InsertAuditEvent(event models.AuditEvent) (int64, error) {
	at := event.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	result, err := s.db.Exec(
		`INSERT INTO audit_events (at, actor, action, target, detail, status) VALUES (?, ?, ?, ?, ?, ?)`,
		at.UTC().Format(sqliteTimeLayout), event.Actor, event.Action, event.Target, event.Detail, event.Status,
	)
	if err != nil {
		return 0, fmt.Errorf("insert audit event: %w", err)
	}
	return result.LastInsertId()
}

func auditWhere(filter AuditFilter) (string, []any) {
	var clauses []string
	var args []any
	if filter.Action != "" {
		clauses = append(clauses, "action = ?")
		args = append(args, filter.Action)
	}
	if filter.Status != 0 {
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.Query != "" {
		clauses = append(clauses, "(target LIKE ? OR detail LIKE ? OR actor LIKE ?)")
		like := "%" + filter.Query + "%"
		args = append(args, like, like, like)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (s *SQLiteStore) ListAuditEvents(filter AuditFilter) ([]models.AuditEvent, error) {
	where, args := auditWhere(filter)
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit, filter.Offset)
	rows, err := s.db.Query(
		"SELECT id, at, actor, action, target, detail, status FROM audit_events"+where+
			" ORDER BY at DESC, id DESC LIMIT ? OFFSET ?",
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []models.AuditEvent
	for rows.Next() {
		var e models.AuditEvent
		var at string
		if err := rows.Scan(&e.ID, &at, &e.Actor, &e.Action, &e.Target, &e.Detail, &e.Status); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		e.At = parseSQLiteTime(at)
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *SQLiteStore) CountAuditEvents(filter AuditFilter) (int64, error) {
	where, args := auditWhere(filter)
	var count int64
	if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_events"+where, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count audit events: %w", err)
	}
	return count, nil
}

func (s *SQLiteStore) AuditDailyCounts(since time.Time) ([]AuditDayCount, error) {
	rows, err := s.db.Query(
		`SELECT date(at) AS day, COUNT(*) FROM audit_events WHERE at >= ? GROUP BY day ORDER BY day`,
		since.UTC().Format(sqliteTimeLayout),
	)
	if err != nil {
		return nil, fmt.Errorf("audit daily counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var counts []AuditDayCount
	for rows.Next() {
		var day string
		var count int64
		if err := rows.Scan(&day, &count); err != nil {
			return nil, fmt.Errorf("scan daily count: %w", err)
		}
		parsed, err := time.Parse("2006-01-02", day)
		if err != nil {
			return nil, fmt.Errorf("parse day %q: %w", day, err)
		}
		counts = append(counts, AuditDayCount{Day: parsed, Count: count})
	}
	return counts, rows.Err()
}

func (s *SQLiteStore) AuditStatusCounts(since time.Time) ([]AuditStatusCount, error) {
	rows, err := s.db.Query(
		`SELECT status, COUNT(*) FROM audit_events WHERE at >= ? GROUP BY status ORDER BY COUNT(*) DESC`,
		since.UTC().Format(sqliteTimeLayout),
	)
	if err != nil {
		return nil, fmt.Errorf("audit status counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var counts []AuditStatusCount
	for rows.Next() {
		var sc AuditStatusCount
		if err := rows.Scan(&sc.Status, &sc.Count); err != nil {
			return nil, fmt.Errorf("scan status count: %w", err)
		}
		counts = append(counts, sc)
	}
	return counts, rows.Err()
}

// UpsertBucketScan stores the latest scan and folds it into today's usage
// sample so the analytics chart has a real daily series.
func (s *SQLiteStore) UpsertBucketScan(bucket string, objects, bytes int64) error {
	tx, err := s.db.Raw().Begin()
	if err != nil {
		return fmt.Errorf("begin scan upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		`INSERT INTO bucket_scans (bucket_name, objects, bytes, scanned_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(bucket_name) DO UPDATE SET objects = excluded.objects, bytes = excluded.bytes, scanned_at = CURRENT_TIMESTAMP`,
		bucket, objects, bytes,
	); err != nil {
		return fmt.Errorf("upsert bucket scan: %w", err)
	}
	// SQLite parses `ON` as a join unless the INSERT..SELECT has a WHERE (#upsert).
	if _, err := tx.Exec(
		`INSERT INTO usage_daily (day, bytes, objects)
		 SELECT date('now'), COALESCE(SUM(bytes), 0), COALESCE(SUM(objects), 0) FROM bucket_scans WHERE true
		 ON CONFLICT(day) DO UPDATE SET bytes = excluded.bytes, objects = excluded.objects`,
	); err != nil {
		return fmt.Errorf("upsert usage daily: %w", err)
	}
	return tx.Commit()
}

func (s *SQLiteStore) BucketScans() (map[string]BucketUsage, error) {
	rows, err := s.db.Query("SELECT bucket_name, objects, bytes, scanned_at FROM bucket_scans")
	if err != nil {
		return nil, fmt.Errorf("list bucket scans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	scans := map[string]BucketUsage{}
	for rows.Next() {
		var name, scannedAt string
		var usage BucketUsage
		if err := rows.Scan(&name, &usage.Objects, &usage.Bytes, &scannedAt); err != nil {
			return nil, fmt.Errorf("scan bucket scan: %w", err)
		}
		usage.ScannedAt = parseSQLiteTime(scannedAt)
		scans[name] = usage
	}
	return scans, rows.Err()
}

func (s *SQLiteStore) UsageHistory(days int) ([]models.UsagePoint, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := s.db.Query(
		`SELECT day, bytes, objects FROM usage_daily WHERE day >= date('now', ?) ORDER BY day`,
		fmt.Sprintf("-%d days", days),
	)
	if err != nil {
		return nil, fmt.Errorf("usage history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var points []models.UsagePoint
	for rows.Next() {
		var day string
		var point models.UsagePoint
		if err := rows.Scan(&day, &point.Bytes, &point.Objects); err != nil {
			return nil, fmt.Errorf("scan usage point: %w", err)
		}
		parsed, err := time.Parse("2006-01-02", day)
		if err != nil {
			return nil, fmt.Errorf("parse usage day %q: %w", day, err)
		}
		point.Day = parsed
		points = append(points, point)
	}
	return points, rows.Err()
}

// parseSQLiteTime accepts the driver's DATETIME text formats.
func parseSQLiteTime(value string) time.Time {
	for _, layout := range []string{sqliteTimeLayout, "2006-01-02T15:04:05Z", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
