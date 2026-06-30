export type Review = {
  id: number
  marketplace: string
  externalReviewId: string
  externalProductId: string
  sellerArticle?: string
  rating: number | null
  authorName: string
  text: string
  pros: string
  cons: string
  createdAt: string
  media: { kind: string; url: string; previewUrl?: string; position: number }[]
  visibility: 'visible' | 'hidden'
  pinned: boolean
  status: 'pending' | 'approved' | 'rejected' | 'imported' | 'deleted'
  authorEmail?: string
  submissionMeta?: Record<string, unknown>
  adminReply?: { text: string; at?: string }
  replyPublish?: { state: string; error?: string; publishedAt?: string }
}

export type DashboardData = {
  total_reviews: number
  visible_reviews: number
  pending_reviews: number
  deleted_reviews: number
  average_rating: number
  by_marketplace: Record<string, number>
  recent_syncs: {
    ID: number
    Marketplace: string
    StartedAt: string
    FinishedAt?: string
    Status: string
    ReviewsSeen: number
    ReviewsUpserted: number
    ErrorText?: string
  }[]
}

export type MarketplaceStatus = {
  id: string
  enabled: boolean
  configured: boolean
  fields?: Record<string, boolean>
}

export type Question = {
  id: number
  marketplace: string
  externalQuestionId: string
  externalProductId: string
  sellerArticle?: string
  authorName: string
  text: string
  status: string
  visibility: 'visible' | 'hidden'
  createdAt: string
  answer?: { text: string; at?: string }
  answerPublish?: { state: string; error?: string; publishedAt?: string }
}

export type ShowcaseRule = {
  ID?: number
  TenantID?: number
  MinRating: number
  RequirePhoto: boolean
  MinTextLen: number
  MaxAgeDays: number
  SortBy: 'recent' | 'rating'
  Limit: number
}
