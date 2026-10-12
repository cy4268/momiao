package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdmissionConfigDefaultsAndBounds(t *testing.T) {
	cfg := config{}
	lookup := func(string) (string, bool) { return "", false }
	if err := admissionConfig(&cfg, lookup); err != nil || cfg.AdmissionEnabled {
		t.Fatal("default must be disabled")
	}
	for _, value := range []string{"yes", "1", "", "TRUE"} {
		err := admissionConfig(&cfg, func(k string) (string, bool) { return value, k == "MOMIAO_ADMISSION_ENABLED" })
		if err == nil {
			t.Fatal("invalid enable value accepted")
		}
	}
	cfg = config{WebDir: "web", PublicOrigin: "https://portal.example", WalletDSNFile: "wallet"}
	err := admissionConfig(&cfg, func(k string) (string, bool) {
		switch k {
		case "MOMIAO_ADMISSION_ENABLED":
			return "true", true
		}
		return "", false
	})
	if err != nil || !cfg.AdmissionEnabled {
		t.Fatal("local admission still requires native socket or reader key")
	}
}
func TestAdmissionPublicConfigNoAuthority(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		w := httptest.NewRecorder()
		newAdmissionConfigHandler(enabled).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/platform/v1/admission/config", nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "reader") || strings.Contains(w.Body.String(), "token") {
			t.Fatal("public config exposes authority")
		}
		w = httptest.NewRecorder()
		newAdmissionConfigHandler(enabled).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/platform/v1/admission/config", nil))
		if w.Code != 405 {
			t.Fatal("config accepted mutation")
		}
	}
}
