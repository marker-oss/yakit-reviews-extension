package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"reviews/internal/auth"
	"reviews/internal/store"
)

const (
	maxSubmissionBytes = 80 << 20
	maxSubmissionMedia = 5
	maxImageBytes      = 8 << 20
	maxVideoBytes      = 50 << 20
	minFormOpenTime    = 4 * time.Second
	maxFormOpenTime    = 2 * time.Hour
	maxCustomFields    = 6
	maxCustomOptions   = 12
	maxCustomStrLen    = 60
)

// customField describes one extra selectable parameter on the review form
// (товарные атрибуты вида "рост / вес / посадка").
type customField struct {
	ID            string   `json:"id"`
	Label         string   `json:"label"`
	Type          string   `json:"type"` // select | chips | text
	Options       []string `json:"options"`
	Required      bool     `json:"required"`
	Filterable    bool     `json:"filterable,omitempty"`    // expose as public /api/reviews filter
	ShowInReview  bool     `json:"showInReview,omitempty"`  // render answer inside the review card
	ShowInSummary bool     `json:"showInSummary,omitempty"` // aggregate into the widget summary
}

// normalizeCustomFields validates the admin-configured custom-field list.
// Anything violating the limits is dropped rather than rejected so a bad
// saved config can never brick the submission endpoint.
func normalizeCustomFields(raw []customField) []customField {
	if len(raw) > maxCustomFields {
		raw = raw[:maxCustomFields]
	}
	out := make([]customField, 0, len(raw))
	seen := map[string]bool{}
	for _, field := range raw {
		field.ID = strings.TrimSpace(field.ID)
		field.Label = strings.TrimSpace(field.Label)
		field.Type = strings.TrimSpace(field.Type)
		if field.ID == "" || len(field.ID) > maxCustomStrLen || field.Label == "" || len(field.Label) > maxCustomStrLen {
			continue
		}
		if seen[field.ID] {
			continue
		}
		if field.Type != "select" && field.Type != "chips" && field.Type != "text" {
			continue
		}
		if len(field.Options) > maxCustomOptions {
			field.Options = field.Options[:maxCustomOptions]
		}
		options := make([]string, 0, len(field.Options))
		for _, option := range field.Options {
			option = strings.TrimSpace(option)
			if option == "" || len(option) > maxCustomStrLen {
				continue
			}
			options = append(options, option)
		}
		field.Options = options
		if field.Type != "text" && len(field.Options) < 2 {
			continue
		}
		seen[field.ID] = true
		out = append(out, field)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// customAnswersFromForm parses the widget's `custom` form value
// (JSON object {fieldId: value}) against the configured fields: values must
// match a configured option, required fields must be present, and unknown
// ids are dropped.
func customAnswersFromForm(raw string, fields []customField) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	var answers map[string]string
	if err := json.Unmarshal([]byte(raw), &answers); err != nil {
		return "", errors.New("invalid custom answers payload")
	}
	clean := make(map[string]string, len(fields))
	for _, field := range fields {
		value, ok := answers[field.ID]
		if !ok {
			if field.Required {
				return "", fmt.Errorf("field %q is required", field.Label)
			}
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			if field.Required {
				return "", fmt.Errorf("field %q is required", field.Label)
			}
			continue
		}
		if field.Type != "text" {
			match := false
			for _, option := range field.Options {
				if option == value {
					match = true
					break
				}
			}
			if !match {
				return "", fmt.Errorf("invalid value for field %q", field.Label)
			}
		}
		clean[field.ID] = value
	}
	if len(clean) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(clean)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

var allowedSubmissionMedia = map[string]struct {
	kind string
	max  int64
	ext  string
}{
	"image/jpeg":      {kind: "photo", max: maxImageBytes, ext: ".jpg"},
	"image/png":       {kind: "photo", max: maxImageBytes, ext: ".png"},
	"image/webp":      {kind: "photo", max: maxImageBytes, ext: ".webp"},
	"video/mp4":       {kind: "video", max: maxVideoBytes, ext: ".mp4"},
	"video/webm":      {kind: "video", max: maxVideoBytes, ext: ".webm"},
	"video/quicktime": {kind: "video", max: maxVideoBytes, ext: ".mov"},
}

type submissionLimiter struct {
	mu             sync.Mutex
	byIP           map[string][]time.Time
	byEmailArticle map[string][]time.Time
}

func newSubmissionLimiter() *submissionLimiter {
	return &submissionLimiter{
		byIP:           map[string][]time.Time{},
		byEmailArticle: map[string][]time.Time{},
	}
}

func (l *submissionLimiter) allow(now time.Time, ip, email, article string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	ipTimes := l.byIP[ip]
	if countSince(ipTimes, now.Add(-time.Hour)) >= 3 {
		return "too many submissions from this network, try later", false
	}
	if countSince(ipTimes, now.Add(-24*time.Hour)) >= 10 {
		return "daily submission limit reached for this network", false
	}
	key := email + "|" + strings.ToLower(strings.TrimSpace(article))
	if countSince(l.byEmailArticle[key], now.Add(-24*time.Hour)) >= 1 {
		return "this email already submitted a review for this product today", false
	}
	l.byIP[ip] = append(ipTimes, now)
	l.byEmailArticle[key] = append(l.byEmailArticle[key], now)
	return "", true
}

func (l *submissionLimiter) prune(now time.Time) {
	cutoff := now.Add(-24 * time.Hour)
	for key, values := range l.byIP {
		l.byIP[key] = filterSince(values, cutoff)
		if len(l.byIP[key]) == 0 {
			delete(l.byIP, key)
		}
	}
	for key, values := range l.byEmailArticle {
		l.byEmailArticle[key] = filterSince(values, cutoff)
		if len(l.byEmailArticle[key]) == 0 {
			delete(l.byEmailArticle, key)
		}
	}
}

func filterSince(values []time.Time, cutoff time.Time) []time.Time {
	out := values[:0]
	for _, value := range values {
		if value.After(cutoff) {
			out = append(out, value)
		}
	}
	return out
}

func countSince(values []time.Time, cutoff time.Time) int {
	n := 0
	for _, value := range values {
		if value.After(cutoff) {
			n++
		}
	}
	return n
}

type submissionConfigResponse struct {
	Enabled        bool          `json:"enabled"`
	MaxFiles       int           `json:"maxFiles"`
	MaxImageBytes  int64         `json:"maxImageBytes"`
	MaxVideoBytes  int64         `json:"maxVideoBytes"`
	MaxTotalBytes  int64         `json:"maxTotalBytes"`
	AllowedTypes   []string      `json:"allowedTypes"`
	PrivacyURL     string        `json:"privacyUrl,omitempty"`
	ReviewTermsURL string        `json:"reviewTermsUrl,omitempty"`
	CustomFields   []customField `json:"customFields,omitempty"`
}

func (s *Server) handleReviewSubmissionConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, submissionConfigResponse{
		Enabled:        true,
		MaxFiles:       maxSubmissionMedia,
		MaxImageBytes:  maxImageBytes,
		MaxVideoBytes:  maxVideoBytes,
		MaxTotalBytes:  maxSubmissionBytes,
		AllowedTypes:   []string{"image/jpeg", "image/png", "image/webp", "video/mp4", "video/webm", "video/quicktime"},
		PrivacyURL:     s.agreementURL(r),
		ReviewTermsURL: s.reviewTermsURL(r),
		CustomFields:   s.customFields(r.Context()),
	})
}

