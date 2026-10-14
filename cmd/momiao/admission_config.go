package main

import (
	"errors"
	"net/http"
)

func admissionConfig(cfg *config, lookup func(string) (string, bool)) error {
	enabled, present := lookup("MOMIAO_ADMISSION_ENABLED")
	if present && enabled != "true" && enabled != "false" {
		return errors.New("MOMIAO_ADMISSION_ENABLED must be true or false")
	}
	cfg.AdmissionEnabled = enabled == "true"
	if !cfg.AdmissionEnabled {
		return nil
	}
	if cfg.WebDir == "" || cfg.PublicOrigin == "" || cfg.WalletDSNFile == "" {
		return errors.New("admission requires portal, public origin and local wallet configuration")
	}
	return nil
}

func newAdmissionConfigHandler(enabled bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		walletSuccess(w, struct {
			Enabled             bool   `json:"enabled"`
			RegistrationEnabled bool   `json:"registration_enabled"`
			Eligibility         string `json:"eligibility"`
		}{enabled, enabled, "新用户需加入指定 Discord 服务器并取得所需身份组；已有绑定账户可直接登录。"})
	})
}
