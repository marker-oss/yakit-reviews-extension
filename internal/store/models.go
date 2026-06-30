package store

import "time"

type Product struct {
	ID             uint `gorm:"primaryKey"`
	TenantID       uint `gorm:"not null;default:1;index"`
	Title          *string
	SiteProductKey *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ProductMarketplaceLink struct {
	ID                uint `gorm:"primaryKey"`
	TenantID          uint `gorm:"not null;default:1;index;uniqueIndex:idx_marketplace_product"`
	ProductID         uint `gorm:"index;not null"`
	Product           Product
	Marketplace       string `gorm:"size:16;not null;uniqueIndex:idx_marketplace_product"`
	ExternalProductID string `gorm:"size:128;not null;uniqueIndex:idx_marketplace_product"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Review struct {
	ID                 uint   `gorm:"primaryKey"`
	TenantID           uint   `gorm:"not null;default:1;index;uniqueIndex:idx_marketplace_review"`
	Marketplace        string `gorm:"size:16;not null;uniqueIndex:idx_marketplace_review"`
	ExternalReviewID   string `gorm:"size:128;not null;uniqueIndex:idx_marketplace_review"`
	ExternalProductID  string `gorm:"size:128;not null;index"`
	SellerArticle      string `gorm:"size:128;index"`
	ProductID          *uint
	Product            *Product
	ReviewerIdentityID *uint
	ReviewerIdentity   *ReviewerIdentity
	Rating             *int
	AuthorName         string
	Text               string
	Pros               string
	Cons               string
	CreatedAtMP        time.Time `gorm:"not null;index"`
	UpdatedAtMP        *time.Time
	MPAnswerText       *string
	MPAnswerState      *string
	Status             string `gorm:"size:32;not null;default:imported"`
	AdminReplyText     *string
	AdminReplyAt       *time.Time
	ReplyPublishState  *string `gorm:"size:16;index"`
	ReplyPublishError  *string
	ReplyPublishedAt   *time.Time
	Visibility         string `gorm:"size:16;not null;default:visible;index"`
	Pinned             bool   `gorm:"not null;default:false;index"`
	AuthorEmailHash    string `gorm:"size:64;index"`
	SubmissionIPHash   string `gorm:"size:64;index"`
	SubmissionUAHash   string `gorm:"size:64"`
	SubmissionOrigin   string `gorm:"size:256"`
	SubmissionReferrer string `gorm:"size:512"`
	ConsentPrivacyAt   *time.Time
	ConsentTermsAt     *time.Time
	AntispamReason     string `gorm:"size:256"`
	Raw                string
	FetchedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time
	Media              []ReviewMedia `gorm:"foreignKey:ReviewID"`
}

type ReviewerIdentity struct {
	ID              uint   `gorm:"primaryKey"`
	TenantID        uint   `gorm:"not null;default:1;index;uniqueIndex:idx_reviewer_email"`
	EmailNormalized string `gorm:"size:320;not null;uniqueIndex:idx_reviewer_email"`
	EmailHash       string `gorm:"size:64;not null;index"`
	VerifiedAt      *time.Time
	ShopCustomerID  *string `gorm:"size:128;index"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ReviewMedia struct {
	ID          uint `gorm:"primaryKey"`
	ReviewID    uint `gorm:"not null;uniqueIndex:idx_review_media_url"`
	Review      Review
	Kind        string `gorm:"size:16;not null"`
	URL         string `gorm:"not null;uniqueIndex:idx_review_media_url"`
	PreviewURL  *string
	StoragePath string
	MIMEType    string `gorm:"size:128"`
	SizeBytes   int64
	AccessToken string `gorm:"size:64;index"`
	Position    int
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
