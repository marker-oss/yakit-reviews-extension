package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"reviews/internal/store"
)

type adminQuestion struct {
	ID                 uint                   `json:"id"`
	Marketplace        string                 `json:"marketplace"`
	ExternalQuestionID string                 `json:"externalQuestionId"`
	ExternalProductID  string                 `json:"externalProductId"`
	SellerArticle      string                 `json:"sellerArticle"`
	AuthorName         string                 `json:"authorName"`
	Text               string                 `json:"text"`
	Status             string                 `json:"status"`
	Visibility         string                 `json:"visibility"`
	CreatedAtMP        time.Time              `json:"createdAt"`
	Answer             *adminQuestionAnswer   `json:"answer,omitempty"`
	AnswerPublish      *answerPublishStatus   `json:"answerPublish,omitempty"`
}

type adminQuestionAnswer struct {
	Text string     `json:"text"`
	At   *time.Time `json:"at,omitempty"`
}

type answerPublishStatus struct {
	State       string     `json:"state"`
	Error       string     `json:"error,omitempty"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
}

type adminQuestionsResponse struct {
	Questions []adminQuestion `json:"questions"`
}

func questionToAdmin(q store.Question) adminQuestion {
	aq := adminQuestion{
		ID:                 q.ID,
		Marketplace:        q.Marketplace,
		ExternalQuestionID: q.ExternalQuestionID,
		ExternalProductID:  q.ExternalProductID,
		SellerArticle:      q.SellerArticle,
		AuthorName:         q.AuthorName,
		Text:               q.Text,
		Status:             q.Status,
		Visibility:         q.Visibility,
		CreatedAtMP:        q.CreatedAtMP,
	}
	if q.AnswerText != nil && *q.AnswerText != "" {
		aq.Answer = &adminQuestionAnswer{
			Text: *q.AnswerText,
			At:   q.AnswerAt,
		}
	}
	if q.AnswerPublishState != nil {
		aq.AnswerPublish = &answerPublishStatus{
			State:       *q.AnswerPublishState,
			PublishedAt: q.AnswerPublishedAt,
		}
		if q.AnswerPublishError != nil {
			aq.AnswerPublish.Error = *q.AnswerPublishError
		}
	}
	return aq
}

func (s *Server) handleAdminQuestions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.QuestionFilter{
		Marketplace: q.Get("marketplace"),
		Status:      q.Get("status"),
		Visibility:  q.Get("visibility"),
		Limit:       parseInt(q.Get("limit"), 50),
		Offset:      parseInt(q.Get("offset"), 0),
	}

	questions, err := s.store.ListQuestions(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	items := make([]adminQuestion, 0, len(questions))
	for _, q := range questions {
		items = append(items, questionToAdmin(q))
	}
	writeJSON(w, http.StatusOK, adminQuestionsResponse{Questions: items})
}

type answerRequest struct {
	Text string `json:"text"`
}

func (s *Server) handleAdminQuestionAnswer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid question id"))
		return
	}

	var req answerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}

	if err := s.store.SetQuestionAnswer(r.Context(), uint(id), req.Text); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	q, err := s.store.QuestionByID(r.Context(), uint(id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.publishQuestionAnswer(r.Context(), q)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAdminQuestionAnswerRetry(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid question id"))
		return
	}

	q, err := s.store.QuestionByID(r.Context(), uint(id))
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("question not found"))
		return
	}

	if q.AnswerPublishState != nil && *q.AnswerPublishState == "published" {
		writeError(w, http.StatusConflict, errors.New("answer already published"))
		return
	}

	s.publishQuestionAnswer(r.Context(), q)

	updated, err := s.store.QuestionByID(r.Context(), uint(id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	state := ""
	if updated.AnswerPublishState != nil {
		state = *updated.AnswerPublishState
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": state})
}
