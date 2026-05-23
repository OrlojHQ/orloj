package a2a

// AgentCard represents an A2A Agent Card as defined by the A2A protocol.
type AgentCard struct {
	Name            string           `json:"name"`
	Description     string           `json:"description,omitempty"`
	URL             string           `json:"url"`
	Version         string           `json:"version,omitempty"`
	ProtocolVersion string           `json:"protocolVersion,omitempty"`
	Capabilities    CardCapabilities `json:"capabilities,omitempty"`
	Skills          []CardSkill      `json:"skills,omitempty"`
	Authentication  *CardAuth        `json:"authentication,omitempty"`
	Provider        *CardProvider    `json:"provider,omitempty"`
}

type CardCapabilities struct {
	Streaming         bool `json:"streaming,omitempty"`
	PushNotifications bool `json:"pushNotifications,omitempty"`
	StateTransitions  bool `json:"stateTransitionHistory,omitempty"`
}

type CardSkill struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
}

type CardAuth struct {
	Schemes []string `json:"schemes,omitempty"`
}

type CardProvider struct {
	Organization string `json:"organization,omitempty"`
	URL          string `json:"url,omitempty"`
}

// JSON-RPC types
type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id"`
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	ErrCodeParse          = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
	ErrCodeTaskNotFound   = -32001
	ErrCodeTaskCancelled  = -32002
	ErrCodeAgentNotFound  = -32003
)

// A2A Task types
type TaskSendParams struct {
	ID            string            `json:"id"`
	Message       TaskMessage       `json:"message"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	HistoryLength *int              `json:"historyLength,omitempty"`
}

type TaskGetParams struct {
	ID            string `json:"id"`
	HistoryLength *int   `json:"historyLength,omitempty"`
}

type TaskCancelParams struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
}

type TaskMessage struct {
	Role  string     `json:"role"`
	Parts []TaskPart `json:"parts"`
}

type TaskPart struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	Data     any            `json:"data,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type TaskResult struct {
	ID        string            `json:"id"`
	Status    TaskStatus        `json:"status"`
	Artifacts []TaskArtifact    `json:"artifacts,omitempty"`
	History   []TaskMessage     `json:"history,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type TaskStatus struct {
	State   string       `json:"state"`
	Message *TaskMessage `json:"message,omitempty"`
}

type TaskArtifact struct {
	Name        string     `json:"name,omitempty"`
	Description string     `json:"description,omitempty"`
	Parts       []TaskPart `json:"parts"`
	Index       int        `json:"index"`
}

// A2A Task states
const (
	TaskStateSubmitted     = "submitted"
	TaskStateWorking       = "working"
	TaskStateInputRequired = "input-required"
	TaskStateCompleted     = "completed"
	TaskStateFailed        = "failed"
	TaskStateCanceled      = "canceled"
	TaskStateRejected      = "rejected"
)

// SSE event types
type TaskStatusEvent struct {
	ID     string     `json:"id"`
	Status TaskStatus `json:"status"`
	Final  bool       `json:"final,omitempty"`
}

type TaskArtifactEvent struct {
	ID       string       `json:"id"`
	Artifact TaskArtifact `json:"artifact"`
}

// RemoteAgentEntry represents a configured remote A2A agent in the registry.
type RemoteAgentEntry struct {
	Name            string     `json:"name"`
	URL             string     `json:"url"`
	ProtocolVersion string     `json:"protocolVersion,omitempty"`
	CacheStatus     string     `json:"cacheStatus,omitempty"`
	LastRefreshed   string     `json:"lastRefreshed,omitempty"`
	CacheTTL        string     `json:"cacheTTL,omitempty"`
	Error           string     `json:"error,omitempty"`
	Card            *AgentCard `json:"card,omitempty"`
}

// RegistryResponse is the response for GET /v1/a2a/agents
type RegistryResponse struct {
	LocalAgents  []AgentCard        `json:"localAgents"`
	RemoteAgents []RemoteAgentEntry `json:"remoteAgents"`
}

// A2A method names (current and legacy)
const (
	MethodTaskSend      = "tasks/send"
	MethodTaskGet       = "tasks/get"
	MethodTaskCancel    = "tasks/cancel"
	MethodTaskSubscribe = "tasks/sendSubscribe"
)
