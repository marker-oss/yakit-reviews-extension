package reviewjson

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"reviews/internal/store"
)

func ptrInt(v int) *int       { return &v }
func ptrStr(v string) *string { return &v }

func TestToReview_WBMapping(t *testing.T) {
	mapper := Mapper{
		ProductURLTemplate: "https://example-shop.test/search?query={seller_article_url}",
		ProductLinks:       map[string]string{"1523": "https://example-shop.test/products/p-1523"},
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
	if len(out.Media) != 1 || out.Media[0].Kind != "photo" {
		t.Fatalf("media = %+v", out.Media)
	}
}

func TestToReview_EmbedPassthrough(t *testing.T) {
	r := store.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "wb-e",
		ExternalProductID: "1",
		CreatedAtMP:       time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Media: []store.ReviewMedia{
			{Kind: "video", URL: "https://vk.com/video-1_2", EmbedProvider: "vk", EmbedID: "-1_2", Position: 0},
			{Kind: "photo", URL: "https://cdn/p1.jpg", Position: 1},
		},
	}

	out := Mapper{}.ToReview(r)
	if len(out.Media) != 2 {
		t.Fatalf("media len = %d", len(out.Media))
	}
	if out.Media[0].EmbedProvider != "vk" || out.Media[0].EmbedID != "-1_2" {
		t.Fatalf("embed fields = %q/%q", out.Media[0].EmbedProvider, out.Media[0].EmbedID)
	}
	if out.Media[1].EmbedProvider != "" || out.Media[1].EmbedID != "" {
		t.Fatalf("plain media embed fields = %q/%q", out.Media[1].EmbedProvider, out.Media[1].EmbedID)
	}
}

func TestToReview_EmbedJSONFields(t *testing.T) {
	r := store.Review{Marketplace: "wb", ExternalReviewID: "wb-e", ExternalProductID: "1",
		CreatedAtMP: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Media: []store.ReviewMedia{
			{Kind: "video", URL: "https://vk.com/video-1_2", EmbedProvider: "vk", EmbedID: "-1_2"},
			{Kind: "photo", URL: "https://cdn/p1.jpg"},
		},
	}

	var payload struct {
		Media []struct {
			EmbedProvider string `json:"embedProvider"`
			EmbedID       string `json:"embedId"`
		} `json:"media"`
	}
	b, err := json.Marshal(Mapper{}.ToReview(r))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Media[0].EmbedProvider != "vk" || payload.Media[0].EmbedID != "-1_2" {
		t.Fatalf("embed media JSON = %+v", payload.Media[0])
	}
	if payload.Media[1].EmbedProvider != "" || payload.Media[1].EmbedID != "" {
		t.Fatalf("plain media JSON = %+v", payload.Media[1])
	}
}

func TestToReview_AppliesMarketplacePolicy(t *testing.T) {
	showLinks := false
	mapper := Mapper{
		MarketplacePolicy: MarketplacePolicies{
			"wb": {Label: "Маркетплейс", ShowSourceLinks: &showLinks},
		},
	}
	r := store.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "wb-1001",
		ExternalProductID: "70476012",
		SellerArticle:     "1523",
		CreatedAtMP:       time.Date(2026, 5, 28, 12, 20, 0, 0, time.UTC),
	}

	out := mapper.ToReview(r)

	if out.MarketplaceLabel != "Маркетплейс" {
		t.Fatalf("marketplaceLabel = %q", out.MarketplaceLabel)
	}
	if out.MarketplaceReviewURL != "" || out.MarketplaceProductURL != "" {
		t.Fatalf("source links leaked: review=%q product=%q", out.MarketplaceReviewURL, out.MarketplaceProductURL)
	}
}

func TestExcludedMarketplaces(t *testing.T) {
	mapper := Mapper{MarketplacePolicy: MarketplacePolicies{
		"wb": {Hidden: true},
		"ym": {},
	}}
	got := mapper.ExcludedMarketplaces()
	if len(got) != 1 || got[0] != "wb" {
		t.Fatalf("excluded = %+v", got)
	}
}

func TestToReview_TemplateFallbackWhenArticleNotInLinks(t *testing.T) {
	mapper := Mapper{
		ProductURLTemplate: "https://example-shop.test/search?query={seller_article_url}",
		ProductLinks:       map[string]string{"9999": "https://example-shop.test/products/p-9999"},
	}
	r := store.Review{
		Marketplace:       "wb",
		ExternalReviewID:  "wb-1",
		ExternalProductID: "70476012",
		SellerArticle:     "1523",
		CreatedAtMP:       time.Unix(1, 0),
	}
	out := mapper.ToReview(r)
	// 1523 is not in ProductLinks, so the template is used with the escaped article.
	if out.SellerProductURL != "https://example-shop.test/search?query=1523" {
		t.Fatalf("sellerProductUrl = %q", out.SellerProductURL)
	}
}

