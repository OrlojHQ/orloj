package agentruntime

import (
	"errors"
	"fmt"
	"net/http"
)

// ModelGatewayError is returned by model gateways when the upstream provider
// returns an HTTP error. Callers can inspect StatusCode to distinguish
// transient failures (5xx, 429) from permanent ones (4xx).
type ModelGatewayError struct {
	StatusCode int
	Provider   string
	Message    string
}

func (e *ModelGatewayError) Error() string {
	return fmt.Sprintf("model request failed status=%d: %s", e.StatusCode, e.Message)
}

// IsModelGatewayError returns the underlying *ModelGatewayError if err wraps
// one, along with a flag indicating whether the caller should retry.
func IsModelGatewayError(err error) (gatewayErr *ModelGatewayError, retryable bool) {
	var mge *ModelGatewayError
	if !errors.As(err, &mge) {
		return nil, false
	}
	switch {
	case mge.StatusCode == http.StatusTooManyRequests:
		return mge, true
	case mge.StatusCode >= http.StatusInternalServerError:
		return mge, true
	case mge.StatusCode >= http.StatusBadRequest:
		// 4xx errors (except 429) are permanent: bad request, auth failure,
		// model not found, etc. Retrying would just reproduce the same error.
		return mge, false
	default:
		return mge, true
	}
}
