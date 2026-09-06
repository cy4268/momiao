package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func admissionConfig(cfg *config, lookup func(string) (string, bool)) error {
	enabled, present := lookup("MOMIAO_ADMISSION_ENABLED")
	if present && enabled != "true" && enabled != "false" {
		return errors.New("MOMIAO_ADMISSION_ENABLED must be true or false")
	}
	cfg.AdmissionEnabled = enabled == "true"
	file, hasFile := lookup("MOMIAO_REGISTRATION_READER_KEY_FILE")
	if !cfg.AdmissionEnabled {
		if hasFile {
			return errors.New("registration reader requires enabled admission")
		}
		return nil
	}
	if cfg.WebDir == "" || cfg.NewAPISocket == "" || cfg.PublicOrigin == "" || cfg.WalletDSNFile == "" || !hasFile || !filepath.IsAbs(file) {
		return errors.New("admission requires complete portal and wallet configuration plus an absolute reader key file")
	}
	cfg.RegistrationReaderKeyFile = filepath.Clean(file)
	return nil
}

func readRegistrationReaderKey(path string) (string, error) {
	fail := errors.New("registration reader key file is invalid or unreadable")
	// Production is Linux. POSIX mode bits do not establish private Windows ACLs.
	// Local Windows acceptance uses an injected, synthetic in-memory key.
	if runtime.GOOS != "linux" || !filepath.IsAbs(path) {
		return "", fail
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 || before.Size() > 8192 {
		return "", fail
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fail
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || !after.Mode().IsRegular() || after.Mode().Perm()&0077 != 0 || after.Size() > 8192 {
		return "", fail
	}
	body, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(body) > 8192 {
		return "", fail
	}
	key := strings.TrimSpace(string(body))
	if !validReaderKey(key) {
		return "", fail
	}
	return key, nil
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
