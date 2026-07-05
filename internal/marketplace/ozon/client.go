package ozon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"reviews/internal/config"
	"reviews/internal/marketplace"
)

const (
	marketplaceID   = config.MarketplaceOzon
	defaultBaseURL  = "https://api-seller.ozon.ru"
	defaultPageSize = 100
	productMapTTL   = 10 * time.Minute
)

type Client struct {
	baseURL    string
	clientID   string
	apiKey     string
	httpClient *http.Client
	pageSize   int

	// productArticles caches the sku→offer_id map between review pages; see
	// cachedProductArticles.
	productArticles   map[string]string
	productArticlesAt time.Time
}

func New(cfg config.OzonConfig) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		clientID:   cfg.ClientID,
		apiKey:     cfg.APIKey,
		httpClient: http.DefaultClient,
		pageSize:   defaultPageSize,
	}
}

func NewWithHTTPClient(cfg config.OzonConfig, baseURL string, httpClient *http.Client, pageSize int) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		clientID:   cfg.ClientID,
		apiKey:     cfg.APIKey,
		httpClient: httpClient,
		pageSize:   pageSize,
	}
}

func (c *Client) Marketplace() string {
	return marketplaceID
}

func (c *Client) FetchReviews(ctx context.Context, since time.Time, cursor string) ([]marketplace.Review, string, error) {
	payload, err := c.fetchPage(ctx, cursor)
	if err != nil {
		return nil, "", err
	}

	articles := c.cachedProductArticles(ctx)
	reviews := make([]marketplace.Review, 0, len(payload.Reviews))
	stopPaging := false
	for _, item := range payload.Reviews {
		review, err := item.toReview()
		if err != nil {
			return nil, "", err
		}
		if article, ok := articles[review.ExternalProductID]; ok && article != "" {
			review.SellerArticle = article
		}
		if !since.IsZero() && review.CreatedAtMP.Before(since) {
			stopPaging = true
			continue
		}
		reviews = append(reviews, review)
	}

	if stopPaging || !payload.HasNext || payload.LastID == "" {
		return reviews, "", nil
	}
	return reviews, payload.LastID, nil
}

func (c *Client) fetchPage(ctx context.Context, cursor string) (ozonReviewListResponse, error) {
	requestPayload := ozonReviewListRequest{
		Limit:   c.pageSize,
		SortDir: "DESC",
		LastID:  cursor,
	}
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return ozonReviewListResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/review/list", bytes.NewReader(body))
	if err != nil {
		return ozonReviewListResponse{}, err
	}
	req.Header.Set("Client-Id", c.clientID)
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ozonReviewListResponse{}, err
	}
	defer resp.Body.Close()

	var payload ozonReviewListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ozonReviewListResponse{}, fmt.Errorf("decode Ozon review list response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if payload.Message != "" {
			return ozonReviewListResponse{}, fmt.Errorf("Ozon review list: status %d: %s", resp.StatusCode, payload.Message)
		}
		if len(payload.Details) > 0 {
			return ozonReviewListResponse{}, fmt.Errorf("Ozon review list: status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload.Details[0])))
		}
		return ozonReviewListResponse{}, fmt.Errorf("Ozon review list: status %d", resp.StatusCode)
	}

	return payload, nil
}

type ozonReviewListRequest struct {
	Limit   int    `json:"limit"`
	SortDir string `json:"sort_dir"`
	LastID  string `json:"last_id,omitempty"`
}

type ozonReviewListResponse struct {
	Reviews []ozonReview      `json:"reviews"`
	HasNext bool              `json:"has_next"`
	LastID  string            `json:"last_id"`
	Message string            `json:"message"`
	Details []json.RawMessage `json:"details"`
}

type ozonReview struct {
	ID             string     `json:"id"`
	SKU            flexString `json:"sku"`
	Text           string     `json:"text"`
	Rating         int        `json:"rating"`
	Status         string     `json:"status"`
	PublishedAt    string     `json:"published_at"`
	OrderStatus    string     `json:"order_status"`
	CommentsAmount int        `json:"comments_amount"`
	PhotosAmount   int        `json:"photos_amount"`
	VideosAmount   int        `json:"videos_amount"`
}

