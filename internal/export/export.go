// Package export builds static per-article review JSON artifacts for the
// embeddable site widget.
package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"reviews/internal/reviewjson"
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
// aggregates. Reviews without a seller article are skipped.
func BuildBundles(reviews []store.Review, mapper reviewjson.Mapper) map[string]*Bundle {
	bundles := make(map[string]*Bundle)
	for _, review := range reviews {
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
		sort.SliceStable(bundle.Reviews, func(i, j int) bool {
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