// customFields loads the admin-configured custom form fields. Uses the
// widget-config payload so settings ride the existing versioned publish flow.
func (s *Server) customFields(ctx context.Context) []customField {
	// tenantScope stamps the tenant before this runs; read the product-context
	// widget config which the admin Editor manages.
	cfg, err := s.store.GetActiveWidgetConfig(ctx, "product")
	if err != nil {
		return nil
	}
	var payload struct {
		CustomFields []customField `json:"customFields"`
	}
	if err := json.Unmarshal([]byte(cfg.Payload), &payload); err != nil {
		return nil
	}
	return normalizeCustomFields(payload.CustomFields)
}

func (s *Server) handleCreateReviewSubmission(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSubmissionBytes)
	if err := r.ParseMultipartForm(maxSubmissionBytes); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid multipart form"))
		return
	}
	if reason := validateSubmissionTrap(r); reason != "" {
		writeError(w, http.StatusTooManyRequests, errors.New(reason))
		return
	}
	sellerArticle := strings.TrimSpace(r.FormValue("sellerArticle"))
	rating, err := strconv.Atoi(strings.TrimSpace(r.FormValue("rating")))
	if err != nil || rating < 1 || rating > 5 {
		writeError(w, http.StatusBadRequest, errors.New("rating must be between 1 and 5"))
		return
	}
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
		writeError(w, http.StatusBadRequest, errors.New("review text is required"))
		return
	}
	customData, err := customAnswersFromForm(r.FormValue("custom"), s.customFields(r.Context()))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// A single consent checkbox covers the seller's user-agreement / personal
	// data page. termsConsent is accepted for backward compatibility but defaults
	// to the privacy consent when the form omits it.
	if !formBool(r, "privacyConsent") {
		writeError(w, http.StatusBadRequest, errors.New("consent is required"))
		return
	}
	now := time.Now().UTC()
	ip := clientIP(r)
	if s.submissions == nil {
		s.submissions = newSubmissionLimiter()
	}
	if reason, ok := s.submissions.allow(now, ip, authorEmail, sellerArticle); !ok {
		writeError(w, http.StatusTooManyRequests, errors.New(reason))
		return
	}

	media, cleanup, err := s.saveSubmissionMedia(r.MultipartForm)
	if err != nil {
		cleanup()
		writeError(w, http.StatusBadRequest, err)
		return
	}
	token, err := auth.NewSessionToken()
	if err != nil {
		cleanup()
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	review, err := s.store.CreateSiteReview(r.Context(), store.SiteReviewInput{
		ExternalReviewID: "site-" + token,
		SellerArticle:    sellerArticle,
		Rating:           rating,
		AuthorName:       authorName,
		AuthorEmail:      authorEmail,
		Text:             text,
		Pros:             r.FormValue("pros"),
		Cons:             r.FormValue("cons"),
		IPHash:           store.HashPII(ip),
		UserAgentHash:    store.HashPII(r.UserAgent()),
		Origin:           r.Header.Get("Origin"),
		Referrer:         r.Referer(),
		PrivacyConsentAt: now,
		TermsConsentAt:   now,
		Media:            media,
		CustomData:       customData,
	})
	if err != nil {
		cleanup()
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"status": "pending", "reviewId": review.ID})
}