func (r ozonReview) toReview() (marketplace.Review, error) {
	publishedAt, err := time.Parse(time.RFC3339Nano, r.PublishedAt)
	if err != nil {
		return marketplace.Review{}, fmt.Errorf("parse Ozon published_at for %s: %w", r.ID, err)
	}

	raw, err := json.Marshal(r)
	if err != nil {
		return marketplace.Review{}, err
	}

	var rating *int
	if r.Rating > 0 {
		value := r.Rating
		rating = &value
	}
	sku := r.SKU.String()

	return marketplace.Review{
		Marketplace:       marketplaceID,
		ExternalReviewID:  r.ID,
		ExternalProductID: sku,
		SellerArticle:     sku,
		Rating:            rating,
		Text:              r.Text,
		CreatedAtMP:       publishedAt,
		Raw:               raw,
	}, nil
}

// NOTE: Ozon question field names not yet verified against live docs — confirm before enabling Q&A MP-fetch in production.
type ozonQuestionListRequest struct {
	Filter ozonQuestionFilter `json:"filter"`
	Limit  int                `json:"limit"`
	LastID string             `json:"last_id,omitempty"`
}

type ozonQuestionFilter struct {
	Visibility string `json:"visibility"` // e.g. "ALL"
}

type ozonQuestionListResponse struct {
	Items   []ozonQuestion    `json:"items"`
	HasNext bool              `json:"has_next"`
	LastID  string            `json:"last_id"`
	Message string            `json:"message"`
	Details []json.RawMessage `json:"details"`
}

type ozonQuestion struct {
	QuestionID string     `json:"question_id"`
	SKU        flexString `json:"sku"`
	Text       string     `json:"text"`
	AuthorName string     `json:"author_name"`
	CreatedAt  string     `json:"created_at"`
}

func (c *Client) FetchQuestions(ctx context.Context, since time.Time, cursor string) ([]marketplace.Question, string, error) {
	requestPayload := ozonQuestionListRequest{
		Filter: ozonQuestionFilter{Visibility: "ALL"},
		Limit:  c.pageSize,
		LastID: cursor,
	}
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/question/list", bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Client-Id", c.clientID)
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	var payload ozonQuestionListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, "", fmt.Errorf("decode Ozon question list response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if payload.Message != "" {
			return nil, "", fmt.Errorf("Ozon question list: status %d: %s", resp.StatusCode, payload.Message)
		}
		if len(payload.Details) > 0 {
			return nil, "", fmt.Errorf("Ozon question list: status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload.Details[0])))
		}
		return nil, "", fmt.Errorf("Ozon question list: status %d", resp.StatusCode)
	}

	questions := make([]marketplace.Question, 0, len(payload.Items))
	for _, q := range payload.Items {
		createdAt, err := time.Parse(time.RFC3339Nano, q.CreatedAt)
		if err != nil {
			return nil, "", fmt.Errorf("parse Ozon question created_at for %s: %w", q.QuestionID, err)
		}
		if !since.IsZero() && createdAt.Before(since) {
			continue
		}
		sku := q.SKU.String()
		questions = append(questions, marketplace.Question{
			ExternalQuestionID: q.QuestionID,
			ExternalProductID:  sku,
			ExternalSKU:        sku,
			AuthorName:         q.AuthorName,
			Text:               q.Text,
			CreatedAtMP:        createdAt,
		})
	}

	nextCursor := ""
	if payload.HasNext && payload.LastID != "" {
		nextCursor = payload.LastID
	}
	return questions, nextCursor, nil
}

func (c *Client) PublishQuestionAnswer(ctx context.Context, externalQuestionID, sku, text string) error {
	payload := map[string]any{"question_id": externalQuestionID, "sku": sku, "text": text}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/question/answer/create", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", c.clientID)
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Ozon publish question answer: status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) PublishReply(ctx context.Context, externalReviewID, text string) error {
	reviewID, err := strconv.ParseInt(externalReviewID, 10, 64)
	if err != nil {
		return fmt.Errorf("Ozon publish reply: bad review id %q: %w", externalReviewID, err)
	}
	payload := map[string]any{
		"review_id":                reviewID,
		"text":                     text,
		"mark_review_as_processed": true,
		"parent_comment_id":        0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/review/comment/create", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", c.clientID)
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Ozon publish reply: status %d", resp.StatusCode)
	}
	return nil
}

