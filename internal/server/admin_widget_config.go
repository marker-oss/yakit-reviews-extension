package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"reviews/internal/store"
)

var errInvalidWidgetContext = errors.New("invalid widget context")

type widgetConfigVersionResponse struct {
	Version   int    `json:"version"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

func widgetContextFromRequest(r *http.Request) string {
	ctxName := r.PathValue("context")
	if ctxName == "" {
		ctxName = r.URL.Query().Get("context")
	}
	if ctxName == "" {
		ctxName = "product"
	}
	return ctxName
}

func validateWidgetContext(ctxName string) error {
	if ctxName != "product" && ctxName != "homepage" {
		return errInvalidWidgetContext
	}
	return nil
}

func (s *Server) handleGetWidgetConfig(w http.ResponseWriter, r *http.Request) {
	ctxName := widgetContextFromRequest(r)
	if err := validateWidgetContext(ctxName); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := s.store.GetActiveWidgetConfig(r.Context(), ctxName)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeRawJSON(w, http.StatusOK, cfg.Payload)
}

func (s *Server) handlePublishWidgetConfig(w http.ResponseWriter, r *http.Request) {
	ctxName := widgetContextFromRequest(r)
	if err := validateWidgetContext(ctxName); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128<<10))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, errors.New("invalid json"))
		return
	}
	cfg, err := s.store.PublishWidgetConfig(r.Context(), ctxName, string(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": cfg.Version})
}

func (s *Server) handleListWidgetConfigVersions(w http.ResponseWriter, r *http.Request) {
	ctxName := widgetContextFromRequest(r)
	if err := validateWidgetContext(ctxName); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	versions, err := s.store.ListWidgetConfigVersions(r.Context(), ctxName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items := make([]widgetConfigVersionResponse, 0, len(versions))
	for _, version := range versions {
		items = append(items, widgetConfigVersionResponse{
			Version:   version.Version,
			Active:    version.Active,
			CreatedAt: version.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": items})
}

func (s *Server) handleRollbackWidgetConfig(w http.ResponseWriter, r *http.Request) {
	ctxName := widgetContextFromRequest(r)
	if err := validateWidgetContext(ctxName); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid version"))
		return
	}
	if err := s.store.SetActiveWidgetConfigVersion(r.Context(), ctxName, version); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePublicWidgetConfig(w http.ResponseWriter, r *http.Request) {
	ctxName := widgetContextFromRequest(r)
	if err := validateWidgetContext(ctxName); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := s.store.GetActiveWidgetConfig(r.Context(), ctxName)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeRawJSON(w, http.StatusOK, cfg.Payload)
}

func writeRawJSON(w http.ResponseWriter, status int, payload string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(payload))
}
