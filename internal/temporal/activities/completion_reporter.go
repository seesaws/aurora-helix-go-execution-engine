package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/domain"
)

var callbackPrefix = regexp.MustCompile(`^(?:/[A-Za-z0-9_.-]+)*$`)

type TokenProvider func(context.Context) (string, error)
type CompletionReporter struct {
	base   string
	token  TokenProvider
	client *http.Client
}

func NewCompletionReporter(base string, token TokenProvider, client *http.Client) (*CompletionReporter, error) {
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawPath != "" {
		return nil, fmt.Errorf("SOP_BASE_URL must be a fixed HTTP base without credentials, query or fragment")
	}
	prefix := strings.TrimSuffix(parsed.Path, "/")
	if !callbackPrefix.MatchString(prefix) || prefix != "" && path.Clean(prefix) != prefix {
		return nil, fmt.Errorf("invalid fixed SOP context prefix")
	}
	base = parsed.Scheme + "://" + parsed.Host + prefix
	if token == nil {
		return nil, fmt.Errorf("callback token provider required")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	transport := *client
	transport.Timeout = 10 * time.Second
	// A redirect must not leak the service credential to another destination.
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &CompletionReporter{base: strings.TrimSuffix(base, "/"), token: token, client: &transport}, nil
}
func (r *CompletionReporter) DeliverResult(ctx context.Context, result domain.WorkerResult) error {
	if !domain.IsTerminal(result.Status) {
		return fmt.Errorf("callback requires terminal result")
	}
	token, err := r.token(ctx)
	if err != nil || token == "" {
		return fmt.Errorf("callback service credential unavailable")
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("invalid callback result")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.base+"/internal/sop/service-dispatches/"+url.PathEscape(result.DispatchID)+"/result", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("invalid callback request")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := r.client.Do(request)
	if err != nil {
		return fmt.Errorf("callback transport unavailable")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("callback rejected with HTTP %d", response.StatusCode)
	}
	return nil
}
