package wb

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
	marketplaceID   = config.MarketplaceWB
	defaultBaseURL  = "https://feedbacks-api.wildberries.ru"
	defaultPageSize = 100
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	pageSize   int
}

func New(cfg config.WBConfig) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		token:      cfg.Token,
		httpClient: http.DefaultClient,
		pageSize:   defaultPageSize,
	}
}

func NewWithHTTPClient(cfg config.WBConfig, baseURL string, httpClient *http.Client, pageSize int) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      cfg.Token,
		httpClient: httpClient,
		pageSize:   pageSize,
	}
}

func (c *Client) Marketplace() string {
	return marketplaceID
}

func (c *Client) FetchReviews(ctx context.Context, since time.Time, cursor string) ([]marketplace.Review, string, error) {
	stage, skip, err := parseCursor(cursor)
	if err != nil {
		return nil, "", err
	}

	feedbacks, err := c.fetchFeedbacks(ctx, stage.answered, since, skip)
	if err != nil {
		return nil, "", err
	}

	reviews := make([]marketplace.Review, 0, len(feedbacks))
	for _, feedback := range feedbacks {
		review, err := feedback.toReview()
		if err != nil {
			return nil, "", err
		}
		reviews = append(reviews, review)
	}

	if len(feedbacks) >= c.pageSize {
		return reviews, formatCursor(stage, skip+c.pageSize), nil
	}
	if !stage.answered {
		return reviews, formatCursor(wbStage{answered: true}, 0), nil
	}
	return reviews, "", nil
}

func (c *Client) fetchFeedbacks(ctx context.Context, answered bool, since time.Time, skip int) ([]wbFeedback, error) {
	endpoint, err := url.Parse(c.baseURL + "/api/v1/feedbacks")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("isAnswered", strconv.FormatBool(answered))
	query.Set("take", strconv.Itoa(c.pageSize))
	query.Set("skip", strconv.Itoa(skip))
	query.Set("order", "dateDesc")
	if !since.IsZero() {
		query.Set("dateFrom", strconv.FormatInt(since.Unix(), 10))
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload wbFeedbacksResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode WB feedbacks response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if payload.ErrorText != "" {
			return nil, fmt.Errorf("WB feedbacks: status %d: %s", resp.StatusCode, payload.ErrorText)
		}
		return nil, fmt.Errorf("WB feedbacks: status %d", resp.StatusCode)
	}
	if payload.Error {
		return nil, fmt.Errorf("WB feedbacks: %s", payload.ErrorText)
	}

	return payload.Data.Feedbacks, nil
}

type wbStage struct {
	answered bool
}

func parseCursor(cursor string) (wbStage, int, error) {
	if cursor == "" {
		return wbStage{answered: false}, 0, nil
	}

	name, skipText, ok := strings.Cut(cursor, ":")
	if !ok {
		return wbStage{}, 0, fmt.Errorf("invalid WB cursor %q", cursor)
	}
	skip, err := strconv.Atoi(skipText)
	if err != nil || skip < 0 {
		return wbStage{}, 0, fmt.Errorf("invalid WB cursor %q", cursor)
	}

	switch name {
	case "unanswered":
		return wbStage{answered: false}, skip, nil
	case "answered":
		return wbStage{answered: true}, skip, nil
	default:
		return wbStage{}, 0, fmt.Errorf("invalid WB cursor %q", cursor)
	}
}

func formatCursor(stage wbStage, skip int) string {
	if stage.answered {
		return fmt.Sprintf("answered:%d", skip)
	}
	return fmt.Sprintf("unanswered:%d", skip)
}

type wbFeedbacksResponse struct {
	Data      wbFeedbacksData `json:"data"`
	Error     bool            `json:"error"`
	ErrorText string          `json:"errorText"`
}

type wbFeedbacksData struct {
	Feedbacks []wbFeedback `json:"feedbacks"`
}

type wbFeedback struct {
	ID               string           `json:"id"`
	Text             string           `json:"text"`
	Pros             string           `json:"pros"`
	Cons             string           `json:"cons"`
	ProductValuation int              `json:"productValuation"`
	CreatedDate      string           `json:"createdDate"`
	Answer           *wbAnswer        `json:"answer"`
	ProductDetails   wbProductDetails `json:"productDetails"`
	PhotoLinks       []wbPhotoLink    `json:"photoLinks"`
	Video            *wbVideo         `json:"video"`
	UserName         string           `json:"userName"`
}

type wbAnswer struct {
	Text  string `json:"text"`
	State string `json:"state"`
}

type wbProductDetails struct {
	NMID            int64  `json:"nmId"`
	SupplierArticle string `json:"supplierArticle"`
}

type wbPhotoLink struct {
	FullSize string `json:"fullSize"`
	MiniSize string `json:"miniSize"`
}

type wbVideo struct {
	PreviewImage string `json:"previewImage"`
	Link         string `json:"link"`
}

