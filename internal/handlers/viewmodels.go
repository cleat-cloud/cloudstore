package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/cleat-cloud/cloudstore/internal/models"
)

// pageSize is the console's dense table page size.
const pageSize = 10

// bucketFilters holds the overview query params.
type bucketFilters struct {
	Query  string
	Region string
	Class  string
	Access string
}

func bucketFiltersFrom(r *http.Request) bucketFilters {
	query := r.URL.Query()
	return bucketFilters{
		Query:  strings.TrimSpace(query.Get("q")),
		Region: query.Get("region"),
		Class:  query.Get("class"),
		Access: query.Get("access"),
	}
}

func filterBuckets(buckets []models.Bucket, f bucketFilters) []models.Bucket {
	name := strings.ToLower(f.Query)
	out := make([]models.Bucket, 0, len(buckets))
	for _, bucket := range buckets {
		if name != "" && !strings.Contains(strings.ToLower(bucket.Name), name) {
			continue
		}
		if f.Region != "" && f.Region != "all" && bucket.Region != f.Region {
			continue
		}
		if f.Class != "" && f.Class != "all" && bucket.Class != f.Class {
			continue
		}
		switch f.Access {
		case "public":
			if !bucket.Public {
				continue
			}
		case "private":
			if bucket.Public {
				continue
			}
		}
		out = append(out, bucket)
	}
	return out
}

// pagination is everything the footer needs to render page links.
type pagination struct {
	Page    int
	Pages   int
	Total   int
	From    int
	To      int
	HasPrev bool
	HasNext bool
	Prev    pageLink
	Next    pageLink
	Links   []pageLink
}

type pageLink struct {
	Number int
	Label  string
	Href   string
	Active bool
}

// paginate slices one page and precomputes the footer links preserving the
// current query string.
func paginate[T any](items []T, page int, query string) ([]T, pagination) {
	total := len(items)
	pages := (total + pageSize - 1) / pageSize
	if pages == 0 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	href := func(n int) string {
		separator := "?"
		if query != "" {
			separator = "&"
		}
		return "?" + query + separator + "page=" + strconv.Itoa(n)
	}

	result := pagination{
		Page: page, Pages: pages, Total: total, From: start + 1, To: end,
	}
	if total == 0 {
		result.From, result.To = 0, 0
	}
	result.HasPrev = page > 1
	result.HasNext = page < pages
	result.Prev = pageLink{Number: page - 1, Label: "←", Href: href(page - 1)}
	result.Next = pageLink{Number: page + 1, Label: "→", Href: href(page + 1)}
	for n := 1; n <= pages; n++ {
		result.Links = append(result.Links, pageLink{
			Number: n, Label: strconv.Itoa(n), Href: href(n), Active: n == page,
		})
	}
	return items[start:end], result
}

// classSplit splits scanned bytes by the lifecycle-derived class.
type classSplit struct {
	Standard int64
	Nearline int64
	Archive  int64
	Total    int64
}

func splitByClass(buckets []models.Bucket) classSplit {
	var split classSplit
	for _, bucket := range buckets {
		split.Total += bucket.Bytes
		switch bucket.Class {
		case "Nearline":
			split.Nearline += bucket.Bytes
		case "Archive":
			split.Archive += bucket.Bytes
		default:
			split.Standard += bucket.Bytes
		}
	}
	return split
}

func percentOf(part, total int64) int {
	if total <= 0 {
		return 0
	}
	return int(part * 100 / total)
}

type countLabel struct {
	Label string
	Count int
}

// regionBreakdown groups buckets by region, largest groups first.
func regionBreakdown(buckets []models.Bucket, top int) []countLabel {
	counts := map[string]int{}
	for _, bucket := range buckets {
		region := bucket.Region
		if region == "" {
			region = "—"
		}
		counts[region]++
	}
	labels := make([]countLabel, 0, len(counts))
	for label, count := range counts {
		labels = append(labels, countLabel{Label: label, Count: count})
	}
	sort.Slice(labels, func(i, j int) bool {
		if labels[i].Count != labels[j].Count {
			return labels[i].Count > labels[j].Count
		}
		return labels[i].Label < labels[j].Label
	})
	if top > 0 && len(labels) > top {
		labels = labels[:top]
	}
	return labels
}

// totalObjects sums scanned object counts.
func totalObjects(buckets []models.Bucket) int64 {
	var total int64
	for _, bucket := range buckets {
		total += bucket.Objects
	}
	return total
}

// regionOptions lists the regions present, for the filter select.
func regionOptions(buckets []models.Bucket) []string {
	seen := map[string]bool{}
	var regions []string
	for _, bucket := range buckets {
		if bucket.Region == "" || seen[bucket.Region] {
			continue
		}
		seen[bucket.Region] = true
		regions = append(regions, bucket.Region)
	}
	sort.Strings(regions)
	return regions
}

// linePath scales values into an SVG polyline ("x,y x,y …"), y inverted.
func linePath(values []int64, width, height float64) string {
	if len(values) == 0 {
		return ""
	}
	max := maxValue(values)
	step := 0.0
	if len(values) > 1 {
		step = width / float64(len(values)-1)
	}
	var b strings.Builder
	for i, value := range values {
		x := float64(i) * step
		y := height
		if max > 0 {
			y = height - (float64(value)/float64(max))*height
		}
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(strconv.FormatFloat(x, 'f', 2, 64))
		b.WriteString(",")
		b.WriteString(strconv.FormatFloat(y, 'f', 2, 64))
	}
	return b.String()
}

// areaPath closes the polyline to the baseline for a filled chart.
func areaPath(values []int64, width, height float64) string {
	line := linePath(values, width, height)
	if line == "" {
		return ""
	}
	return "M " + strings.ReplaceAll(strings.ReplaceAll(line, " ", " L "), ",", " ") +
		" L " + strconv.FormatFloat(width, 'f', 2, 64) + "," + strconv.FormatFloat(height, 'f', 2, 64) +
		" L 0," + strconv.FormatFloat(height, 'f', 2, 64) + " Z"
}

func maxValue(values []int64) int64 {
	var max int64
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}

// usageValues extracts the byte series for charts.
func usageValues(points []models.UsagePoint) []int64 {
	values := make([]int64, 0, len(points))
	for _, point := range points {
		values = append(values, point.Bytes)
	}
	return values
}

// objectIcon maps an object to the Material Symbols glyph used in listings.
func objectIcon(contentType, key string) string {
	switch {
	case strings.HasPrefix(contentType, "video/"):
		return "movie"
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.HasPrefix(contentType, "text/vtt") || strings.HasSuffix(key, ".vtt"):
		return "subtitles"
	case strings.Contains(contentType, "json"):
		return "data_object"
	case strings.HasPrefix(contentType, "text/"):
		return "description"
	default:
		return "draft"
	}
}

// statusKind maps an audit status to the badge palette used in tables.
func statusKind(status int) string {
	switch {
	case status >= 500:
		return "error"
	case status >= 400:
		return "warning"
	case status >= 300:
		return "muted"
	default:
		return "ok"
	}
}
