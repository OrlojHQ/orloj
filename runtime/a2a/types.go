package a2a

// AgentCard represents an A2A Agent Card as defined by the A2A protocol.
type AgentCard struct {
	Name                 string                    `json:"name"`
	Description          string                    `json:"description"`
	URL                  string                    `json:"url,omitempty"`
	Version              string                    `json:"version"`
	ProtocolVersion      string                    `json:"protocolVersion,omitempty"`
	SupportedInterfaces  []AgentInterface          `json:"supportedInterfaces"`
	Capabilities         CardCapabilities          `json:"capabilities"`
	DefaultInputModes    []string                  `json:"defaultInputModes"`
	DefaultOutputModes   []string                  `json:"defaultOutputModes"`
	Skills               []CardSkill               `json:"skills"`
	Authentication       *CardAuth                 `json:"authentication,omitempty"`
	SecuritySchemes      map[string]map[string]any `json:"securitySchemes,omitempty"`
	SecurityRequirements []map[string][]string     `json:"securityRequirements,omitempty"`
	Signatures           []AgentCardSignature      `json:"signatures,omitempty"`
	Provider             *CardProvider             `json:"provider,omitempty"`
}

type CardCapabilities struct {
	Streaming         bool            `json:"streaming,omitempty"`
	PushNotifications bool            `json:"pushNotifications,omitempty"`
	ExtendedAgentCard bool            `json:"extendedAgentCard,omitempty"`
	StateTransitions  bool            `json:"stateTransitionHistory,omitempty"`
	Extensions        []CardExtension `json:"extensions,omitempty"`
}

type CardSkill struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
	InputModes  []string       `json:"inputModes,omitempty"`
	OutputModes []string       `json:"outputModes,omitempty"`
	Examples    []string       `json:"examples,omitempty"`
	Tags        []string       `json:"tags"`
}

type AgentInterface struct {
	URL             string `json:"url"`
	ProtocolBinding string `json:"protocolBinding"`
	Tenant          string `json:"tenant,omitempty"`
	ProtocolVersion string `json:"protocolVersion"`
}

type CardExtension struct {
	URI         string         `json:"uri,omitempty"`
	Description string         `json:"description,omitempty"`
	Required    bool           `json:"required,omitempty"`
	Params      map[string]any `json:"params,omitempty"`
}

type AgentCardSignature struct {
	Protected string         `json:"protected"`
	Signature string         `json:"signature"`
	Header    map[string]any `json:"header,omitempty"`
}

// PreferredInterface returns the first matching interface in server preference
// order. Empty binding or version values do not constrain selection.
func (c AgentCard) PreferredInterface(binding, version string) (AgentInterface, bool) {
	for _, candidate := range c.SupportedInterfaces {
		if binding != "" && candidate.ProtocolBinding != binding {
			continue
		}
		if version != "" && candidate.ProtocolVersion != version {
			continue
		}
		return candidate, true
	}
	return AgentInterface{}, false
}

// EffectiveProtocolVersion returns the preferred interface version while
// retaining compatibility with cards that only carry the legacy top-level field.
func (c AgentCard) EffectiveProtocolVersion() string {
	if len(c.SupportedInterfaces) > 0 && c.SupportedInterfaces[0].ProtocolVersion != "" {
		return c.SupportedInterfaces[0].ProtocolVersion
	}
	return c.ProtocolVersion
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