func validateSubmissionTrap(r *http.Request) string {
	if strings.TrimSpace(r.FormValue("website")) != "" {
		return "submission rejected"
	}
	raw := strings.TrimSpace(r.FormValue("openedAt"))
	if raw == "" {
		return "form timing is required"
	}
	openedMs, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return "invalid form timing"
	}
	opened := time.UnixMilli(openedMs)
	age := time.Since(opened)
	if age < minFormOpenTime {
		return "form submitted too quickly"
	}
	if age > maxFormOpenTime {
		return "form expired"
	}
	return ""
}

func formBool(r *http.Request, key string) bool {
	switch strings.ToLower(strings.TrimSpace(r.FormValue(key))) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func (s *Server) saveSubmissionMedia(form *multipart.Form) ([]store.ReviewMedia, func(), error) {
	cleanupPaths := []string{}
	cleanup := func() {
		for _, path := range cleanupPaths {
			_ = os.Remove(path)
		}
	}
	if form == nil || len(form.File["media"]) == 0 {
		return nil, cleanup, nil
	}
	files := form.File["media"]
	if len(files) > maxSubmissionMedia {
		return nil, cleanup, fmt.Errorf("maximum %d media files are allowed", maxSubmissionMedia)
	}
	uploadDir := s.cfg.UploadDir
	if uploadDir == "" {
		uploadDir = "data/review-uploads"
	}
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		return nil, cleanup, fmt.Errorf("create upload dir: %w", err)
	}
	media := make([]store.ReviewMedia, 0, len(files))
	for i, header := range files {
		item, path, err := saveOneSubmissionMedia(uploadDir, header, i+1)
		if err != nil {
			return nil, cleanup, err
		}
		cleanupPaths = append(cleanupPaths, path)
		media = append(media, item)
	}
	return media, cleanup, nil
}

