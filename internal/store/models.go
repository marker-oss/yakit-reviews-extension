package store

import "time"

type Product struct {
	ID             uint `gorm:"primaryKey"`
	Title          *string
	SiteProductKey *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ProductMarketplaceLink struct {
	ID                uint `gorm:"primaryKey"`
	ProductID         uint `gorm:"index;not null"`
	Product           Product
	Marketplace       string `gorm:"size:16;not null;uniqueIndex:idx_marketplace_product"`
	ExternalProductID string `gorm:"size:128;not null;uniqueIndex:idx_marketplace_product"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Review struct {
	ID                uint   `gorm:"primaryKey"`
	Marketplace       string `gorm:"size:16;not null;uniqueIndex:idx_marketplace_review"`
	ExternalReviewID  string `gorm:"size:128;not null;uniqueIndex:idx_marketplace_review"`
	ExternalProductID string `gorm:"size:128;not null;index"`
	SellerArticle     string `gorm:"size:128;index"`
	ProductID         *uint
	Product           *Product
	Rating            *int
	AuthorName        string
	Text              string
	Pros              string
	Cons              string
	CreatedAtMP       time.Time `gorm:"not null;index"`
	UpdatedAtMP       *time.Time
	MPAnswerText      *string
	MPAnswerState     *string
	Status            string `gorm:"size:32;not null;default:imported"`
	Raw               string
	FetchedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time
	Media             []ReviewMedia `gorm:"foreignKey:ReviewID"`
}

type ReviewMedia struct {
	ID         uint `gorm:"primaryKey"`
	ReviewID   uint `gorm:"not null;uniqueIndex:idx_review_media_url"`
	Review     Review
	Kind       string `gorm:"size:16;not null"`
	URL        string `gorm:"not null;uniqueIndex:idx_review_media_url"`
	PreviewURL *string
	Position   int
}

type SyncState struct {
	Marketplace  string `gorm:"size:16;primaryKey"`
	LastSyncedAt *time.Time
	Backfilled   bool `gorm:"not null;default:false"`
}

type SyncRun struct {
	ID              uint      `gorm:"primaryKey"`
	Marketplace     string    `gorm:"size:16;not null;index"`
	StartedAt       time.Time `gorm:"not null;index"`
	FinishedAt      *time.Time
	Status          string `gorm:"size:16;not null;index"`
	ReviewsSeen     int
	ReviewsUpserted int
	ErrorText       *string
}
