package api

import (
	"mime"
	"net/http"
	"strings"
)

var allowedMutationContentTypes = map[string]struct{}{
	"application/json":                  {},
	"application/yaml":                  {},
	"application/x-yaml":                {},
	"text/yaml":                         {},
	"application/vnd.api+json":          {},
	"application/vnd.oai.openapi+json": {},
}

func (s *Server) withContentTypeCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
		default:
			next.ServeHTTP(w, r)
			return
		}
		raw := strings.TrimSpace(r.Header.Get("Content-Type"))
		if raw == "" {
			next.ServeHTTP(w, r)
			return
		}
		mediaType, _, err := mime.ParseMediaType(raw)
		if err != nil {
			http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
			return
		}
		mediaType = strings.ToLower(strings.TrimSpace(mediaType))
		if _, ok := allowedMutationContentTypes[mediaType]; !ok {
			http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
			return
		}
		next.ServeHTTP(w, r)
	})
}