func (f wbFeedback) toReview() (marketplace.Review, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, f.CreatedDate)
	if err != nil {
		return marketplace.Review{}, fmt.Errorf("parse WB createdDate for %s: %w", f.ID, err)
	}

	raw, err := json.Marshal(f)
	if err != nil {
		return marketplace.Review{}, err
	}

	rating := f.ProductValuation
	var answer *marketplace.Answer
	if f.Answer != nil {
		answer = &marketplace.Answer{
			Text:  f.Answer.Text,
			State: f.Answer.State,
		}
	}

	media := make([]marketplace.Media, 0, len(f.PhotoLinks)+1)
	for i, photo := range f.PhotoLinks {
		if photo.FullSize == "" {
			continue
		}
		media = append(media, marketplace.Media{
			Kind:       "photo",
			URL:        photo.FullSize,
			PreviewURL: photo.MiniSize,
			Position:   i,
		})
	}
	if f.Video != nil && f.Video.Link != "" {
		media = append(media, marketplace.Media{
			Kind:       "video",
			URL:        f.Video.Link,
			PreviewURL: f.Video.PreviewImage,
			Position:   len(media),
		})
	}

	return marketplace.Review{
		Marketplace:       marketplaceID,
		ExternalReviewID:  f.ID,
		ExternalProductID: f.externalProductID(),
		SellerArticle:     f.ProductDetails.SupplierArticle,
		Rating:            &rating,
		AuthorName:        f.UserName,
		Text:              f.Text,
		Pros:              f.Pros,
		Cons:              f.Cons,
		CreatedAtMP:       createdAt,
		Answer:            answer,
		Media:             media,
		Raw:               raw,
	}, nil
}

func (f wbFeedback) externalProductID() string {
	if f.ProductDetails.NMID > 0 {
		return strconv.FormatInt(f.ProductDetails.NMID, 10)
	}
	return f.ProductDetails.SupplierArticle
}

// NOTE: WB question field names not yet verified against live docs — confirm before enabling Q&A MP-fetch in production.
type wbQuestionsResponse struct {
	Data      wbQuestionsData `json:"data"`
	Error     bool            `json:"error"`
	ErrorText string          `json:"errorText"`
}

type wbQuestionsData struct {
	Questions []wbQuestion `json:"questions"`
}

type wbQuestion struct {
	ID             string           `json:"id"`
	Text           string           `json:"text"`
	CreatedDate    string           `json:"createdDate"`
	UserName       string           `json:"userName"`
	Answer         *wbAnswer        `json:"answer"`
	ProductDetails wbProductDetails `json:"productDetails"`
}

func (c *Client) FetchQuestions(ctx context.Context, since time.Time, cursor string) ([]marketplace.Question, string, error) {
	stage, skip, err := parseCursor(cursor)
	if err != nil {
		// If cursor is not in the WB review-cursor format, treat as empty.
		stage = wbStage{answered: false}
		skip = 0
	}

	endpoint, err := url.Parse(c.baseURL + "/api/v1/questions")
	if err != nil {
		return nil, "", err
	}
	query := endpoint.Query()
	query.Set("isAnswered", strconv.FormatBool(stage.answered))
	query.Set("take", strconv.Itoa(c.pageSize))
	query.Set("skip", strconv.Itoa(skip))
	if !since.IsZero() {
		query.Set("dateFrom", strconv.FormatInt(since.Unix(), 10))
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	var payload wbQuestionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, "", fmt.Errorf("decode WB questions response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if payload.ErrorText != "" {
			return nil, "", fmt.Errorf("WB questions: status %d: %s", resp.StatusCode, payload.ErrorText)
		}
		return nil, "", fmt.Errorf("WB questions: status %d", resp.StatusCode)
	}
	if payload.Error {
		return nil, "", fmt.Errorf("WB questions: %s", payload.ErrorText)
	}

	questions := make([]marketplace.Question, 0, len(payload.Data.Questions))
	for _, q := range payload.Data.Questions {
		createdAt, err := time.Parse(time.RFC3339Nano, q.CreatedDate)
		if err != nil {
			return nil, "", fmt.Errorf("parse WB question createdDate for %s: %w", q.ID, err)
		}
		var answer *marketplace.Answer
		if q.Answer != nil && q.Answer.Text != "" {
			answer = &marketplace.Answer{Text: q.Answer.Text, State: "published"}
		}
		questions = append(questions, marketplace.Question{
			ExternalQuestionID: q.ID,
			ExternalProductID:  q.externalProductID(),
			SellerArticle:      q.ProductDetails.SupplierArticle,
			AuthorName:         q.UserName,
			Text:               q.Text,
			Answer:             answer,
			CreatedAtMP:        createdAt,
		})
	}

	nextCursor := ""
	if len(payload.Data.Questions) >= c.pageSize {
		nextCursor = formatCursor(stage, skip+c.pageSize)
	} else if !stage.answered {
		nextCursor = formatCursor(wbStage{answered: true}, 0)
	}
	return questions, nextCursor, nil
}

func (q wbQuestion) externalProductID() string {
	if q.ProductDetails.NMID > 0 {
		return strconv.FormatInt(q.ProductDetails.NMID, 10)
	}
	return q.ProductDetails.SupplierArticle
}

func (c *Client) PublishQuestionAnswer(ctx context.Context, externalQuestionID, _ /*sku*/, text string) error {
	payload := map[string]any{"id": externalQuestionID, "answer": map[string]string{"text": text}, "state": "wbRu"}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+"/api/v1/questions", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("WB publish question answer: status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) PublishReply(ctx context.Context, externalReviewID, text string) error {
	body, err := json.Marshal(map[string]string{"id": externalReviewID, "text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/feedbacks/answer", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("WB publish reply: status %d", resp.StatusCode)
}
