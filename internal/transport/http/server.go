package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/app"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
)

type Server struct {
	logger     *slog.Logger
	dispatcher app.Dispatcher
	now        func() time.Time
	idgen      func() string
}

func NewServer(logger *slog.Logger, dispatcher app.Dispatcher) *Server {
	return &Server{
		logger:     logger,
		dispatcher: dispatcher,
		now:        time.Now,
		idgen: func() string {
			return fmt.Sprintf("dispatch-%d", time.Now().UnixNano())
		},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /dispatches", s.handleDispatch)
	mux.HandleFunc("GET /results/", s.handleResult)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

func (s *Server) handleDispatch(w http.ResponseWriter, r *http.Request) {
	var req CreateDispatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.ProcessInstanceID == "" || req.NodeInstanceID == "" || req.NodeKey == "" || req.TaskType == "" {
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("processInstanceId, nodeInstanceId, nodeKey, and taskType are required"))
		return
	}
	if req.DispatchID == "" {
		req.DispatchID = s.idgen()
	}
	payload, err := json.Marshal(domain.TaskRequest{
		TaskType: req.TaskType,
		Input:    req.Input,
		Metadata: req.Metadata,
	})
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	dispatch := domain.DispatchRecord{
		DispatchID:        req.DispatchID,
		ProcessInstanceID: req.ProcessInstanceID,
		NodeInstanceID:    req.NodeInstanceID,
		NodeKey:           req.NodeKey,
		TaskType:          req.TaskType,
		IdempotencyKey:    req.IdempotencyKey,
		Input:             req.Input,
		Metadata:          req.Metadata,
		Retry: domain.RetryPolicy{
			MaxAttempts: req.Retry.MaxAttempts,
			Backoff:     req.Retry.Backoff,
		},
		TimeoutSeconds: req.TimeoutSeconds,
		DispatchStatus: domain.DispatchStatusDispatched,
		Payload:        payload,
		CreatedAt:      s.now(),
	}
	ack, err := s.dispatcher.Dispatch(r.Context(), dispatch)
	if err != nil {
		s.logger.Error("failed to process dispatch", "error", err)
		s.writeError(w, http.StatusInternalServerError, err)
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
		s.writeError(w, http.StatusNotFound, err)
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) writeError(w http.ResponseWriter, status int, err error) {
	s.writeJSON(w, status, map[string]any{"error": err.Error()})
}
