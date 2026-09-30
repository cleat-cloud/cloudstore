package storage

import (
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/fakedata"

	"github.com/puppe1990/cloudstore/internal/models"
)

func TestValidateBucketName_valid(t *testing.T) {
	valid := []string{"prod-assets-media-cdn", "analytics-lakehouse-raw", "abc123", "my-bucket-01"}
	for _, name := range valid {
		if err := ValidateBucketName(name); err != nil {
			t.Errorf("ValidateBucketName(%q) = %v, want nil", name, err)
		}
	}
}

func TestValidateBucketName_invalid(t *testing.T) {
	invalid := []string{"", "abc", "UPPER", "with space", "-leading", "trailing-", "under_score", "dot.name", "slash/name", strings.Repeat("a", 51)}
	for _, name := range invalid {
		if err := ValidateBucketName(name); err == nil {
			t.Errorf("ValidateBucketName(%q) = nil, want error", name)
		}
	}
}

func TestValidateBucketName_rejectsFakerNoise(t *testing.T) {
	fakedata.Seed(7)
	for i := 0; i < 50; i++ {
		name := fakedata.Sentence() // words, spaces, punctuation — never a bucket name
		if err := ValidateBucketName(name); err == nil {
			t.Fatalf("ValidateBucketName(%q) = nil, want error", name)
		}
	}
}

func TestClassFromLifecycle(t *testing.T) {
	cases := []struct {
		name  string
		rules []models.LifecycleRule
		want  string
	}{
		{"no rules", nil, "Standard"},
		{"hide only", []models.LifecycleRule{{Prefix: "", HideAfterDays: 30}}, "Nearline"},
		{"hide and delete", []models.LifecycleRule{{HideAfterDays: 30, DeleteAfterDays: 180}}, "Archive"},
		{"delete only", []models.LifecycleRule{{DeleteAfterDays: 60}}, "Archive"},
		{"zero days ignored", []models.LifecycleRule{{Prefix: "tmp/"}}, "Standard"},
	}
	for _, tc := range cases {
		if got := ClassFromLifecycle(tc.rules); got != tc.want {
			t.Errorf("%s: ClassFromLifecycle = %q, want %q", tc.name, got, tc.want)
		}
	}
}
