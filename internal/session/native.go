package session

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/poker/authbridge"
	"github.com/cy4268/momiao/internal/poker/connectticket"
)

func New(o Options) (*Service, error) {
	if !o.valid() {
		return nil, configFault
	}
	block, _ := aes.NewCipher(o.SessionSealKey[:]) // The fixed array is always a valid AES-256 key.
	seal, _ := cipher.NewGCM(block)
	s := &Service{options: o, native: nativeself.NewTransport(o.NativeSocket), ops: nativeself.NewTransport(o.OpsSocket), seal: seal}
	s.native.ResponseHeaderTimeout, s.ops.ResponseHeaderTimeout = 2*time.Second, 2*time.Second
	s.reader, _ = authbridge.NewNativeReader(s.native, o.NativeReaderKey)
	s.authority, _ = authbridge.New(o.Pool, s.reader.Check)
	return s, nil
}
func (s *Service) Close() {
	if s != nil && s.native != nil {
		s.native.CloseIdleConnections()
	}
	if s != nil && s.ops != nil {
		s.ops.CloseIdleConnections()
	}
}

type selfObservation struct {
	transport http.RoundTripper
	status    int
	failed    bool
}

type selfBodyObservation struct {
	io.ReadCloser
	state *selfObservation
}

func (b selfBodyObservation) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.state.failed = b.state.failed || err != nil && err != io.EOF
	return n, err
}

func (o *selfObservation) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := o.transport.RoundTrip(r)
	o.status = -1
	if response != nil {
		o.status = response.StatusCode
		response.Body = selfBodyObservation{response.Body, o}
	}
	return response, err
}
func (s *Service) AdoptNative(ctx context.Context, c NativeCredential) (Grant, error) {
	if s == nil || s.authority == nil || ctx == nil || !decimal(c.UserID, false) || !canonicalUUID(c.SID) {
		return Grant{}, inputFault
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	id, _ := strconv.ParseInt(c.UserID, 10, 64)
	observation := &selfObservation{transport: s.native}
	check, done := context.WithTimeout(ctx, 2*time.Second)
	err := nativeself.Verify(check, observation, id, c.Username, c.AccessToken, c.SID)
	done()
	if err != nil {
		if observation.failed || observation.status < 0 || observation.status >= 500 {
			return Grant{}, unavailableFault
		}
		return Grant{}, Fault{401, "SESSION_UNAUTHORIZED", false}
	}
	payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(c.AccessToken, ".")[1])
	var claims struct{ SV, UV uint64 }
	if json.Unmarshal(payload, &claims) != nil {
		return Grant{}, authFault
	}
	ref := authbridge.NativeRef{UserID: c.UserID, SessionIDHash: hashHexBytes([]byte(c.SID)), SessionVersion: claims.SV}
	live, err := s.reader.Check(ctx, ref)
	if err != nil {
		return Grant{}, nativeFault(err)
	}
	if live.UserAuthVersion != claims.UV {
		return Grant{}, authFault
	}
	bound, err := s.authority.Bind(ctx, ref)
	if err != nil {
		return Grant{}, nativeFault(err)
	}
	if err = s.authority.Check(ctx, bound); err != nil {
		return Grant{}, nativeFault(err)
	}
	now := time.Now().UnixMilli()
	absolute := min(live.ExpiresAt.UnixMilli(), live.CreatedAt.Add(s.options.Lifetime.Absolute).UnixMilli())
	r := record{
		Schema: 1, State: "AUTHENTICATED", UserID: c.UserID, Created: ms(now), ChainStart: ms(live.CreatedAt.UnixMilli()),
		LastSeen: ms(now), Idle: ms(min(now+s.options.Lifetime.Idle.Milliseconds(), absolute)), Absolute: ms(absolute),
		LoginMethod: "NATIVE_SESSION", Authenticated: ms(live.CreatedAt.UnixMilli()), SecurityEpoch: strconv.FormatUint(bound.SecurityEpoch, 10),
		AccountStatus: "ACTIVE", Checked: ms(now), Version: "1", NativeHash: ref.SessionIDHash,
		NativeSV: strconv.FormatUint(ref.SessionVersion, 10), NativeUV: strconv.FormatUint(live.UserAuthVersion, 10),
		NativeCreated: ms(live.CreatedAt.UnixMilli()), NativeExpires: ms(live.ExpiresAt.UnixMilli()),
	}
	sid := secret()
	r.Hash, r.CSRF = secretHash(sid), secret()
	r.NativeBox = s.sealSID(c.SID, r)
	r, err = s.put(ctx, nil, r, milliseconds(r.Idle), 0)
	if err != nil {
		return Grant{}, err
	}
	return Grant{s, r.view(), sid}, nil
}
func nativeFault(err error) Fault {
	if errors.Is(err, connectticket.ErrRevoked) {
		return authFault
	}
	return unavailableFault
}
func (s *Service) check(ctx context.Context, r record) error {
	sv, _ := strconv.ParseUint(r.NativeSV, 10, 64)
	ref := authbridge.NativeRef{UserID: r.UserID, SessionIDHash: r.NativeHash, SessionVersion: sv}
	live, err := s.reader.Check(ctx, ref)
	if err == nil && (strconv.FormatUint(live.UserAuthVersion, 10) != r.NativeUV ||
		ms(live.CreatedAt.UnixMilli()) != r.NativeCreated || ms(live.ExpiresAt.UnixMilli()) != r.NativeExpires) {
		err = connectticket.ErrRevoked
	}
	if err == nil {
		epoch, _ := strconv.ParseUint(r.SecurityEpoch, 10, 64)
		err = s.authority.Check(ctx, connectticket.Session{UserID: r.UserID, SessionIDHash: r.NativeHash, SessionVersion: sv, SecurityEpoch: epoch})
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, connectticket.ErrRevoked) {
		_ = s.drop(ctx, sessionPrefix+r.Hash)
	}
	return nativeFault(err)
}
func (s *Service) current(ctx context.Context, h RequestSession) (record, error) {
	if s == nil || ctx == nil || h.owner != s || !h.unsafe || !digest(h.hash) {
		return record{}, inputFault
	}
	r, err := s.load(ctx, h.hash)
	if err != nil {
		return r, err
	}
	if !sameSecret(r.CSRF, h.view.CSRFToken) {
		return record{}, csrfFault
	}
	return r, s.check(ctx, r)
}
