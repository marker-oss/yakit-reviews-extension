// Package reviewjson maps store.Review records to the public JSON shape
// shared by the HTTP API (serve) and the static export.
package reviewjson

import (
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"reviews/internal/store"
)

const marketplaceWB = "wb"

// Mapper holds the per-deployment configuration needed to compute outbound
// links. Zero value is usable (no product links, empty template).
type Mapper struct {
	ProductURLTemplate string
	ProductLinks       map[string]string
}

// Review is the public JSON representation of a stored review.
type Review struct {
	ID                    uint       `json:"id"`
	Marketplace           string     `json:"marketplace"`
	ExternalReviewID      string     `json:"externalReviewId"`
	ExternalProductID     string     `json:"externalProductId"`
	SellerArticle         string     `json:"sellerArticle,omitempty"`
	Rating                *int       `json:"rating"`
	AuthorName            string     `json:"authorName"`
	Text                  string     `json:"text"`
	Pros                  string     `json:"pros"`
	Cons                  string     `json:"cons"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             *time.Time `json:"updatedAt,omitempty"`
	Answer                *Answer    `json:"answer,omitempty"`
	Media                 []Media    `json:"media"`
	MarketplaceReviewURL  string     `json:"marketplaceReviewUrl,omitempty"`
	MarketplaceProductURL string     `json:"marketplaceProductUrl,omitempty"`
	SellerProductURL      string     `json:"sellerProductUrl,omitempty"`
	Pinned                bool       `json:"pinned,omitempty"`
}

// Answer is the public JSON representation of a marketplace answer.
type Answer struct {
	Text  string `json:"text"`
	State string `json:"state"`
	Kind  string `json:"kind"` // "seller" | "marketplace"
}

// Media is the public JSON representation of review media.
type Media struct {
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	PreviewURL string `json:"previewUrl,omitempty"`
	Position   int    `json:"position"`
}

func (m Mapper) ToReview(review store.Review) Review {
	sellerArticle := SellerArticleForReview(review)

	var answer *Answer
	if review.AdminReplyText != nil && strings.TrimSpace(*review.AdminReplyText) != "" {
		answer = &Answer{Text: *review.AdminReplyText, State: "published", Kind: "seller"}
	} else if review.MPAnswerText != nil || review.MPAnswerState != nil {
		answer = &Answer{Text: stringValue(review.MPAnswerText), State: stringValue(review.MPAnswerState), Kind: "marketplace"}
	}

	media := make([]Media, 0, len(review.Media))
	for _, item := range review.Media {
		media = append(media, Media{
			Kind:       item.Kind,
			URL:        item.URL,
			PreviewURL: stringValue(item.PreviewURL),
			Position:   item.Position,
		})
	}

	return Review{
		ID:                    review.ID,
		Marketplace:           review.Marketplace,
		ExternalReviewID:      review.ExternalReviewID,
		ExternalProductID:     review.ExternalProductID,
		SellerArticle:         sellerArticle,
		Rating:                review.Rating,
		AuthorName:            review.AuthorName,
		Text:                  review.Text,
		Pros:                  review.Pros,
		Cons:                  review.Cons,
		CreatedAt:             review.CreatedAtMP,
		UpdatedAt:             review.UpdatedAtMP,
		Answer:                answer,
		Media:                 media,
		MarketplaceReviewURL:  marketplaceReviewURL(review),
		MarketplaceProductURL: marketplaceProductURL(review),
		SellerProductURL:      m.sellerProductURL(review, sellerArticle),
		Pinned:                review.Pinned,
	}
}

func marketplaceReviewURL(review store.Review) string {
	productURL := marketplaceProductURL(review)
	if productURL == "" {
		return ""
	}
	if review.Marketplace == marketplaceWB {
		return productURL + "#comments"
	}
	return productURL
}

func marketplaceProductURL(review store.Review) string {
	switch review.Marketplace {
	case marketplaceWB:
		if review.ExternalProductID == "" {
			return ""
		}
		return "https://www.wildberries.ru/catalog/" + urlPathEscape(review.ExternalProductID) + "/detail.aspx"
	default:
		return ""
	}
}

// When the seller article is empty, {article} uses the external product id while {seller_article} expands to empty.
func (m Mapper) sellerProductURL(review store.Review, sellerArticle string) string {
	article := sellerArticle
	if article == "" {
		article = review.ExternalProductID
	}
	if article == "" {
		return ""
	}
	if sellerArticle != "" {
		if u, ok := m.productLinkForSellerArticle(sellerArticle); ok {
			return u
		}
	}
	if m.ProductURLTemplate == "" {
		return ""
	}
	return strings.NewReplacer(
		"{article}", article,
		"{seller_article}", sellerArticle,
		"{seller_article_url}", url.QueryEscape(sellerArticle),
		"{external_product_id}", review.ExternalProductID,
		"{external_product_id_url}", url.QueryEscape(review.ExternalProductID),
		"{marketplace}", review.Marketplace,
	).Replace(m.ProductURLTemplate)
}

func (m Mapper) productLinkForSellerArticle(article string) (string, bool) {
	if len(m.ProductLinks) == 0 {
		return "", false
	}
	if u, ok := m.ProductLinks[article]; ok {
		return u, true
	}
	normalized := m.NormalizeSellerArticle(article)
	if normalized == article {
		return "", false
	}
	u, ok := m.ProductLinks[normalized]
	return u, ok
}

// NormalizeSellerArticle returns the export/grouping article for this
// deployment. It first applies generic marketplace normalization, then uses
// known site product articles to collapse variant suffixes like
// "6202бежевый" or "1715-3-52_Зеленый" to the matching site article.
func (m Mapper) NormalizeSellerArticle(article string) string {
	normalized := NormalizeSellerArticle(article)
	if normalized == "" || len(m.ProductLinks) == 0 {
		return normalized
	}
	if _, ok := m.ProductLinks[normalized]; ok {
		return normalized
	}

	keys := make([]string, 0, len(m.ProductLinks))
	for key := range m.ProductLinks {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})

	for _, key := range keys {
		if key == "" || len(key) >= len(normalized) || !strings.HasPrefix(normalized, key) {
			continue
		}
		suffix := strings.TrimSpace(strings.TrimPrefix(normalized, key))
		if startsWithCyrillicLetter(suffix) || startsWithSizeColorSuffix(suffix) {
			return key
		}
	}
	return normalized
}

// NormalizeSellerArticle collapses marketplace article variants (e.g.
// "3467/Белый") to their base ("3467") and trims whitespace.
func NormalizeSellerArticle(article string) string {
	article = strings.TrimSpace(article)
	if before, _, ok := strings.Cut(article, "/"); ok {
		return strings.TrimSpace(before)
	}
	return article
}

func startsWithCyrillicLetter(value string) bool {
	for _, r := range value {
		return unicode.Is(unicode.Cyrillic, r) && unicode.IsLetter(r)
	}
	return false
}

func startsWithSizeColorSuffix(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || !strings.ContainsRune("-_ ", []rune(value)[0]) {
		return false
	}
	value = strings.TrimLeft(value, "-_ ")
	if value == "" {
		return false
	}
	for _, r := range value {
		return unicode.IsDigit(r)
	}
	return false
}

// SellerArticleForReview returns the explicit seller article, falling back to
// parsing it out of the raw WB payload.
func SellerArticleForReview(review store.Review) string {
	if review.SellerArticle != "" {
		return review.SellerArticle
	}
	if review.Raw == "" || review.Marketplace != marketplaceWB {
		return ""
	}
	var raw struct {
		ProductDetails struct {
			SupplierArticle string `json:"supplierArticle"`
		} `json:"productDetails"`
	}
	if err := json.Unmarshal([]byte(review.Raw), &raw); err != nil {
		return ""
	}
	return raw.ProductDetails.SupplierArticle
}

// urlPathEscape hand-escapes only / ? # & so readable characters (e.g. Cyrillic) stay untouched in WB catalog paths.
func urlPathEscape(value string) string {
	return strings.NewReplacer("/", "%2F", "?", "%3F", "#", "%23", "&", "%26").Replace(value)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
