// Package reviewjson maps store.Review records to the public JSON shape
// shared by the HTTP API (serve) and the static export.
package reviewjson

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

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
}

// Answer is the public JSON representation of a marketplace answer.
type Answer struct {
	Text  string `json:"text"`
	State string `json:"state"`
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
	if review.MPAnswerText != nil || review.MPAnswerState != nil {
		answer = &Answer{Text: stringValue(review.MPAnswerText), State: stringValue(review.MPAnswerState)}
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
	normalized := NormalizeSellerArticle(article)
	if normalized == article {
		return "", false
	}
	u, ok := m.ProductLinks[normalized]
	return u, ok
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
