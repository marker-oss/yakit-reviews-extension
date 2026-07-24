package ym

import (
	"context"
	"encoding/json"
	"errors"
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

type ymQuestionsResponse struct {
	Status string            `json:"status"`
	Result ymQuestionsResult `json:"result"`
	Errors []ymError         `json:"errors"`
}

type ymQuestionsResult struct {
	Questions []goodsQuestion `json:"questions"`
	Paging    ymPaging        `json:"paging"`
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

type goodsQuestion struct {
	QuestionIdentifiers goodsQuestionIdentifiers `json:"questionIdentifiers"`
	Text                string                   `json:"text"`
	CreatedAt           string                   `json:"createdAt"`
	Author              goodsQuestionAuthor      `json:"author"`
}

type goodsQuestionIdentifiers struct {
	ID         int64  `json:"id"`
	OfferID    string `json:"offerId"`
	CategoryID int64  `json:"categoryId"`
}

type goodsQuestionAuthor struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

func (c *Client) FetchQuestions(ctx context.Context, since time.Time, cursor string) ([]marketplace.Question, string, error) {
	window, err := parseQuestionCursor(since, cursor)
	if err != nil {
		return nil, "", err
	}

	endpoint, err := url.Parse(fmt.Sprintf("%s/v1/businesses/%s/goods-questions", c.baseURL, c.businessID))
	if err != nil {
		return nil, "", err
	}
	query := endpoint.Query()
	query.Set("limit", strconv.Itoa(c.pageSize))
	if window.pageToken != "" {
		query.Set("pageToken", window.pageToken)
	}
	endpoint.RawQuery = query.Encode()

	body := map[string]any{
		"needAnswer": false,
		"sort":       "CREATED_AT_DESC",
		"dateFrom":   window.from.Format(time.DateOnly),
		"dateTo":     window.to.Format(time.DateOnly),
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(string(rawBody)))
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

	var payload ymQuestionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, "", fmt.Errorf("decode YM goods-questions response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(payload.Errors) > 0 {
			return nil, "", fmt.Errorf("YM goods-questions: status %d: %s", resp.StatusCode, payload.Errors[0].Message)
		}
		return nil, "", fmt.Errorf("YM goods-questions: status %d", resp.StatusCode)
	}

	questions := make([]marketplace.Question, 0, len(payload.Result.Questions))
	for _, q := range payload.Result.Questions {
		question, err := q.toQuestion(window.from)
		if err != nil {
			return nil, "", err
		}
		if question.ExternalQuestionID == "" {
			continue
		}
		answer, err := c.fetchQuestionAnswer(ctx, question.ExternalQuestionID)
		if err != nil {
			return nil, "", err
		}
		question.Answer = answer
		questions = append(questions, question)
	}

	if payload.Result.Paging.NextPageToken != "" {
		return questions, formatQuestionCursor(window.from, window.to, payload.Result.Paging.NextPageToken), nil
	}
	if next, ok := nextQuestionWindow(window); ok {
		return questions, formatQuestionCursor(next.from, next.to, ""), nil
	}
	return questions, "", nil
}

func (q goodsQuestion) toQuestion(since time.Time) (marketplace.Question, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, q.CreatedAt)
	if err != nil {
		return marketplace.Question{}, fmt.Errorf("parse YM question createdAt for %d: %w", q.QuestionIdentifiers.ID, err)
	}
	if !since.IsZero() && createdAt.Before(since) {
		return marketplace.Question{}, nil
	}
	offerID := q.QuestionIdentifiers.OfferID
	return marketplace.Question{
		ExternalQuestionID: strconv.FormatInt(q.QuestionIdentifiers.ID, 10),
		ExternalProductID:  offerID,
		SellerArticle:      offerID,
		AuthorName:         q.Author.Name,
		Text:               q.Text,
		CreatedAtMP:        createdAt,
	}, nil
}

type ymQuestionWindow struct {
	from      time.Time
	to        time.Time
	pageToken string
}