type flexString string

func (s *flexString) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*s = ""
		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*s = flexString(text)
		return nil
	}

	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err == nil {
		*s = flexString(number.String())
		return nil
	}

	var value float64
	if err := json.Unmarshal(data, &value); err == nil {
		*s = flexString(strconv.FormatFloat(value, 'f', -1, 64))
		return nil
	}

	return fmt.Errorf("invalid flexible string value %s", string(data))
}

func (s flexString) String() string {
	return string(s)
}

// ProductArticles returns the seller's sku→offer_id map by paginating
// /v3/product/list. offer_id is the seller's own article, so it is the value
// that matches the shop site's articles; the review payload only carries the
// marketplace sku. Requires the Api-Key to have the products role.
func (c *Client) ProductArticles(ctx context.Context) (map[string]string, error) {
	articles := make(map[string]string)
	lastID := ""
	for {
		payload, err := c.fetchProductPage(ctx, lastID)
		if err != nil {
			return nil, err
		}
		for _, item := range payload.Result.Items {
			sku := item.SKU.String()
			if sku != "" && item.OfferID != "" {
				articles[sku] = item.OfferID
			}
		}
		if len(payload.Result.Items) < c.pageSize || payload.Result.LastID == "" {
			return articles, nil
		}
		lastID = payload.Result.LastID
	}
}

// CheckProductsAccess verifies the Api-Key can list the seller's products —
// the role the review→article mapping needs. One cheap page call.
func (c *Client) CheckProductsAccess(ctx context.Context) error {
	_, err := c.fetchProductPage(ctx, "")
	return err
}

// cachedProductArticles memoizes the product map so a paged review sync does
// not re-list the catalog on every page. A failed listing (typically an
// Api-Key without the products role) degrades to an empty map: seller
// articles then keep the marketplace sku — the pre-mapping behavior.
func (c *Client) cachedProductArticles(ctx context.Context) map[string]string {
	if c.productArticles != nil && time.Since(c.productArticlesAt) < productMapTTL {
		return c.productArticles
	}
	articles, err := c.ProductArticles(ctx)
	if err != nil {
		articles = map[string]string{}
	}
	c.productArticles = articles
	c.productArticlesAt = time.Now()
	return articles
}

func (c *Client) fetchProductPage(ctx context.Context, cursor string) (ozonProductListResponse, error) {
	requestPayload := ozonProductListRequest{
		Filter: ozonProductListFilter{Visibility: "ALL"},
		Limit:  c.pageSize,
		LastID: cursor,
	}
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return ozonProductListResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v3/product/list", bytes.NewReader(body))
	if err != nil {
		return ozonProductListResponse{}, err
	}
	req.Header.Set("Client-Id", c.clientID)
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ozonProductListResponse{}, err
	}
	defer resp.Body.Close()

	var payload ozonProductListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ozonProductListResponse{}, fmt.Errorf("decode Ozon product list response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if payload.Message != "" {
			return ozonProductListResponse{}, fmt.Errorf("Ozon product list: status %d: %s", resp.StatusCode, payload.Message)
		}
		return ozonProductListResponse{}, fmt.Errorf("Ozon product list: status %d", resp.StatusCode)
	}
	return payload, nil
}

type ozonProductListRequest struct {
	Filter ozonProductListFilter `json:"filter"`
	Limit  int                   `json:"limit"`
	LastID string                `json:"last_id,omitempty"`
}

type ozonProductListFilter struct {
	Visibility string `json:"visibility"`
}

type ozonProductListResponse struct {
	Result struct {
		Items  []ozonProductListItem `json:"items"`
		LastID string                `json:"last_id"`
	} `json:"result"`
	Message string `json:"message"`
}

type ozonProductListItem struct {
	OfferID string     `json:"offer_id"`
	SKU     flexString `json:"sku"`
}
