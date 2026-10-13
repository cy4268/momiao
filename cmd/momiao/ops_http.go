package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/platform"
)

const opsAPIPath = "/api/v1/ops"

func opsAPIRoute(path string) bool {
	return path == opsAPIPath || strings.HasPrefix(path, opsAPIPath+"/")
}

func opsRequestSafe(r *http.Request) bool {
	if r == nil || r.URL == nil || r.URL.RawPath != "" || r.ContentLength > 65536 {
		return false
	}
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
		return false
	}
	for _, name := range []string{"Authorization", "New-Api-User", "X-Auth-Session", "X-Game-CSRF-Token", "Proxy-Authorization"} {
		if len(r.Header.Values(name)) != 0 {
			return false
		}
	}
	return true
}

func decodeOpsBody(r *http.Request, out any) bool {
	if r == nil || len(r.Header.Values("Content-Type")) != 1 {
		return false
	}
	media, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(parameters) != 0 {
		return false
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	if err != nil || len(raw) == 0 || len(raw) > 65536 || !utf8.Valid(raw) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if !nativeself.UniqueJSON(decoder, 0) {
		return false
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return false
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(out); err != nil {
		return false
	}
	return errors.Is(decoder.Decode(new(any)), io.EOF)
}

func opsEpoch(value string) (int64, bool) {
	epoch, err := strconv.ParseInt(value, 10, 64)
	return epoch, err == nil && epoch > 0 && strconv.FormatInt(epoch, 10) == value
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	walletError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
	return false
}

func opsUUIDPatternHTTP(value string) bool {
	if len(value) != 36 {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil && strings.ToLower(value) == value && value[8] == '-' && value[13] == '-' && value[18] == '-' && value[23] == '-'
}

func writeOpsError(w http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, "OPS_UNAVAILABLE"
	switch {
	case errors.Is(err, platform.ErrOpsInvalid):
		status, code = http.StatusBadRequest, err.Error()
	case errors.Is(err, platform.ErrOpsForbidden):
		status, code = http.StatusForbidden, err.Error()
	case errors.Is(err, platform.ErrOpsNotFound):
		status, code = http.StatusNotFound, err.Error()
	case errors.Is(err, platform.ErrOpsConflict), errors.Is(err, platform.ErrOpsLastSuperAdmin):
		status, code = http.StatusConflict, err.Error()
	case errors.Is(err, platform.ErrOpsUnavailable):
		status, code = http.StatusServiceUnavailable, err.Error()
	}
	walletError(w, status, code)
}
