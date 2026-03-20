package api

import "strings"

type AuthMode string

const (
	AuthModeOff    AuthMode = "off"
	AuthModeNative AuthMode = "native"
	AuthModeSSO    AuthMode = "sso"
)

func normalizeAuthMode(raw string) AuthMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "off":
		return AuthModeOff
	case "native":
		return AuthModeNative
	case "sso":
		return AuthModeSSO
	default:
		return AuthModeOff
	}
}
