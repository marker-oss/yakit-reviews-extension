package ym

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"reviews/internal/config"
	"reviews/internal/marketplace"
)

const (
	marketplaceID   = config.MarketplaceYM
	defaultBaseURL  = "https://api.partner.market.yandex.ru"
	defaultPageSize = 50
)

type Client struct {
	baseURL    string
	apiKey     string
	businessID string
	httpClient *http.Client
	pageSize   int
}

func New(cfg config.YMConfig) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		apiKey:     cfg.APIKey,
		businessID: cfg.BusinessID,
		httpClient: http.DefaultClient,
		pageSize:   defaultPageSize,
	}
}

func NewWithHTTPClient(cfg config.YMConfig, baseURL string, httpClient *http.Client, pageSize int) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     cfg.APIKey,
		businessID: cfg.BusinessID,
		httpClient: httpClient,
		pageSize:   pageSize,
	}
}

func (c *Client) Marketplace() string {
	return marketplaceID
}

func (c *Client) FetchReviews(ctx context.Context, since time.Time, cursor string) ([]marketplace.Review, string, error) {
	feedbacks, nextToken, err := c.fetchPage(ctx, cursor)
	if err != nil {
		return nil, "", err
	}

	reviews := make([]marketplace.Review, 0, len(feedbacks))
	for _, feedback := range feedbacks {
		review, err := feedback.toReview()
		if err != nil {
			return nil, "", err
		}
		if !since.IsZero() && review.CreatedAtMP.Before(since) {
			continue
		}
		reviews = append(reviews, review)
	}

	return reviews, nextToken, nil
}

func (c *Client) fetchPage(ctx context.Context, cursor string) ([]goodsFeedback, string, error) {
	endpoint, err := url.Parse(fmt.Sprintf("%s/v2/businesses/%s/goods-feedback", c.baseURL, c.businessID))
	if err != nil {
		return nil, "", err
	}
	query := endpoint.Query()
	query.Set("limit", strconv.Itoa(c.pageSize))
	if cursor != "" {
		query.Set("page_token", cursor)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader("{}"))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	var payload ymFeedbacksResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, "", fmt.Errorf("decode YM goods-feedback response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(payload.Errors) > 0 {
			return nil, "", fmt.Errorf("YM goods-feedback: status %d: %s", resp.StatusCode, payload.Errors[0].Message)
		}
		return nil, "", fmt.Errorf("YM goods-feedback: status %d", resp.StatusCode)
	}

	return payload.Result.Feedbacks, payload.Result.Paging.NextPageToken, nil
}

type ymFeedbacksResponse struct {
	Status string            `json:"status"`
	Result ymFeedbacksResult `json:"result"`
	Errors []ymError         `json:"errors"`
}

type ymError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ymFeedbacksResult struct {
	Feedbacks []goodsFeedback `json:"feedbacks"`
	Paging    ymPaging        `json:"paging"`
}

type ymPaging struct {
	NextPageToken string `json:"nextPageToken"`
}

type goodsFeedback struct {
	FeedbackID   int64                    `json:"feedbackId"`
	CreatedAt    string                   `json:"createdAt"`
	NeedReaction bool                     `json:"needReaction"`
	Author       string                   `json:"author"`
	Identifiers  goodsFeedbackIdentifiers `json:"identifiers"`
	Description  goodsFeedbackDescription `json:"description"`
	Media        goodsFeedbackMedia       `json:"media"`
	Statistics   goodsFeedbackStatistics  `json:"statistics"`
}

type goodsFeedbackIdentifiers struct {
	OfferID   string `json:"offerId"`
	ShopSku   string `json:"shopSku"`
	MarketSku int64  `json:"marketSku"`
	ModelID   int64  `json:"modelId"`
}

type goodsFeedbackDescription struct {
	Advantages    string `json:"advantages"`
	Disadvantages string `json:"disadvantages"`
	Comment       string `json:"comment"`
}

type goodsFeedbackMedia struct {
	Photos []string `json:"photos"`
	Videos []string `json:"videos"`
}

type goodsFeedbackStatistics struct {
	Rating int `json:"rating"`
}

func (f goodsFeedback) toReview() (marketplace.Review, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, f.CreatedAt)
	if err != nil {
		return marketplace.Review{}, fmt.Errorf("parse YM createdAt for %d: %w", f.FeedbackID, err)
	}

	raw, err := json.Marshal(f)
	if err != nil {
		return marketplace.Review{}, err
	}

	var rating *int
	if f.Statistics.Rating > 0 {
		value := f.Statistics.Rating
		rating = &value
	}

	media := make([]marketplace.Media, 0, len(f.Media.Photos)+len(f.Media.Videos))
	for i, photo := range f.Media.Photos {
		if photo == "" {
			continue
		}
		media = append(media, marketplace.Media{
			Kind:     "photo",
			URL:      photo,
			Position: i,
		})
	}
	for _, video := range f.Media.Videos {
		if video == "" {
			continue
		}
		media = append(media, marketplace.Media{
			Kind:     "video",
			URL:      video,
			Position: len(media),
		})
	}

	return marketplace.Review{
		Marketplace:       marketplaceID,
		ExternalReviewID:  strconv.FormatInt(f.FeedbackID, 10),
		ExternalProductID: f.externalProductID(),
		SellerArticle:     f.Identifiers.OfferID,
		Rating:            rating,
		AuthorName:        f.Author,
		Text:              f.Description.Comment,
		Pros:              f.Description.Advantages,
		Cons:              f.Description.Disadvantages,
		CreatedAtMP:       createdAt,
		Media:             media,
		Raw:               raw,
	}, nil
}

func (f goodsFeedback) externalProductID() string {
	if f.Identifiers.MarketSku > 0 {
		return strconv.FormatInt(f.Identifiers.MarketSku, 10)
	}
	if f.Identifiers.ModelID > 0 {
		return strconv.FormatInt(f.Identifiers.ModelID, 10)
	}
	return f.Identifiers.OfferID
}
