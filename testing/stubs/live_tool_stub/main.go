package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type retryState struct {
	count    int
	lastSeen time.Time
}

type stubServer struct {
	logger       *log.Logger
	authHeader   string
	authValue    string
	retryReset   time.Duration
	logRequests  bool
	mu           sync.Mutex
	retryByInput map[string]retryState
}

type contractResponse struct {
	ToolContractVersion string         `json:"tool_contract_version,omitempty"`
	Status              string         `json:"status"`
	Output              contractOutput `json:"output,omitempty"`
}

type contractOutput struct {
	Result string `json:"result,omitempty"`
}

func main() {
	listenAddr := flag.String("listen", ":18080", "listen address")
	authHeader := flag.String("auth-header", "X-Stub-Key", "header required by /tool/auth")
	authValue := flag.String("auth-value", "stub-auth-key", "expected header value for /tool/auth")
	retryReset := flag.Duration("retry-reset", 5*time.Second, "idle duration after which retry-once state resets")
	logRequests := flag.Bool("log-requests", true, "log incoming requests")
	flag.Parse()

	logger := log.New(os.Stdout, "live-tool-stub ", log.LstdFlags)
	server := &stubServer{
		logger:       logger,
		authHeader:   strings.TrimSpace(*authHeader),
		authValue:    strings.TrimSpace(*authValue),
		retryReset:   *retryReset,
		logRequests:  *logRequests,
		retryByInput: make(map[string]retryState),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", server.handleHealth)
	mux.HandleFunc("/tool/smoke", server.handleSmoke)
	mux.HandleFunc("/tool/decision", server.handleDecision)
	mux.HandleFunc("/tool/auth", server.handleAuth)
	mux.HandleFunc("/tool/retry-once", server.handleRetryOnce)

	logger.Printf("listening on %s", strings.TrimSpace(*listenAddr))
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		logger.Fatalf("listen failed: %v", err)
	}
}

func (s *stubServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *stubServer) handleSmoke(w http.ResponseWriter, r *http.Request) {
	raw, payload := s.readBody(r)
	result := fmt.Sprintf(
		"TOOL_ENDPOINT=smoke PAYLOAD_OK=true ECHO_GOAL=%s ECHO_TIMESTAMP_HINT=%s RAW_SHA=%s STUB_HEALTH=healthy",
		valueFor(payload, "goal"),
		valueFor(payload, "timestamp_hint"),
		hashText(raw),
	)
	s.writeContract(w, result)
}

func (s *stubServer) handleDecision(w http.ResponseWriter, r *http.Request) {
	_, payload := s.readBody(r)
	result := fmt.Sprintf(
		"TOOL_ENDPOINT=decision DECISION_SOURCE=stub ECHO_GOAL=%s ECHO_CLAIM=%s EVIDENCE_TOKEN=external-check",
		valueFor(payload, "goal"),
		valueFor(payload, "claim"),
	)
	s.writeContract(w, result)
}

func (s *stubServer) handleAuth(w http.ResponseWriter, r *http.Request) {
	_, payload := s.readBody(r)
	if strings.TrimSpace(r.Header.Get(s.authHeader)) != s.authValue {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "missing or invalid auth header",
		})
		return
	}
	result := fmt.Sprintf(
		"AUTH_STATUS=ok FLOW=contract-auth ECHO_GOAL=%s HEADER_NAME=%s",
		valueFor(payload, "goal"),
		s.authHeader,
	)
	s.writeContract(w, result)
}

func (s *stubServer) handleRetryOnce(w http.ResponseWriter, r *http.Request) {
	raw, payload := s.readBody(r)
	key := hashText("retry-once:" + raw)
	attempt := s.incrementRetry(key)
	if attempt == 1 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "transient stub failure",
			"attempt": "1",
			"key":     key,
		})
		return
	}
	result := fmt.Sprintf(
		"RETRY_RESULT=recovered ATTEMPT=%d RETRY_CASE=%s BODY_HASH=%s",
		attempt,
		valueFor(payload, "retry_case"),
		key,
	)
	s.writeContract(w, result)
}

func (s *stubServer) readBody(r *http.Request) (string, map[string]string) {
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	raw := strings.TrimSpace(string(body))
	if s.logRequests && s.logger != nil {
		s.logger.Printf("%s %s body=%s", r.Method, r.URL.Path, compact(raw))
	}
	if raw == "" {
		return "", map[string]string{}
	}
	var generic map[string]any
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		return raw, map[string]string{"_raw": raw}
	}
	out := make(map[string]string, len(generic))
	for key, value := range generic {
		out[key] = strings.TrimSpace(fmt.Sprint(value))
	}
	return raw, out
}

func (s *stubServer) incrementRetry(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.retryByInput[key]
	now := time.Now().UTC()
	if current.lastSeen.IsZero() || now.Sub(current.lastSeen) > s.retryReset {
		current.count = 0
	}
	current.count++
	current.lastSeen = now
	s.retryByInput[key] = current
	return current.count
}

func (s *stubServer) writeContract(w http.ResponseWriter, result string) {
	writeJSON(w, http.StatusOK, contractResponse{
		ToolContractVersion: "v1",
		Status:              "ok",
		Output:              contractOutput{Result: result},
	})
}

func valueFor(payload map[string]string, key string) string {
	value := strings.TrimSpace(payload[key])
	if value != "" {
		return value
	}
	return "n/a"
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func compact(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 240 {
		return value
	}
	return value[:240]
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
