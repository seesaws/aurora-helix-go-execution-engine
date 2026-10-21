package http

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/app"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
)

type Server struct {
	logger       *slog.Logger
	dispatcher   app.Dispatcher
	serviceToken string
}

func NewServer(logger *slog.Logger, dispatcher app.Dispatcher, serviceToken string) *Server {
	return &Server{logger: logger, dispatcher: dispatcher, serviceToken: serviceToken}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /dispatches", s.protected(s.handleDispatch))
	mux.HandleFunc("POST /dispatches/{id}/cancel", s.protected(s.handleCancel))
	mux.HandleFunc("GET /results/", s.protected(s.handleResult))
	return mux
}

func (s *Server) protected(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expected := sha256.Sum256([]byte("Bearer " + s.serviceToken))
		actual := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if s.serviceToken == "" || subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
			s.writeError(w, http.StatusUnauthorized, fmt.Errorf("machine authentication required"))
			return
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

func (s *Server) handleDispatch(w http.ResponseWriter, r *http.Request) {
	var req CreateDispatchRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("invalid dispatch JSON"))
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("expected one dispatch object"))
		return
	}
	dispatch := domain.DispatchRecord{
		DispatchID:          req.DispatchID,
		TenantID:            req.TenantID,
		ExecutionGeneration: req.ExecutionGeneration,
		ProcessInstanceID:   req.ProcessInstanceID,
		NodeInstanceID:      req.NodeInstanceID,
		NodeKey:             req.NodeKey,
		TaskType:            req.TaskType,
		IdempotencyKey:      req.IdempotencyKey,
		Input:               req.Input,
		Metadata:            req.Metadata,
		Retry: domain.RetryPolicy{
			MaxAttempts: req.Retry.MaxAttempts,
			Backoff:     req.Retry.Backoff,
		},
		TimeoutSeconds: req.TimeoutSeconds,
	}
	dispatch, err := domain.NormalizeDispatch(dispatch)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	ack, err := s.dispatcher.Dispatch(r.Context(), dispatch)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			s.writeError(w, http.StatusConflict, domain.ErrConflict)
		} else {
			s.logger.Error("dispatch service unavailable")
			s.writeError(w, http.StatusServiceUnavailable, fmt.Errorf("dispatch service unavailable"))
		}
		return
	}
	s.writeJSON(w, http.StatusOK, CreateDispatchResponse{DispatchID: ack.DispatchID, Status: ack.Status})
}

func (s *Server) handleResult(w http.ResponseWriter, r *http.Request) {
	dispatchID := strings.TrimPrefix(r.URL.Path, "/results/")
	if dispatchID == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("dispatchId is required"))
		return
	}
	result, err := s.dispatcher.GetResult(r.Context(), dispatchID)
	if err != nil {
		s.writeDispatchError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	ack, err := s.dispatcher.Cancel(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeDispatchError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, ack)
}
func (s *Server) writeDispatchError(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, domain.ErrNotFound)
	} else {
		s.writeError(w, http.StatusServiceUnavailable, fmt.Errorf("dispatch service unavailable"))
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) writeError(w http.ResponseWriter, status int, err error) {
	s.writeJSON(w, status, map[string]any{"error": err.Error()})
}
