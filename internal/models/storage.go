package models

import "time"

// StorageAccount is one provider credential row. B2 application keys are the
// only provider today; the shape stays generic so more can land later.
type StorageAccount struct {
	ID        int64
	Label     string
	Provider  string
	KeyID     string
	Region    string
	Status    string
	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Bucket is a provider bucket enriched with the latest console scan.
type Bucket struct {
	ID      string
	Name    string
	Region  string
	Public  bool
	Class   string
	Created time.Time
	Objects int64
	Bytes   int64
	Scanned bool
}

// LifecycleRule mirrors the B2 lifecycle fields the console can edit.
type LifecycleRule struct {
	Prefix          string
	HideAfterDays   int
	DeleteAfterDays int
}

// CORSRule mirrors one B2 CORS rule. The JSON tags are the wire format of the
// console's CORS editor (B2 operation names, not HTTP verbs).
type CORSRule struct {
	Name          string   `json:"corsRuleName"`
	Origins       []string `json:"allowedOrigins"`
	Operations    []string `json:"allowedOperations"`
	Headers       []string `json:"allowedHeaders"`
	ExposeHeaders []string `json:"exposeHeaders"`
	MaxAge        int      `json:"maxAgeSeconds"`
}

// BucketSettings is the editable governance surface of a bucket.
type BucketSettings struct {
	Name              string
	Public            bool
	Lifecycle         []LifecycleRule
	CORS              []CORSRule
	DefaultEncryption string
	FileLockEnabled   bool
	RetentionDays     int
	VersionCount      int64
}

// BucketSettingsUpdate carries the fields a console save can change.
type BucketSettingsUpdate struct {
	Lifecycle []LifecycleRule
	CORS      []CORSRule
}

// ObjectInfo describes one stored object (file version).
type ObjectInfo struct {
	Key          string
	FileID       string
	Size         int64
	ContentType  string
	UploadedAt   time.Time
	ETag         string
	Class        string
	CacheControl string
	Metadata     map[string]string
}

// ObjectPage is one page of a prefix listing: synthesized folders plus objects.
type ObjectPage struct {
	Prefix     string
	Folders    []string
	Objects    []ObjectInfo
	NextCursor string
	HasMore    bool
}

// FileVersion is one version of a stored file (b2_list_file_versions row),
// including hide markers and unfinished large files.
type FileVersion struct {
	Key         string
	FileID      string
	Action      string // "upload" | "hide" | "start"
	Size        int64
	ContentType string
	UploadedAt  time.Time
	SHA1        string
	MD5         string
	Retention   string // "governance" | "compliance" | ""
	LegalHold   bool
	Encryption  string // "SSE-B2" | "SSE-C" | ""
}

// Hidden reports whether the version is a hide marker instead of content.
func (v FileVersion) Hidden() bool { return v.Action == "hide" }

// FileVersionPage is one page of a version listing.
type FileVersionPage struct {
	Versions []FileVersion
	NextName string
	NextID   string
	HasMore  bool
}

// UsagePoint is one daily usage sample used by the analytics charts.
type UsagePoint struct {
	Day     time.Time
	Bytes   int64
	Objects int64
}

// AuditEvent records one console operation for the audit trail.
type AuditEvent struct {
	ID     int64
	At     time.Time
	Actor  string
	Action string
	Target string
	Detail string
	Status int
}
