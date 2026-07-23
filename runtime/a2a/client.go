package a2a

import (
	"bytes"
	"context"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	lf "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/telemetry"
)

// Client handles outbound A2A requests to remote agents.
type Client struct {
	httpClient        *http.Client
	allowPrivate      bool
	skipURLValidation bool // test-only: bypasses ValidateEndpointURL
	cardCache         map[string]*cachedCard
	cardCacheTTL      time.Duration
	requireSigned     bool
	resolveCardKey    CardKeyResolver
	mu                sync.RWMutex
}

type cachedCard struct {
	card      AgentCard
	fetchedAt time.Time
	err       error
}

// ClientConfig configures the outbound A2A client.
type ClientConfig struct {
	AllowPrivate       bool
	CardCacheTTL       time.Duration
	RequireSignedCards bool
	ResolveCardKey     CardKeyResolver
}

// CardKeyResolver resolves a trusted public key for a protected JWS kid.
type CardKeyResolver func(context.Context, string) (crypto.PublicKey, error)

// NewClient creates a new outbound A2A client with SSRF-safe HTTP.
func NewClient(config ClientConfig) *Client {
	ttl := config.CardCacheTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Client{
		httpClient:     agentruntime.SafeHTTPClient(config.AllowPrivate, 60*time.Second),
		allowPrivate:   config.AllowPrivate,
		cardCache:      make(map[string]*cachedCard),
		cardCacheTTL:   ttl,
		requireSigned:  config.RequireSignedCards,
		resolveCardKey: config.ResolveCardKey,
	}
}

