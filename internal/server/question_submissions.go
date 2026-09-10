package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"reviews/internal/store"
)

// questionSubmissionLimiter is a separate rate-limiter instance for site
// question submissions. Limits mirror the review submission limiter: 3/hour
// and 10/day per IP, plus 1/day per email+article pair.
var questionSubmissionLimiter = newSubmissionLimiter()

type questionSubmissionConfigResponse struct {
	Enabled      bool   `json:"enabled"`
	AgreementURL string `json:"agreementUrl,omitempty"`
}

func (s *Server) handleQuestionSubmissionConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, questionSubmissionConfigResponse{
		Enabled:      true,
		AgreementURL: s.agreementURL(r),
	})
}

func (s *Server) handleCreateQuestionSubmission(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid form"))
		return
	}

	if reason := validateSubmissionTrap(r); reason != "" {
		writeError(w, http.StatusTooManyRequests, errors.New(reason))
		return
	}

	sellerArticle := strings.TrimSpace(r.FormValue("sellerArticle"))
	authorName := strings.TrimSpace(r.FormValue("authorName"))
	if authorName == "" {
		writeError(w, http.StatusBadRequest, errors.New("author name is required"))
		return
	}

	authorEmail, err := store.NormalizeEmail(r.FormValue("authorEmail"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		writeError(w, http.StatusBadRequest, errors.New("question text is required"))
		return
	}

	if !formBool(r, "privacyConsent") {
		writeError(w, http.StatusBadRequest, errors.New("consent is required"))
		return
	}

	now := time.Now().UTC()
	ip := clientIP(r)

	if reason, ok := questionSubmissionLimiter.allow(now, ip, authorEmail, sellerArticle); !ok {
		writeError(w, http.StatusTooManyRequests, errors.New(reason))
		return
	}

	q, err := s.store.CreateSiteQuestion(r.Context(), store.SiteQuestionInput{
		SellerArticle:    sellerArticle,
		AuthorName:       authorName,
		AuthorEmail:      authorEmail,
		Text:             text,
		IPHash:           store.HashPII(ip),
		ConsentPrivacyAt: now,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"status": "pending", "questionId": q.ID})
}
