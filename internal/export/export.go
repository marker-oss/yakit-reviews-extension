// Package export builds static per-article review JSON artifacts for the
// embeddable site widget.
package export

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"reviews/internal/reviewjson"
	"reviews/internal/site"
	"reviews/internal/store"
)

type Aggregate struct {
	Count       int     `json:"count"`
	RatingCount int     `json:"ratingCount"`
	RatingAvg   float64 `json:"ratingAvg"`
}

type Bundle struct {
	Article   string              `json:"article"`
	Aggregate Aggregate           `json:"aggregate"`
	Reviews   []reviewjson.Review `json:"reviews"`
}

type IndexEntry struct {
	Count     int     `json:"count"`
	RatingAvg float64 `json:"ratingAvg"`
}

type Index struct {
	GeneratedAt time.Time             `json:"generatedAt"`
	Articles    map[string]IndexEntry `json:"articles"`
}

// BuildBundles groups reviews by normalized seller article and computes
// aggregates. Reviews without a seller article are skipped. Optional pin maps
// place matching review IDs first within each article bundle.
func BuildBundles(reviews []store.Review, mapper reviewjson.Mapper, pinsByArticle ...map[string][]uint) map[string]*Bundle {
	pins := map[string][]uint{}
	if len(pinsByArticle) > 0 && pinsByArticle[0] != nil {
		pins = pinsByArticle[0]
	}
	bundles := make(map[string]*Bundle)
	for _, review := range reviews {
		if mapper.ReviewHidden(review) {
			continue
		}
		article := mapper.NormalizeSellerArticle(reviewjson.SellerArticleForReview(review))
		if article == "" {
			continue
		}

		bundle := bundles[article]
		if bundle == nil {
			bundle = &Bundle{Article: article}
			bundles[article] = bundle
		}
		bundle.Reviews = append(bundle.Reviews, mapper.ToReview(review))
	}

	for _, bundle := range bundles {
		pinOrder := reviewPinOrder(pins[bundle.Article])
		sort.SliceStable(bundle.Reviews, func(i, j int) bool {
			left, leftPinned := pinOrder[bundle.Reviews[i].ID]
			right, rightPinned := pinOrder[bundle.Reviews[j].ID]
			if leftPinned && rightPinned {
				return left < right
			}
			if leftPinned != rightPinned {
				return leftPinned
			}
			return bundle.Reviews[i].CreatedAt.After(bundle.Reviews[j].CreatedAt)
		})

		var sum, ratingCount int
		for _, review := range bundle.Reviews {
			if review.Rating == nil {
				continue
			}
			sum += *review.Rating
			ratingCount++
		}

		bundle.Aggregate = Aggregate{
			Count:       len(bundle.Reviews),
			RatingCount: ratingCount,
		}
		if ratingCount > 0 {
			bundle.Aggregate.RatingAvg = roundTenth(float64(sum) / float64(ratingCount))
		}
	}

	return bundles
}

func reviewPinOrder(ids []uint) map[uint]int {
	order := make(map[uint]int, len(ids))
	for i, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := order[id]; !exists {
			order[id] = i
		}
	}
	return order
}

// Write emits by-article/<article>.json files and index.json into dir.
func Write(dir string, bundles map[string]*Bundle, generatedAt time.Time) error {
	byArticleDir := filepath.Join(dir, "by-article")
	if err := os.RemoveAll(byArticleDir); err != nil {
		return err
	}
	if err := os.MkdirAll(byArticleDir, 0o755); err != nil {
		return err
	}

	index := Index{
		GeneratedAt: generatedAt,
		Articles:    make(map[string]IndexEntry, len(bundles)),
	}
	for article, bundle := range bundles {
		if err := writeJSONFile(filepath.Join(byArticleDir, articleFileName(article)), bundle); err != nil {
			return err
		}
		index.Articles[article] = IndexEntry{
			Count:     bundle.Aggregate.Count,
			RatingAvg: bundle.Aggregate.RatingAvg,
		}
	}

	return writeJSONFile(filepath.Join(dir, "index.json"), index)
}

// LinkIndex maps shop product page locations to their seller article. The
// embed loader fetches this to resolve the current product on SPA navigation,
// where the page's own #requestContext no longer reflects the visited product.
type LinkIndex struct {
	GeneratedAt time.Time         `json:"generatedAt"`
	ByPath      map[string]string `json:"byPath"`
	ByID        map[string]string `json:"byID"`
}

// BuildLinkIndex derives path- and id-keyed article lookups from the full
// crawled product-link list. Many product paths may share one article.
func BuildLinkIndex(links []site.ProductLink, generatedAt time.Time) LinkIndex {
	idx := LinkIndex{
		GeneratedAt: generatedAt,
		ByPath:      make(map[string]string),
		ByID:        make(map[string]string),
	}
	for _, link := range links {
		article := strings.TrimSpace(link.SellerArticle)
		if article == "" {
			continue
		}
		path := linkPath(link.URL)
		if path == "" {
			continue
		}
		idx.ByPath[path] = article
		if id := productID(path); id != "" {
			idx.ByID[id] = article
		}
	}
	return idx
}

// WriteLinks writes the link index as links.json into dir.
func WriteLinks(dir string, idx LinkIndex) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeJSONFile(filepath.Join(dir, "links.json"), idx)
}

// linkPath returns the normalized URL path (no trailing slash) of a product URL.
func linkPath(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.TrimRight(parsed.Path, "/")
}

// productID extracts the trailing numeric Kit product id from a product path
// (e.g. /products/plate-...-102291 → "102291"). Returns "" when the tail after
// the last hyphen is not purely numeric.
func productID(path string) string {
	i := strings.LastIndex(path, "-")
	if i < 0 || i+1 >= len(path) {
		return ""
	}
	tail := path[i+1:]
	for _, r := range tail {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return tail
}

func articleFileName(article string) string {
	safe := make([]rune, 0, len(article))
	for _, r := range article {
		switch r {
		case '/', '\\', ' ':
			safe = append(safe, '_')
		default:
			safe = append(safe, r)
		}
	}
	return string(safe) + ".json"
}

func roundTenth(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

func writeJSONFile(path string, value any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
