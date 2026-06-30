package marketplace

import (
	"context"
	"time"
)

type Review struct {
	Marketplace       string
	ExternalReviewID  string
	ExternalProductID string
	SellerArticle     string
	Rating            *int
	AuthorName        string
	Text              string
	Pros              string
	Cons              string
	CreatedAtMP       time.Time
	UpdatedAtMP       *time.Time
	Answer            *Answer
	Media             []Media
	Raw               []byte
}

type Media struct {
	Kind       string
	URL        string
	PreviewURL string
	Position   int
}

type Answer struct {
	Text  string
	State string
}

type Adapter interface {
	Marketplace() string
	FetchReviews(ctx context.Context, since time.Time, cursor string) ([]Review, string, error)
}

// ReplyPublisher is implemented by adapters that can publish a seller reply
// back to the marketplace. Adapters that cannot (or for accounts lacking
// access) simply do not implement it; callers treat that as "unsupported".
type ReplyPublisher interface {
	PublishReply(ctx context.Context, externalReviewID, text string) error
}

// Question is a product question fetched from a marketplace.
type Question struct {
	ExternalQuestionID string
	ExternalProductID  string
	SellerArticle      string
	ExternalSKU        string // Ozon needs numeric SKU to answer; WB leaves this empty
	AuthorName         string
	Text               string
	CreatedAtMP        time.Time
}

// QuestionFetcher is implemented by adapters that can fetch unanswered product
// questions from the marketplace.
type QuestionFetcher interface {
	FetchQuestions(ctx context.Context, since time.Time, cursor string) ([]Question, string, error)
}

// QuestionAnswerPublisher is implemented by adapters that can publish a seller
// answer to a product question back to the marketplace. Adapters that cannot
// simply do not implement it; callers treat that as "unsupported".
type QuestionAnswerPublisher interface {
	PublishQuestionAnswer(ctx context.Context, externalQuestionID, sku, text string) error
}
