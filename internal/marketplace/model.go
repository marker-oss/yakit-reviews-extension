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
