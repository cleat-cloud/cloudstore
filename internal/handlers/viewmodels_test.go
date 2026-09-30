package handlers

import (
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/puppe1990/cloudstore/internal/models"
)

func fakerBuckets() []models.Bucket {
	faker := gofakeit.New(21)
	regions := []string{"us-west-004", "eu-central-003"}
	classes := []string{"Standard", "Nearline", "Archive"}
	buckets := make([]models.Bucket, 0, 12)
	for i := 0; i < 12; i++ {
		buckets = append(buckets, models.Bucket{
			Name:    faker.Word() + "-" + faker.Word(),
			Region:  regions[faker.Number(0, len(regions)-1)],
			Class:   classes[faker.Number(0, len(classes)-1)],
			Public:  faker.Bool(),
			Objects: int64(faker.Number(1, 1000)),
			Bytes:   int64(faker.Number(1, 1_000_000)),
		})
	}
	return buckets
}

func TestFilterBuckets(t *testing.T) {
	buckets := []models.Bucket{
		{Name: "prod-assets-media-cdn", Region: "us-west-004", Class: "Standard", Public: false},
		{Name: "public-documentation-static", Region: "us-west-002", Class: "Nearline", Public: true},
		{Name: "customer-backups-glacier", Region: "eu-central-003", Class: "Archive", Public: false},
	}

	cases := []struct {
		name   string
		filter bucketFilters
		want   int
	}{
		{"empty", bucketFilters{}, 3},
		{"query case-insensitive", bucketFilters{Query: "ASSETS"}, 1},
		{"region", bucketFilters{Region: "us-west-004"}, 1},
		{"class", bucketFilters{Class: "Archive"}, 1},
		{"access public", bucketFilters{Access: "public"}, 1},
		{"access private", bucketFilters{Access: "private"}, 2},
		{"all sentinel ignored", bucketFilters{Region: "all", Class: "all", Access: "all"}, 3},
		{"combined", bucketFilters{Query: "backups", Class: "Archive"}, 1},
		{"no match", bucketFilters{Query: "nope"}, 0},
	}
	for _, tc := range cases {
		if got := len(filterBuckets(buckets, tc.filter)); got != tc.want {
			t.Errorf("%s: got %d buckets, want %d", tc.name, got, tc.want)
		}
	}
}

func TestPaginate_clampsAndKeepsQuery(t *testing.T) {
	items := make([]int, 23)
	page, pagination := paginate(items, 2, "q=media&class=Standard")
	if len(page) != pageSize {
		t.Fatalf("page size = %d, want %d", len(page), pageSize)
	}
	if pagination.Pages != 3 || pagination.From != 11 || pagination.To != 20 {
		t.Fatalf("pagination = %+v", pagination)
	}
	if got := pagination.Links[1].Href; got != "?q=media&class=Standard&page=2" {
		t.Errorf("link href = %q", got)
	}
	if !pagination.Links[1].Active || pagination.Links[0].Active {
		t.Error("active link wrong")
	}

	_, last := paginate(items, 99, "")
	if last.Page != 3 || last.To != 23 || last.HasNext {
		t.Fatalf("clamped last page = %+v", last)
	}

	_, empty := paginate([]int{}, 1, "")
	if empty.Pages != 1 || empty.From != 0 || empty.Total != 0 {
		t.Fatalf("empty pagination = %+v", empty)
	}
}

func TestSplitByClassAndPercent(t *testing.T) {
	split := splitByClass([]models.Bucket{
		{Class: "Standard", Bytes: 100},
		{Class: "Nearline", Bytes: 50},
		{Class: "Archive", Bytes: 50},
	})
	if split.Standard != 100 || split.Nearline != 50 || split.Archive != 50 || split.Total != 200 {
		t.Fatalf("split = %+v", split)
	}
	if got := percentOf(split.Standard, split.Total); got != 50 {
		t.Errorf("percent = %d, want 50", got)
	}
	if got := percentOf(1, 0); got != 0 {
		t.Errorf("percent of empty = %d, want 0", got)
	}
}

func TestRegionBreakdown_ordersByCount(t *testing.T) {
	labels := regionBreakdown(fakerBuckets(), 3)
	if len(labels) == 0 || len(labels) > 3 {
		t.Fatalf("labels = %+v", labels)
	}
	for i := 1; i < len(labels); i++ {
		if labels[i-1].Count < labels[i].Count {
			t.Fatalf("not sorted: %+v", labels)
		}
	}
}

func TestLineAndAreaPaths(t *testing.T) {
	if got := linePath(nil, 100, 20); got != "" {
		t.Errorf("empty linePath = %q", got)
	}
	line := linePath([]int64{0, 5, 10}, 100, 20)
	if !strings.HasPrefix(line, "0.00,20.00") || !strings.HasSuffix(line, "100.00,0.00") {
		t.Errorf("linePath = %q", line)
	}
	area := areaPath([]int64{0, 10}, 100, 20)
	if !strings.HasSuffix(area, "L 100.00,20.00 L 0,20.00 Z") {
		t.Errorf("areaPath = %q", area)
	}
	if got := linePath([]int64{7}, 100, 20); got != "0.00,0.00" {
		t.Errorf("single value = %q", got)
	}
}

func TestObjectIcon(t *testing.T) {
	cases := []struct{ contentType, key, want string }{
		{"video/mp4", "hero.mp4", "movie"},
		{"image/webp", "banner.webp", "image"},
		{"text/vtt", "subs.vtt", "subtitles"},
		{"application/json", "manifest.json", "data_object"},
		{"text/plain", "notes.txt", "description"},
		{"application/octet-stream", "blob.bin", "draft"},
	}
	for _, tc := range cases {
		if got := objectIcon(tc.contentType, tc.key); got != tc.want {
			t.Errorf("objectIcon(%q) = %q, want %q", tc.contentType, got, tc.want)
		}
	}
}

func TestStatusKind(t *testing.T) {
	if statusKind(200) != "ok" || statusKind(304) != "muted" || statusKind(403) != "warning" || statusKind(500) != "error" {
		t.Error("statusKind mapping wrong")
	}
}
