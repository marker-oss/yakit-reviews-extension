package reviewjson

import (
	"testing"
	"time"

	"reviews/internal/store"
)

func ptrInt(v int) *int       { return &v }
func ptrStr(v string) *string { return &v }

func TestToReview_WBMapping(t *testing.T) {
	mapper := Mapper{
		ProductURLTemplate: "https://shegida.ru/search?query={seller_article_url}",
		ProductLinks:       map[string]string{"1523": "https://shegida.ru/products/p-1523"},
	}
	r := store.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "wb-1001",
		ExternalProductID: "70476012",
		SellerArticle:     "1523",
		Rating:            ptrInt(5),
		AuthorName:        "Мария",
		Text:              "Отличная ткань",
		CreatedAtMP:       time.Date(2026, 5, 28, 12, 20, 0, 0, time.UTC),
		MPAnswerText:      ptrStr("Спасибо"),
		MPAnswerState:     ptrStr("published"),
		Media: []store.ReviewMedia{
			{Kind: "photo", URL: "https://cdn/p1.jpg", Position: 0},
		},
	}

	out := mapper.ToReview(r)

	if out.SellerArticle != "1523" {
		t.Fatalf("sellerArticle = %q", out.SellerArticle)
	}
	if out.MarketplaceProductURL != "https://www.wildberries.ru/catalog/70476012/detail.aspx" {
		t.Fatalf("marketplaceProductUrl = %q", out.MarketplaceProductURL)
	}
	if out.MarketplaceReviewURL != "https://www.wildberries.ru/catalog/70476012/detail.aspx#comments" {
		t.Fatalf("marketplaceReviewUrl = %q", out.MarketplaceReviewURL)
	}
	if out.SellerProductURL != "https://shegida.ru/products/p-1523" {
		t.Fatalf("sellerProductUrl = %q", out.SellerProductURL)
	}
	if out.Answer == nil || out.Answer.Text != "Спасибо" {
		t.Fatalf("answer = %+v", out.Answer)
	}
	if len(out.Media) != 1 || out.Media[0].Kind != "photo" {
		t.Fatalf("media = %+v", out.Media)
	}
}

func TestNormalizeSellerArticle(t *testing.T) {
	if got := NormalizeSellerArticle("3467/Белый"); got != "3467" {
		t.Fatalf("normalize = %q", got)
	}
	if got := NormalizeSellerArticle("  107 "); got != "107" {
		t.Fatalf("normalize trim = %q", got)
	}
}