func saveOneSubmissionMedia(uploadDir string, header *multipart.FileHeader, position int) (store.ReviewMedia, string, error) {
	src, err := header.Open()
	if err != nil {
		return store.ReviewMedia{}, "", err
	}
	defer src.Close()

	head := make([]byte, 512)
	n, _ := io.ReadFull(src, head)
	head = head[:n]
	contentType := http.DetectContentType(head)
	if declared := header.Header.Get("Content-Type"); declared != "" {
		if mediatype, _, err := mime.ParseMediaType(declared); err == nil {
			if _, ok := allowedSubmissionMedia[mediatype]; ok {
				contentType = mediatype
			}
		}
	}
	spec, ok := allowedSubmissionMedia[contentType]
	if !ok {
		return store.ReviewMedia{}, "", fmt.Errorf("unsupported media type %s", contentType)
	}
	if header.Size > spec.max {
		return store.ReviewMedia{}, "", fmt.Errorf("media file is too large")
	}
	token, err := auth.NewSessionToken()
	if err != nil {
		return store.ReviewMedia{}, "", err
	}
	path := filepath.Join(uploadDir, token+spec.ext)
	dst, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return store.ReviewMedia{}, "", err
	}
	defer dst.Close()
	written, err := dst.Write(head)
	if err != nil {
		return store.ReviewMedia{}, "", err
	}
	written64, err := io.Copy(dst, io.LimitReader(src, spec.max-int64(written)+1))
	if err != nil {
		return store.ReviewMedia{}, "", err
	}
	size := int64(written) + written64
	if size > spec.max {
		_ = os.Remove(path)
		return store.ReviewMedia{}, "", fmt.Errorf("media file is too large")
	}
	return store.ReviewMedia{
		Kind:        spec.kind,
		URL:         "/user-media/" + token,
		StoragePath: path,
		MIMEType:    contentType,
		SizeBytes:   size,
		AccessToken: token,
		Position:    position,
	}, path, nil
}

func (s *Server) handleUserMedia(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		http.NotFound(w, r)
		return
	}
	media, err := s.store.MediaByAccessToken(r.Context(), token)
	if err != nil || media.StoragePath == "" {
		http.NotFound(w, r)
		return
	}
	public := media.Review.Visibility == "visible" && (media.Review.Status == "approved" || media.Review.Status == "imported")
	if !public {
		if _, ok := s.validSession(r); !ok {
			writeError(w, http.StatusUnauthorized, errors.New("authentication required"))
			return
		}
	}
	if media.MIMEType != "" {
		w.Header().Set("Content-Type", media.MIMEType)
	}
	http.ServeFile(w, r, media.StoragePath)
}

func (s *Server) validSession(r *http.Request) (uint, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return 0, false
	}
	sess, err := s.store.GetValidSession(r.Context(), cookie.Value, time.Now())
	if err != nil {
		return 0, false
	}
	return sess.UserID, true
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
		if first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