func TestToReview_ProductLinkNormalizationFallback(t *testing.T) {
	mapper := Mapper{
		ProductURLTemplate: "https://example-shop.test/search?query={seller_article_url}",
		// Only the normalized base article is registered; the review has a variant.
		ProductLinks: map[string]string{"3467": "https://example-shop.test/products/p-3467"},
	}
	r := store.Review{
		Marketplace:      "wb",
		ExternalReviewID: "wb-2",
		SellerArticle:    "3467/Белый",
		CreatedAtMP:      time.Unix(1, 0),
	}
	out := mapper.ToReview(r)
	// Exact "3467/Белый" misses; normalized "3467" hits the product link.
	if out.SellerProductURL != "https://example-shop.test/products/p-3467" {
		t.Fatalf("sellerProductUrl = %q", out.SellerProductURL)
	}
}

func TestNormalizeSellerArticleWithProductLinks(t *testing.T) {
	mapper := Mapper{
		ProductLinks: map[string]string{
			"6202":   "https://example-shop.test/products/p-6202",
			"3508":   "https://example-shop.test/products/p-3508",
			"3508-1": "https://example-shop.test/products/p-3508-1",
			"1715-3": "https://example-shop.test/products/p-1715-3",
			"105":    "https://example-shop.test/products/p-105",
		},
	}

	if got := mapper.NormalizeSellerArticle("6202бежевый"); got != "6202" {
		t.Fatalf("normalize 6202 color = %q", got)
	}
	if got := mapper.NormalizeSellerArticle("3508-1/Бордовый"); got != "3508-1" {
		t.Fatalf("normalize 3508-1 variant = %q", got)
	}
	if got := mapper.NormalizeSellerArticle("1715-3-52_Зеленый"); got != "1715-3" {
		t.Fatalf("normalize size/color suffix = %q", got)
	}
	if got := mapper.NormalizeSellerArticle("1102"); got != "1102" {
		t.Fatalf("normalize numeric suffix = %q", got)
	}
	if got := mapper.NormalizeSellerArticle("10552"); got != "10552" {
		t.Fatalf("normalize product number containing another article = %q", got)
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

func TestAdminReplyOverridesMarketplaceAnswer(t *testing.T) {
	mpText := "marketplace answer"
	mpState := "published"
	adminText := "seller answer"
	now := time.Now().UTC()
	review := store.Review{
		Marketplace:      "wb",
		ExternalReviewID: "r1",
		MPAnswerText:     &mpText,
		MPAnswerState:    &mpState,
		AdminReplyText:   &adminText,
		AdminReplyAt:     &now,
		CreatedAtMP:      now,
	}

	mapped := Mapper{}.ToReview(review)
	if mapped.Answer == nil {
		t.Fatal("expected answer")
	}
	if mapped.Answer.Text != adminText || mapped.Answer.State != "published" {
		t.Fatalf("answer = %+v", mapped.Answer)
	}
}

func TestAnswerKind(t *testing.T) {
	admin := "seller reply"
	sellerReview := store.Review{AdminReplyText: &admin}
	if a := (Mapper{}).ToReview(sellerReview).Answer; a == nil || a.Kind != "seller" {
		t.Fatalf("seller answer kind = %+v", a)
	}
	mpText := "mp reply"
	mpState := "published"
	mpReview := store.Review{MPAnswerText: &mpText, MPAnswerState: &mpState}
	if a := (Mapper{}).ToReview(mpReview).Answer; a == nil || a.Kind != "marketplace" {
		t.Fatalf("marketplace answer kind = %+v", a)
	}
}

func TestMarketplaceProductURL_YM(t *testing.T) {
	r := store.Review{Marketplace: "ym", ExternalProductID: "12345"}
	got := (Mapper{}).ToReview(r).MarketplaceProductURL
	if got != "https://market.yandex.ru/product/12345" {
		t.Errorf("got %q", got)
	}
	empty := store.Review{Marketplace: "ym", ExternalProductID: ""}
	if got := (Mapper{}).ToReview(empty).MarketplaceProductURL; got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestToReview_CustomDataMapping(t *testing.T) {
	r := store.Review{
		Marketplace:       store.MarketplaceSite,
		ExternalReviewID:  "site-1",
		ExternalProductID: "1523",
		CreatedAtMP:       time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		CustomData:        `{"height":"160-170","fit":"в размер"}`,
	}
	out := (Mapper{}).ToReview(r)
	if out.Custom["height"] != "160-170" || out.Custom["fit"] != "в размер" {
		t.Fatalf("custom = %+v", out.Custom)
	}

	// Invalid or empty payloads stay absent from the JSON.
	if out := (Mapper{}).ToReview(store.Review{CustomData: "not-json"}); out.Custom != nil {
		t.Fatalf("invalid payload must map to nil, got %+v", out.Custom)
	}
	if out := (Mapper{}).ToReview(store.Review{CustomData: "{}"}); out.Custom != nil {
		t.Fatalf("empty payload must map to nil, got %+v", out.Custom)
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"custom":`) {
		t.Fatalf("custom missing from public JSON: %s", encoded)
	}
}
