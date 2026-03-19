package api

import "strings"

type AuthMode string

const (
	AuthModeOff   AuthMode = "off"
	AuthModeLocal AuthMode = "local"
	AuthModeSSO   AuthMode = "sso"
)

func normalizeAuthMode(raw string) AuthMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "off":
		return AuthModeOff
	case "local":
		return AuthModeLocal
	case "sso":
		return AuthModeSSO
	default:
		return AuthModeOff
	}
}