// FetchCard retrieves and caches a remote Agent Card.
// Optional extraHeaders are applied to the HTTP request (e.g. auth).
func (c *Client) FetchCard(ctx context.Context, agentURL string, extraHeaders map[string]string) (AgentCard, error) {
	if !c.skipURLValidation {
		if err := agentruntime.ValidateEndpointURL(agentURL, c.allowPrivate); err != nil {
			return AgentCard{}, fmt.Errorf("a2a: unsafe agent URL: %w", err)
		}
	}

	c.mu.RLock()
	if cached, ok := c.cardCache[agentURL]; ok {
		if time.Since(cached.fetchedAt) < c.cardCacheTTL && cached.err == nil {
			c.mu.RUnlock()
			telemetry.RecordA2ACardCacheHit()
			return cached.card, nil
		}
	}
	c.mu.RUnlock()

	telemetry.RecordA2ACardCacheMiss()
	cardURL := resolveCardURL(agentURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cardURL, nil)
	if err != nil {
		return AgentCard{}, fmt.Errorf("a2a: build card request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.cacheError(agentURL, err)
		return AgentCard{}, fmt.Errorf("a2a: fetch card from %s: %w", cardURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		err := fmt.Errorf("a2a: card fetch returned %d: %s", resp.StatusCode, string(body))
		c.cacheError(agentURL, err)
		return AgentCard{}, err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return AgentCard{}, fmt.Errorf("a2a: read card body: %w", err)
	}

	var card AgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		return AgentCard{}, fmt.Errorf("a2a: decode card: %w", err)
	}
	if err := c.verifyAgentCard(ctx, card); err != nil {
		c.cacheError(agentURL, err)
		return AgentCard{}, fmt.Errorf("a2a: verify card: %w", err)
	}

	c.mu.Lock()
	c.cardCache[agentURL] = &cachedCard{card: card, fetchedAt: time.Now()}
	c.mu.Unlock()

	return card, nil
}

func (c *Client) verifyAgentCard(ctx context.Context, card AgentCard) error {
	if len(card.Signatures) == 0 {
		if c.requireSigned {
			return errors.New("Agent Card signature is required")
		}
		return nil
	}
	if c.resolveCardKey == nil {
		if c.requireSigned {
			return errors.New("Agent Card key resolver is required")
		}
		return nil
	}

	var verificationErrors []error
	for _, signature := range card.Signatures {
		keyID, err := AgentCardSignatureKeyID(signature)
		if err != nil {
			verificationErrors = append(verificationErrors, err)
			continue
		}
		publicKey, err := c.resolveCardKey(ctx, keyID)
		if err != nil {
			verificationErrors = append(verificationErrors, fmt.Errorf("resolve key %q: %w", keyID, err))
			continue
		}
		if err := VerifyAgentCardSignature(card, signature, publicKey); err != nil {
			verificationErrors = append(verificationErrors, fmt.Errorf("verify key %q: %w", keyID, err))
			continue
		}
		return nil
	}
	return fmt.Errorf("no trusted Agent Card signature verified: %w", errors.Join(verificationErrors...))
}

func (c *Client) cacheError(url string, err error) {
	c.mu.Lock()
	c.cardCache[url] = &cachedCard{err: err, fetchedAt: time.Now()}
	c.mu.Unlock()
}

// SendTask sends a task to a remote A2A agent via JSON-RPC.
func (c *Client) SendTask(ctx context.Context, agentURL string, params TaskSendParams, extraHeaders map[string]string) (TaskResult, error) {
	card, err := c.FetchCard(ctx, agentURL, extraHeaders)
	if err != nil && c.requireSigned {
		return TaskResult{}, fmt.Errorf("a2a: required Agent Card verification failed: %w", err)
	}
	if err == nil && cardSupportsV1(card) {
		return c.sendTaskV1(ctx, card, params, extraHeaders)
	}
	return c.callMethod(ctx, agentURL, MethodTaskSend, params, extraHeaders)
}

// GetTask retrieves a task status from a remote A2A agent.
func (c *Client) GetTask(ctx context.Context, agentURL string, params TaskGetParams, extraHeaders map[string]string) (TaskResult, error) {
	card, err := c.FetchCard(ctx, agentURL, extraHeaders)
	if err != nil && c.requireSigned {
		return TaskResult{}, fmt.Errorf("a2a: required Agent Card verification failed: %w", err)
	}
	if err == nil && cardSupportsV1(card) {
		client, callCtx, err := c.v1Client(ctx, card, extraHeaders)
		if err != nil {
			return TaskResult{}, err
		}
		task, err := client.GetTask(callCtx, &lf.GetTaskRequest{ID: lf.TaskID(params.ID), HistoryLength: params.HistoryLength})
		if err != nil {
			return TaskResult{}, fmt.Errorf("a2a: v1 GetTask: %w", err)
		}
		return taskResultFromV1(task), nil
	}
	return c.callMethod(ctx, agentURL, MethodTaskGet, params, extraHeaders)
}

// CancelTask cancels a task on a remote A2A agent.
func (c *Client) CancelTask(ctx context.Context, agentURL string, params TaskCancelParams, extraHeaders map[string]string) (TaskResult, error) {
	card, err := c.FetchCard(ctx, agentURL, extraHeaders)
	if err != nil && c.requireSigned {
		return TaskResult{}, fmt.Errorf("a2a: required Agent Card verification failed: %w", err)
	}
	if err == nil && cardSupportsV1(card) {
		client, callCtx, err := c.v1Client(ctx, card, extraHeaders)
		if err != nil {
			return TaskResult{}, err
		}
		task, err := client.CancelTask(callCtx, &lf.CancelTaskRequest{
			ID:       lf.TaskID(params.ID),
			Metadata: map[string]any{"reason": params.Reason},
		})
		if err != nil {
			return TaskResult{}, fmt.Errorf("a2a: v1 CancelTask: %w", err)
		}
		return taskResultFromV1(task), nil
	}
	return c.callMethod(ctx, agentURL, MethodTaskCancel, params, extraHeaders)
}

func (c *Client) sendTaskV1(ctx context.Context, card AgentCard, params TaskSendParams, extraHeaders map[string]string) (TaskResult, error) {
	parts := make(lf.ContentParts, 0, len(params.Message.Parts))
	for _, part := range params.Message.Parts {
		switch strings.ToLower(strings.TrimSpace(part.Type)) {
		case "", "text":
			parts = append(parts, lf.NewTextPart(part.Text))
		case "data":
			parts = append(parts, lf.NewDataPart(part.Data))
		default:
			return TaskResult{}, fmt.Errorf("a2a: unsupported v1 part type %q", part.Type)
		}
	}
	messageID := strings.TrimSpace(params.ID)
	if messageID == "" {
		messageID = lf.NewMessageID()
	}
	metadata := make(map[string]any, len(params.Metadata))
	for key, value := range params.Metadata {
		metadata[key] = value
	}
	client, callCtx, err := c.v1Client(ctx, card, extraHeaders)
	if err != nil {
		return TaskResult{}, err
	}
	result, err := client.SendMessage(callCtx, &lf.SendMessageRequest{
		Message: &lf.Message{
			ID:    messageID,
			Role:  lf.MessageRoleUser,
			Parts: parts,
		},
		Config:   &lf.SendMessageConfig{HistoryLength: params.HistoryLength},
		Metadata: metadata,
	})
	if err != nil {
		return TaskResult{}, fmt.Errorf("a2a: v1 SendMessage: %w", err)
	}
	switch value := result.(type) {
	case *lf.Task:
		return taskResultFromV1(value), nil
	case *lf.Message:
		return TaskResult{
			ID: string(value.TaskID),
			Status: TaskStatus{
				State:   TaskStateCompleted,
				Message: taskMessageFromV1(value),
			},
		}, nil
	default:
		return TaskResult{}, fmt.Errorf("a2a: unsupported v1 SendMessage result %T", result)
	}
}

func (c *Client) v1Client(ctx context.Context, card AgentCard, extraHeaders map[string]string) (*a2aclient.Client, context.Context, error) {
	interfaces := make([]*lf.AgentInterface, 0, len(card.SupportedInterfaces))
	for _, candidate := range card.SupportedInterfaces {
		if !strings.EqualFold(candidate.ProtocolBinding, string(lf.TransportProtocolJSONRPC)) {
			continue
		}
		interfaces = append(interfaces, &lf.AgentInterface{
			URL:             candidate.URL,
			ProtocolBinding: lf.TransportProtocolJSONRPC,
			ProtocolVersion: lf.ProtocolVersion(candidate.ProtocolVersion),
			Tenant:          candidate.Tenant,
		})
	}
	if len(interfaces) == 0 {
		return nil, ctx, errors.New("a2a: v1 card has no JSON-RPC interface")
	}
	client, err := a2aclient.NewFromEndpoints(
		ctx,
		interfaces,
		a2aclient.WithDefaultsDisabled(),
		a2aclient.WithJSONRPCTransport(c.httpClient),
	)
	if err != nil {
		return nil, ctx, fmt.Errorf("a2a: create v1 client: %w", err)
	}
	serviceParams := make(a2aclient.ServiceParams, len(extraHeaders)+1)
	for key, value := range extraHeaders {
		serviceParams.Append(key, value)
	}
	serviceParams.Append("A2A-Version", "1.0")
	return client, a2aclient.AttachServiceParams(ctx, serviceParams), nil
}

func cardSupportsV1(card AgentCard) bool {
	for _, candidate := range card.SupportedInterfaces {
		if strings.EqualFold(candidate.ProtocolBinding, string(lf.TransportProtocolJSONRPC)) &&
			strings.HasPrefix(candidate.ProtocolVersion, "1.") {
			return true
		}
	}
	return false
}

func taskResultFromV1(task *lf.Task) TaskResult {
	if task == nil {
		return TaskResult{}
	}
	result := TaskResult{
		ID:     string(task.ID),
		Status: TaskStatus{State: legacyStateFromV1(task.Status.State)},
	}
	result.Status.Message = taskMessageFromV1(task.Status.Message)
	for index, artifact := range task.Artifacts {
		if artifact == nil {
			continue
		}
		item := TaskArtifact{
			Name:        artifact.Name,
			Description: artifact.Description,
			Index:       index,
			Parts:       taskPartsFromV1(artifact.Parts),
		}
		result.Artifacts = append(result.Artifacts, item)
	}
	for _, message := range task.History {
		if converted := taskMessageFromV1(message); converted != nil {
			result.History = append(result.History, *converted)
		}
	}
	return result
}

func taskMessageFromV1(message *lf.Message) *TaskMessage {
	if message == nil {
		return nil
	}
	role := "user"
	if message.Role == lf.MessageRoleAgent {
		role = "agent"
	}
	return &TaskMessage{Role: role, Parts: taskPartsFromV1(message.Parts)}
}

func taskPartsFromV1(parts lf.ContentParts) []TaskPart {
	result := make([]TaskPart, 0, len(parts))
	for _, part := range parts {
		if part == nil {
			continue
		}
		switch value := part.Content.(type) {
		case lf.Text:
			result = append(result, TaskPart{Type: "text", Text: string(value), Metadata: part.Metadata})
		case lf.Data:
			result = append(result, TaskPart{Type: "data", Data: value.Value, Metadata: part.Metadata})
		case lf.URL:
			result = append(result, TaskPart{Type: "data", Data: string(value), Metadata: part.Metadata})
		case lf.Raw:
			result = append(result, TaskPart{Type: "data", Data: []byte(value), Metadata: part.Metadata})
		}
	}
	return result
}

func legacyStateFromV1(state lf.TaskState) string {
	switch state {
	case lf.TaskStateSubmitted:
		return TaskStateSubmitted
	case lf.TaskStateWorking:
		return TaskStateWorking
	case lf.TaskStateInputRequired, lf.TaskStateAuthRequired:
		return TaskStateInputRequired
	case lf.TaskStateCompleted:
		return TaskStateCompleted
	case lf.TaskStateCanceled:
		return TaskStateCanceled
	case lf.TaskStateRejected:
		return TaskStateRejected
	default:
		return TaskStateFailed
	}
}

func (c *Client) callMethod(ctx context.Context, agentURL string, method string, params any, extraHeaders map[string]string) (TaskResult, error) {
	start := time.Now()
	outStatus := "ok"
	defer func() {
		telemetry.RecordA2AOutbound(agentURL, outStatus, time.Since(start).Seconds())
	}()

	if !c.skipURLValidation {
		if err := agentruntime.ValidateEndpointURL(agentURL, c.allowPrivate); err != nil {
			outStatus = "error"
			return TaskResult{}, fmt.Errorf("a2a: unsafe URL: %w", err)
		}
	}

	rpcReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(rpcReq)
	if err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, agentURL, bytes.NewReader(body))
	if err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: HTTP %d: %s", resp.StatusCode, truncateBody(respBody, 256))
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: decode response: %w", err)
	}

	if rpcResp.Error != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: remote error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	resultBytes, err := json.Marshal(rpcResp.Result)
	if err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: re-marshal result: %w", err)
	}

	var result TaskResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		outStatus = "error"
		return TaskResult{}, fmt.Errorf("a2a: decode result: %w", err)
	}

	return result, nil
}

// resolveCardURL derives the well-known card URL from an A2A agent URL.
func resolveCardURL(agentURL string) string {
	agentURL = strings.TrimSuffix(agentURL, "/")
	if strings.HasSuffix(agentURL, "/a2a") {
		base := strings.TrimSuffix(agentURL, "/a2a")
		return base + "/.well-known/agent-card.json"
	}
	if strings.HasSuffix(agentURL, "/.well-known/agent-card.json") || strings.HasSuffix(agentURL, "/.well-known/agent.json") {
		return agentURL
	}
	return agentURL + "/.well-known/agent-card.json"
}

func truncateBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}

// CacheStatus returns info about a cached card entry for observability.
func (c *Client) CacheStatus(agentURL string) (lastRefreshed time.Time, hasError bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if cached, ok := c.cardCache[agentURL]; ok {
		return cached.fetchedAt, cached.err != nil
	}
	return time.Time{}, false
}
