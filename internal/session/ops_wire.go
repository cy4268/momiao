package session

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

var (
	wireName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,127}$`)
	wireUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type opsContext struct {
	Version              string `json:"version"`
	OperationID          string `json:"operation_id"`
	ActorUserID          string `json:"actor_user_id"`
	NativeSIDHash        string `json:"native_sid_hash"`
	NativeSessionVersion string `json:"native_session_version"`
	NativeAuthVersion    string `json:"native_auth_version"`
	PlatformSessionHash  string `json:"platform_session_hash"`
	SecurityEpoch        string `json:"security_epoch"`
	AuthzEpoch           string `json:"authz_epoch"`
	Action               string `json:"action"`
	TargetKind           string `json:"target_kind"`
	TargetID             string `json:"target_id"`
	ExpectedVersion      string `json:"expected_version"`
	Environment          string `json:"environment"`
	CommandDigest        string `json:"command_digest"`
	ImpactDigest         string `json:"impact_digest"`
	NonceHash            string `json:"nonce_hash"`
}
type receipt struct {
	ProofID              string `json:"proof_id"`
	Purpose              string `json:"purpose"`
	ActorUserID          string `json:"actor_user_id"`
	NativeSIDHash        string `json:"native_sid_hash"`
	NativeSessionVersion string `json:"native_session_version"`
	NativeAuthVersion    string `json:"native_auth_version"`
	ContextHash          string `json:"context_hash"`
	AuthenticatedAt      string `json:"authenticated_at"`
	ExpiresAt            string `json:"expires_at"`
	Method               string `json:"method"`
}

func canonicalUUID(v string) bool { return wireUUID.MatchString(v) }
func decimal(v string, zero bool) bool {
	n, err := strconv.ParseInt(v, 10, 64)
	return err == nil && (n > 0 || zero && n == 0) && strconv.FormatInt(n, 10) == v
}
func digest(v string) bool {
	b, err := hex.DecodeString(v)
	return err == nil && len(b) == 32 && hex.EncodeToString(b) == v
}
func hexDigest(v [32]byte) string { return hex.EncodeToString(v[:]) }
func hashHexBytes(v []byte) string {
	sum := sha256.Sum256(v)
	return hex.EncodeToString(sum[:])
}
func (c opsContext) hash() string { raw, _ := json.Marshal(c); return hashHexBytes(raw) }
func (c opsContext) valid() bool {
	if c.Version != "1" || !canonicalUUID(c.OperationID) || !decimal(c.SecurityEpoch, true) ||
		!decimal(c.ExpectedVersion, c.Action == "maintenance.create") {
		return false
	}
	for _, v := range []string{c.ActorUserID, c.NativeSessionVersion, c.NativeAuthVersion, c.AuthzEpoch} {
		if !decimal(v, false) {
			return false
		}
	}
	for _, v := range []string{c.NativeSIDHash, c.PlatformSessionHash, c.CommandDigest, c.ImpactDigest, c.NonceHash} {
		if !digest(v) {
			return false
		}
	}
	for _, v := range []string{c.Action, c.TargetKind, c.TargetID, c.Environment} {
		if !wireName.MatchString(v) {
			return false
		}
	}
	return true
}
func (r receipt) valid(p pending, now int64) bool {
	c := p.Context
	at, expiry := milliseconds(r.AuthenticatedAt)*1000, milliseconds(r.ExpiresAt)*1000
	return r.Purpose == "OPS_FRESH" && decimal(r.ProofID, false) && r.ActorUserID == c.ActorUserID &&
		r.NativeSIDHash == c.NativeSIDHash && r.NativeSessionVersion == c.NativeSessionVersion &&
		r.NativeAuthVersion == c.NativeAuthVersion && r.ContextHash == p.Hash &&
		(r.Method == "password" || r.Method == "discord") && at > 0 && at <= now && now-at <= 600000 &&
		expiry > now && expiry <= at+600000 && expiry <= milliseconds(p.Expires)
}
func (s *Service) rpc(ctx context.Context, c opsContext, sid, proof string, out any) error {
	if !c.valid() || !canonicalUUID(sid) || len(proof) > 128 {
		return inputFault
	}
	raw, _ := json.Marshal(c)
	request := struct {
		Context   string `json:"context"`
		NativeSID string `json:"native_sid"`
		Proof     string `json:"proof,omitempty"`
	}{string(raw), sid, proof}
	body, _ := json.Marshal(request)
	if len(raw) > 3072 || len(body) > 4096 {
		return inputFault
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	path := "challenge"
	if proof != "" {
		path = "consume"
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/internal/momiao/ops/fresh/"+path, bytes.NewReader(body))
	req.Host = "localhost"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.options.OpsKey)
	response, err := s.ops.RoundTrip(req)
	if err != nil {
		return unavailableFault
	}
	defer response.Body.Close()
	if response.StatusCode == 401 {
		return proofFault
	}
	if response.StatusCode == 409 {
		return consumedFault
	}
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != 200 || err != nil || media != "application/json" {
		return unavailableFault
	}
	reply, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || ctx.Err() != nil || len(reply) > 4096 || !strictJSON(reply, out) {
		return unavailableFault
	}
	return nil
}