func parseQuestionCursor(since time.Time, cursor string) (ymQuestionWindow, error) {
	if cursor == "" {
		now := time.Now().UTC()
		from := since.UTC()
		if from.IsZero() {
			from = now.AddDate(0, -1, 0)
		}
		to := from.AddDate(0, 1, 0)
		if to.After(now) {
			to = now
		}
		return ymQuestionWindow{from: dateOnly(from), to: dateOnly(to)}, nil
	}

	fromText, rest, ok := strings.Cut(cursor, "|")
	if !ok {
		return ymQuestionWindow{}, errors.New("invalid YM questions cursor")
	}
	toText, pageToken, _ := strings.Cut(rest, "|")
	from, err := time.Parse(time.DateOnly, fromText)
	if err != nil {
		return ymQuestionWindow{}, fmt.Errorf("invalid YM questions cursor: %w", err)
	}
	to, err := time.Parse(time.DateOnly, toText)
	if err != nil {
		return ymQuestionWindow{}, fmt.Errorf("invalid YM questions cursor: %w", err)
	}
	return ymQuestionWindow{from: from, to: to, pageToken: pageToken}, nil
}

func nextQuestionWindow(current ymQuestionWindow) (ymQuestionWindow, bool) {
	now := dateOnly(time.Now().UTC())
	nextFrom := current.to
	if !nextFrom.Before(now) {
		return ymQuestionWindow{}, false
	}
	nextTo := nextFrom.AddDate(0, 1, 0)
	if nextTo.After(now) {
		nextTo = now
	}
	return ymQuestionWindow{from: nextFrom, to: nextTo}, true
}

func formatQuestionCursor(from, to time.Time, pageToken string) string {
	return from.Format(time.DateOnly) + "|" + to.Format(time.DateOnly) + "|" + pageToken
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

type ymAnswersResponse struct {
	Status string          `json:"status"`
	Result ymAnswersResult `json:"result"`
	Errors []ymError       `json:"errors"`
}

type ymAnswersResult struct {
	Answers []goodsQuestionAnswer `json:"answers"`
}

type goodsQuestionAnswer struct {
	Text   string              `json:"text"`
	Status string              `json:"status"`
	Author goodsQuestionAuthor `json:"author"`
}

func (c *Client) fetchQuestionAnswer(ctx context.Context, questionID string) (*marketplace.Answer, error) {
	id, err := strconv.ParseInt(questionID, 10, 64)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{"questionId": id})
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/v1/businesses/%s/goods-questions/answers?limit=%d", c.baseURL, c.businessID, c.pageSize)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload ymAnswersResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode YM goods-questions answers response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(payload.Errors) > 0 {
			return nil, fmt.Errorf("YM goods-questions answers: status %d: %s", resp.StatusCode, payload.Errors[0].Message)
		}
		return nil, fmt.Errorf("YM goods-questions answers: status %d", resp.StatusCode)
	}
	for _, answer := range payload.Result.Answers {
		if answer.Text != "" && answer.Status != "DELETED" {
			state := strings.ToLower(answer.Status)
			if state == "" {
				state = "published"
			}
			return &marketplace.Answer{Text: answer.Text, State: state}, nil
		}
	}
	return nil, nil
}

func (c *Client) PublishQuestionAnswer(ctx context.Context, externalQuestionID, _ /*sku*/, text string) error {
	questionID, err := strconv.ParseInt(externalQuestionID, 10, 64)
	if err != nil {
		return fmt.Errorf("YM publish question answer: bad question id %q: %w", externalQuestionID, err)
	}
	payload := map[string]any{
		"parentEntityId": map[string]any{"id": questionID, "type": "QUESTION"},
		"text":           text,
		"operationType":  "CREATE",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/v1/businesses/%s/goods-questions/update", c.baseURL, c.businessID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("YM publish question answer: status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) PublishReply(ctx context.Context, externalReviewID, text string) error {
	feedbackID, err := strconv.ParseInt(externalReviewID, 10, 64)
	if err != nil {
		return fmt.Errorf("YM publish reply: bad feedback id %q: %w", externalReviewID, err)
	}
	payload := map[string]any{
		"feedbackId": feedbackID,
		"comment":    map[string]string{"text": text},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/v2/businesses/%s/goods-feedback/comments/update", c.baseURL, c.businessID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("YM publish reply: status %d", resp.StatusCode)
	}
	return nil
}
