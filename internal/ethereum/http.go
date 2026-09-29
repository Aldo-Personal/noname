package ethereum

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"infra.local/platform/internal/usage"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"infra.local/platform/internal/access"
)

type Authenticator interface {
	AuthenticateKey(context.Context, string) (access.KeyIdentity, error)
}

type Meter interface {
	Admit(context.Context, usage.Attempt) error
	Finish(context.Context, string, string) error
}

type Handler struct {
	meter   Meter
	metrics metrics
	auth    Authenticator
	client  *Client
	slots   chan struct{}
}

func NewHandler(auth Authenticator, client *Client, meter Meter) *Handler {
	return &Handler{auth: auth, client: client, meter: meter, slots: make(chan struct{}, 32)}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func failure(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code})
}

func rpcFailure(w http.ResponseWriter, status int, id json.RawMessage, err *rpcError) {
	if len(id) == 0 {
		id = json.RawMessage(`null`)
	}
	writeJSON(w, status, struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   *rpcError       `json:"error"`
	}{"2.0", id, err})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	observed := &observedWriter{ResponseWriter: w, status: 200}
	w = observed
	defer func() {
		h.metrics.requests[observed.status/100].Add(1)
		h.metrics.durationNS.Add(uint64(time.Since(started)))
	}()
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		failure(w, 405, "method_not_allowed")
		return
	}
	// Bound both database authorization and provider work, without an unbounded wait queue.
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		failure(w, 503, "gateway_busy")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	headers := r.Header.Values("Authorization")
	if len(headers) != 1 {
		failure(w, 401, "invalid_api_key")
		return
	}
	parts := strings.Split(headers[0], " ")
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		failure(w, 401, "invalid_api_key")
		return
	}
	identity, err := h.auth.AuthenticateKey(ctx, parts[1])
	if errors.Is(err, access.ErrNotFound) {
		failure(w, 401, "invalid_api_key")
		return
	}
	if err != nil {
		failure(w, 503, "authorization_unavailable")
		return
	}
	if identity.ChainID != 1 {
		failure(w, 403, "unsupported_chain")
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || r.Header.Get("Content-Encoding") != "" {
		failure(w, 415, "unsupported_media_type")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			failure(w, 413, "request_too_large")
		} else {
			failure(w, 400, "invalid_body")
		}
		return
	}
	req, invalid := parse(raw)
	if invalid != nil {
		rpcFailure(w, 400, req.ID, invalid)
		return
	}
	if h.meter == nil {
		failure(w, 503, "accounting_unavailable")
		return
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		failure(w, 503, "accounting_unavailable")
		return
	}
	attemptID := hex.EncodeToString(random[:])
	err = h.meter.Admit(ctx, usage.Attempt{ID: attemptID, OrganizationID: identity.OrganizationID, ProjectID: identity.ProjectID, KeyID: identity.KeyID, Method: req.Method})
	if err != nil {
		var limit *usage.Limited
		switch {
		case errors.As(err, &limit):
			w.Header().Set("Retry-After", strconv.Itoa(limit.RetryAfter))
			failure(w, 429, "project_limit_exceeded")
		case errors.Is(err, usage.ErrKey):
			failure(w, 401, "invalid_api_key")
		default:
			h.metrics.admissionErrors.Add(1)
			failure(w, 503, "accounting_unavailable")
		}
		return
	}
	w.Header().Set("X-Usage-Attempt-ID", attemptID)
	result, err := h.client.call(ctx, req)
	outcome := "succeeded"
	if err != nil {
		outcome = "upstream_error"
		if errors.Is(err, errTimeout) {
			outcome = "timed_out"
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			outcome = "canceled"
		}
	}
	finish, cancelFinish := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
	finishErr := h.meter.Finish(finish, attemptID, outcome)
	cancelFinish()
	if finishErr != nil {
		h.metrics.completionErrors.Add(1)
		slog.Error("usage completion pending reconciliation", "attempt_id", attemptID)
	}

	if err != nil {
		if errors.Is(err, errTimeout) {
			rpcFailure(w, 504, req.ID, &rpcError{-32002, "Upstream timeout"})
		} else {
			rpcFailure(w, 502, req.ID, &rpcError{-32001, "Upstream unavailable"})
		}
		return
	}
	writeJSON(w, 200, struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
	}{"2.0", req.ID, result})
}
