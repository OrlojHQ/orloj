package api

import "net/http"

// PublishResourceEventForTest exposes publishResourceEvent for external package tests.
func (s *Server) PublishResourceEventForTest(kind, name, action string, resource any) {
	s.publishResourceEvent(kind, name, action, resource)
}

// IsLongLivedRequestForTest exposes the write-timeout bypass predicate for tests.
func IsLongLivedRequestForTest(r *http.Request) bool {
	return isStreamingWatchRequest(r)
}
