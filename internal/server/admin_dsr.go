package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"reviews/internal/store"
)

// sessionUserID returns the admin user ID stored in the request context by
// requireSession. Returns 0 if no session is present (should not happen on
// protected routes).
func (s *Server) sessionUserID(r *http.Request) uint {
	id, _ := userIDFromContext(r.Context())
	return id
}

// dsrLookup parses the query string and fetches the subject export. It returns
// the export, a mode tag ("email" or "mp"), the email hash (non-empty for email
// mode), and any error. The email hash is used in DSRLog rows — the plaintext
// email is never logged.
func (s *Server) dsrLookup(r *http.Request) (store.SubjectExport, string, string, error) {
	q := r.URL.Query()
	if email := strings.TrimSpace(q.Get("email")); email != "" {
		exp, err := s.store.FindSubjectByEmail(r.Context(), email)
		return exp, "email", store.HashPII(strings.ToLower(email)), err
	}
	marketplace := strings.TrimSpace(q.Get("marketplace"))
	reviewID := strings.TrimSpace(q.Get("reviewId"))
	if marketplace == "" || reviewID == "" {
		return store.SubjectExport{}, "", "", errors.New("specify email, or marketplace+reviewId")
	}
	exp, err := s.store.FindReviewByExternalRef(r.Context(), marketplace, reviewID)
	return exp, "mp", "", err
}

func (s *Server) handleDSRLookup(w http.ResponseWriter, r *http.Request) {
	exp, _, emailHash, err := s.dsrLookup(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.logDSR(r, "lookup", emailHash, exp)
	writeJSON(w, http.StatusOK, exp)
}

func (s *Server) handleDSRExport(w http.ResponseWriter, r *http.Request) {
	exp, _, emailHash, err := s.dsrLookup(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.logDSR(r, "export", emailHash, exp)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="subject-data.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(exp)
}

type dsrDeleteRequest struct {
	Email       string `json:"email"`
	Marketplace string `json:"marketplace"`
	ReviewID    string `json:"reviewId"`
}

func (s *Server) handleDSRDelete(w http.ResponseWriter, r *http.Request) {
	var req dsrDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	var (
		deleted   int
		err       error
		emailHash string
		mp        string
		extID     string
	)
	switch {
	case strings.TrimSpace(req.Email) != "":
		emailHash = store.HashPII(strings.ToLower(strings.TrimSpace(req.Email)))
		deleted, err = s.store.PurgeSubjectByEmail(r.Context(), req.Email)
	case strings.TrimSpace(req.Marketplace) != "" && strings.TrimSpace(req.ReviewID) != "":
		mp, extID = strings.TrimSpace(req.Marketplace), strings.TrimSpace(req.ReviewID)
		deleted, err = s.store.PurgeReviewByExternalRef(r.Context(), mp, extID)
	default:
		writeError(w, http.StatusBadRequest, errors.New("specify email, or marketplace+reviewId"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	_ = s.store.WriteDSRLog(r.Context(), store.DSRLog{
		EmailHash: emailHash, Marketplace: mp, ExternalReviewID: extID,
		Action: "delete", AdminUserID: s.sessionUserID(r), At: time.Now().UTC(),
	})
	writeJSON(w, http.StatusOK, map[string]int{"deleted": deleted})
}

// logDSR writes a DSRLog row. The email hash is used; the plaintext email is
// never written to the log.
func (s *Server) logDSR(r *http.Request, action, emailHash string, exp store.SubjectExport) {
	mp, ext := "", ""
	if emailHash == "" && len(exp.Reviews) > 0 {
		mp, ext = exp.Reviews[0].Marketplace, exp.Reviews[0].ExternalReviewID
	}
	_ = s.store.WriteDSRLog(r.Context(), store.DSRLog{
		EmailHash: emailHash, Marketplace: mp, ExternalReviewID: ext,
		Action: action, AdminUserID: s.sessionUserID(r), At: time.Now().UTC(),
	})
}
