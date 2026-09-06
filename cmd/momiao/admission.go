package main

import (
	"context"
	"github.com/cy4268/momiao/internal/platform"
	"mime"
	"net/http"
	"strings"
	"time"
)

type admissionStore interface {
	EnsureProvisionalProfile(context.Context, int64) (platform.Profile, error)
	ReadAdmission(context.Context, int64) (platform.AdmissionStatus, error)
}

func newAdmissionHandler(origin string, store admissionStore, transport http.RoundTripper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			walletError(w, 503, "ADMISSION_UNAVAILABLE")
			return
		}
		ensure := r.URL.Path == "/platform/v1/admission/ensure"
		if !ensure && r.URL.Path != "/platform/v1/admission" {
			walletError(w, 404, "NOT_FOUND")
			return
		}
		method := http.MethodGet
		if ensure {
			method = http.MethodPost
		}
		if r.Method != method {
			w.Header().Set("Allow", method)
			walletError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			walletError(w, 400, "INVALID_REQUEST")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		user, status := verifyWalletUser(r, transport)
		if status != 0 {
			code := "AUTH_UNAVAILABLE"
			if status == 401 {
				code = "AUTH_UNAUTHORIZED"
			}
			if status == 403 {
				code = "AUTH_FORBIDDEN"
			}
			walletError(w, status, code)
			return
		}
		if ensure {
			if origins := r.Header.Values("Origin"); len(origins) != 1 || origins[0] != origin {
				walletError(w, 403, "ORIGIN_REJECTED")
				return
			}
			ct, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || ct != "application/json" || len(r.Header.Values("Content-Type")) != 1 || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
				walletError(w, 415, "INVALID_CONTENT_TYPE")
				return
			}
			if _, err = decodeStringFields(r.Body); err != nil {
				walletError(w, 400, "INVALID_REQUEST")
				return
			}
			p, err := store.EnsureProvisionalProfile(ctx, user)
			if err != nil {
				walletError(w, 503, "ADMISSION_UNAVAILABLE")
				return
			}
			walletSuccess(w, p)
			return
		}
		data, err := store.ReadAdmission(ctx, user)
		if err != nil {
			walletError(w, 503, "ADMISSION_UNAVAILABLE")
			return
		}
		walletSuccess(w, data)
	})
}
