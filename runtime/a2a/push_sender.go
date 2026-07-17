package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	lf "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
)

const notificationTokenHeader = "A2A-Notification-Token"

// SafePushSender delivers A2A task events while enforcing Orloj's dial-time
// SSRF policy, including DNS rebinding protection.
type SafePushSender struct {
	client       *http.Client
	allowPrivate bool
}

var _ push.Sender = (*SafePushSender)(nil)

func NewSafePushSender(allowPrivate bool, timeout time.Duration) *SafePushSender {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &SafePushSender{
		client:       agentruntime.SafeHTTPClient(allowPrivate, timeout),
		allowPrivate: allowPrivate,
	}
}

func (s *SafePushSender) SendPush(ctx context.Context, config *lf.PushConfig, event lf.Event) error {
	if config == nil || event == nil {
		return lf.ErrInvalidParams
	}
	if err := agentruntime.ValidateEndpointURL(config.URL, s.allowPrivate); err != nil {
		return fmt.Errorf("invalid A2A push endpoint: %w", err)
	}
	body, err := json.Marshal(lf.StreamResponse{Event: event})
	if err != nil {
		return fmt.Errorf("marshal A2A push event: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create A2A push request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if config.Token != "" {
		request.Header.Set(notificationTokenHeader, config.Token)
	}
	if config.Auth != nil && config.Auth.Credentials != "" {
		switch strings.ToLower(strings.TrimSpace(config.Auth.Scheme)) {
		case "bearer":
			request.Header.Set("Authorization", "Bearer "+config.Auth.Credentials)
		case "basic":
			request.Header.Set("Authorization", "Basic "+config.Auth.Credentials)
		default:
			return fmt.Errorf("unsupported A2A push authentication scheme %q", config.Auth.Scheme)
		}
	}

	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("deliver A2A push event: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("A2A push endpoint returned %s", response.Status)
	}
	return nil
}
