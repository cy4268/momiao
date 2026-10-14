package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func sessionAPIRoute(name string) bool {
	return name == "/api/v1/session" || strings.HasPrefix(name, "/api/v1/session/") ||
		name == "/api/v1/auth" || strings.HasPrefix(name, "/api/v1/auth/") ||
		name == "/api/v1/account" || strings.HasPrefix(name, "/api/v1/account/")
}

func sessionEnvelope(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Success bool `json:"success"`
		Data    any  `json:"data"`
	}{true, data})
}
