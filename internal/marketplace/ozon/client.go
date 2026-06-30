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
)

type Client struct {
	baseURL    string
	clientID   string
	apiKey     string
	httpClient *http.Client
	pageSize   int
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

	reviews := make([]marketplace.Review, 0, len(payload.Reviews))
	stopPaging := false
	for _, item := range payload.Reviews {
		review, err := item.toReview()
		if err != nil {
			return nil, "", err
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
