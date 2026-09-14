package authbridge

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/poker/connectticket"
)

// NativeReader uses the platform's fixed native Unix transport in production.
// It never forwards browser credentials, follows redirects or constructs URLs
// from request data. The dedicated reader key cannot authorize ordinary users.
type NativeReader struct {
	transport http.RoundTripper
	key       string
}

func NewNativeReader(transport http.RoundTripper, key string) (*NativeReader, error) {
	b, e := hex.DecodeString(key)
	if transport == nil || e != nil || len(b) != 32 || hex.EncodeToString(b) != key {
		return nil, connectticket.ErrConfig
	}
	return &NativeReader{transport, key}, nil
}
func (*NativeReader) String() string   { return "NativeReader([redacted])" }
func (*NativeReader) GoString() string { return "NativeReader([redacted])" }

func (r *NativeReader) Check(ctx context.Context, ref NativeRef) (NativeSession, error) {
	if r == nil || ctx == nil || !validRef(ref) {
		return NativeSession{}, connectticket.ErrInvalid
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request := struct {
		UserID  string `json:"user_id"`
		Hash    string `json:"session_id_hash"`
		Version string `json:"expected_session_version"`
	}{ref.UserID, ref.SessionIDHash, strconv.FormatUint(ref.SessionVersion, 10)}
	body, _ := json.Marshal(request)
	req, _ := http.NewRequestWithContext(c, http.MethodPost, "http://unix/internal/momiao/poker/session-check", bytes.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Authorization", "Bearer "+r.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, e := r.transport.RoundTrip(req)
	if e != nil {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	defer resp.Body.Close()
	media, _, e := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != 200 || e != nil || media != "application/json" {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 8193))
	if e != nil || len(raw) > 8192 || !utf8.Valid(raw) || c.Err() != nil {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if !nativeself.UniqueJSON(d, 0) {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	if _, e = d.Token(); e != io.EOF {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	var valid *bool
	if json.Unmarshal(fields["valid"], &valid) != nil || valid == nil {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	keys := []string{"user_id", "session_id_hash", "expected_session_version", "valid"}
	if *valid {
		keys = append(keys, "user_auth_version", "session_created_at", "session_expires_at")
	}
	if len(fields) != len(keys) {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			return NativeSession{}, connectticket.ErrUnavailable
		}
	}
	str := func(key string) string {
		var s string
		if json.Unmarshal(fields[key], &s) != nil {
			return ""
		}
		return s
	}
	if str("user_id") != ref.UserID || str("session_id_hash") != ref.SessionIDHash || str("expected_session_version") != request.Version {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	if !*valid {
		return NativeSession{}, connectticket.ErrRevoked
	}
	uv, ok := positive(str("user_auth_version"), maxSafe)
	if !ok {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	created, ok := positive(str("session_created_at"), 253402300799)
	if !ok {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	expires, ok := positive(str("session_expires_at"), 253402300799)
	if !ok || expires <= created {
		return NativeSession{}, connectticket.ErrUnavailable
	}
	n := NativeSession{Ref: ref, UserAuthVersion: uv, CreatedAt: time.Unix(int64(created), 0).UTC(), ExpiresAt: time.Unix(int64(expires), 0).UTC()}
	if n.CreatedAt.After(time.Now()) || !n.ExpiresAt.After(time.Now()) {
		return NativeSession{}, connectticket.ErrRevoked
	}
	return n, nil
}
func positive(raw string, max uint64) (uint64, bool) {
	n, e := strconv.ParseUint(raw, 10, 64)
	return n, e == nil && n > 0 && n <= max && strconv.FormatUint(n, 10) == raw
}
