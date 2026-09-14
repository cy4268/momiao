package session

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type pending struct {
	Context  opsContext `json:"context"`
	Hash     string     `json:"context_hash"`
	Phase    string     `json:"phase"`
	Deadline string     `json:"phase_deadline"`
	Expires  string     `json:"native_expires_at"`
}

func (r record) validPending() bool {
	p := r.Pending
	if p == nil {
		return true
	}
	c := p.Context
	return c.valid() && p.Hash == c.hash() && c.ActorUserID == r.UserID && c.NativeSIDHash == r.NativeHash &&
		c.NativeSessionVersion == r.NativeSV && c.NativeAuthVersion == r.NativeUV && c.SecurityEpoch == r.SecurityEpoch &&
		c.PlatformSessionHash == r.Hash && milliseconds(p.Deadline) > 0 && milliseconds(p.Deadline) <= milliseconds(r.Absolute) &&
		(p.Phase == "ISSUING" && p.Expires == "" || (p.Phase == "READY" || p.Phase == "EXCHANGING") && milliseconds(p.Expires) >= milliseconds(p.Deadline))
}
func (s *Service) principal(ctx context.Context, user string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var epoch, status string
	err := s.options.Pool.QueryRow(ctx, "SELECT authz_epoch::text,status FROM ops.admin_principals WHERE newapi_user_id=$1", user).Scan(&epoch, &status)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (status != "ACTIVE" || !decimal(epoch, false)) {
		return "", principalFault
	}
	if err != nil {
		return "", unavailableFault
	}
	return epoch, nil
}
func (s *Service) end(ctx context.Context, r record) {
	next := r
	next.Pending = nil
	_, _ = s.put(ctx, &r, next, milliseconds(r.Idle), 0)
}
func (s *Service) StartOps(ctx context.Context, h RequestSession, operation Operation) (Challenge, error) {
	if ctx == nil {
		return Challenge{}, inputFault
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	r, err := s.current(ctx, h)
	if err != nil {
		return Challenge{}, err
	}
	now := time.Now().UnixMilli()
	if r.Pending != nil && milliseconds(r.Pending.Deadline) > now {
		return Challenge{}, conflictFault
	}
	epoch, err := s.principal(ctx, r.UserID)
	if err != nil {
		return Challenge{}, err
	}
	id := secret()
	c := opsContext{"1", operation.ID, r.UserID, r.NativeHash, r.NativeSV, r.NativeUV, r.Hash, r.SecurityEpoch, epoch,
		operation.Action, operation.TargetKind, operation.TargetID, operation.ExpectedVersion, s.options.Environment,
		hexDigest(operation.CommandDigest), hexDigest(operation.ImpactDigest), secretHash(id)}
	if !c.valid() {
		return Challenge{}, inputFault
	}
	old := r
	r.Pending = &pending{Context: c, Hash: c.hash(), Phase: "ISSUING", Deadline: ms(min(now+8000, milliseconds(r.Idle)))}
	r, err = s.put(ctx, &old, r, milliseconds(r.Idle), 0)
	if err != nil {
		return Challenge{}, err
	}
	sid, _ := s.openSID(r)
	var reply struct {
		Token   string `json:"challenge_token"`
		Hash    string `json:"context_hash"`
		Expires string `json:"expires_at"`
	}
	if err = s.rpc(ctx, c, sid, "", &reply); err != nil {
		s.end(ctx, r)
		return Challenge{}, err
	}
	expires := milliseconds(reply.Expires) * 1000
	now = time.Now().UnixMilli()
	if reply.Hash != c.hash() || !opaque(reply.Token) || expires <= now || expires > now+600000 {
		s.end(ctx, r)
		return Challenge{}, unavailableFault
	}
	if err = s.check(ctx, r); err != nil {
		s.end(ctx, r)
		return Challenge{}, err
	}
	currentEpoch, err := s.principal(ctx, r.UserID)
	if err == nil && currentEpoch != epoch {
		err = principalFault
	}
	if err != nil {
		s.end(ctx, r)
		return Challenge{}, err
	}
	old = r
	r.Pending = &pending{Context: c, Hash: c.hash(), Phase: "READY", Deadline: ms(min(expires, milliseconds(r.Absolute))), Expires: ms(expires)}
	_, err = s.put(ctx, &old, r, milliseconds(old.Pending.Deadline), 0)
	if err != nil {
		return Challenge{}, err
	}
	return Challenge{id, reply.Token, time.UnixMilli(expires).UTC()}, nil
}

// CancelOps releases one unconsumed challenge owned by the current opaque session.
// The Native challenge may remain until its short expiry, but without the
// matching pending record it cannot be exchanged into a fresh session grant.
func (s *Service) CancelOps(ctx context.Context, h RequestSession, operationID, challengeID string) error {
	if ctx == nil || !canonicalUUID(operationID) || challengeID != "" && !opaque(challengeID) {
		return inputFault
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	r, err := s.current(ctx, h)
	if err != nil {
		return err
	}
	if r.Pending == nil {
		return nil
	}
	if r.Pending.Phase == "EXCHANGING" ||
		r.Pending.Context.OperationID != operationID ||
		challengeID != "" && r.Pending.Context.NonceHash != secretHash(challengeID) {
		return conflictFault
	}
	old := r
	r.Pending = nil
	_, err = s.put(ctx, &old, r, milliseconds(old.Idle), 0)
	return err
}

// CheckOpsChallenge revalidates the live session, authorization epoch and the
// exact READY challenge before a BFF sends an authentication factor upstream.
func (s *Service) CheckOpsChallenge(ctx context.Context, h RequestSession, operationID, challengeID string) error {
	if ctx == nil || !canonicalUUID(operationID) || !opaque(challengeID) {
		return inputFault
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	r, err := s.current(ctx, h)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if r.Pending == nil || r.Pending.Phase != "READY" || milliseconds(r.Pending.Deadline) <= now ||
		r.Pending.Context.OperationID != operationID || r.Pending.Context.NonceHash != secretHash(challengeID) {
		return conflictFault
	}
	epoch, err := s.principal(ctx, r.UserID)
	if err == nil && epoch != r.Pending.Context.AuthzEpoch {
		err = principalFault
	}
	return err
}

func (s *Service) CompleteOps(ctx context.Context, h RequestSession, id, proof string) (Grant, error) {
	if ctx == nil {
		return Grant{}, inputFault
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	r, err := s.current(ctx, h)
	if err != nil {
		return Grant{}, err
	}
	if !opaque(id) || !opaque(proof) {
		return Grant{}, proofFault
	}
	now := time.Now().UnixMilli()
	if r.Pending == nil || r.Pending.Phase != "READY" || r.Pending.Context.NonceHash != secretHash(id) || milliseconds(r.Pending.Deadline) <= now {
		return Grant{}, conflictFault
	}
	epoch, err := s.principal(ctx, r.UserID)
	if err == nil && epoch != r.Pending.Context.AuthzEpoch {
		err = principalFault
	}
	if err != nil {
		s.end(ctx, r)
		return Grant{}, err
	}
	old, p := r, *r.Pending
	p.Phase, p.Deadline = "EXCHANGING", ms(min(now+8000, milliseconds(p.Deadline), milliseconds(r.Idle)))
	r.Pending = &p
	r, err = s.put(ctx, &old, r, milliseconds(p.Deadline), 0)
	if err != nil {
		return Grant{}, err
	}
	sid, _ := s.openSID(r)
	var reply receipt
	if err = s.rpc(ctx, p.Context, sid, proof, &reply); err != nil {
		s.end(ctx, r)
		return Grant{}, err
	}
	if !reply.valid(p, time.Now().UnixMilli()) {
		s.end(ctx, r)
		return Grant{}, unavailableFault
	}
	if err = s.check(ctx, r); err != nil {
		s.end(ctx, r)
		return Grant{}, err
	}
	epoch, err = s.principal(ctx, r.UserID)
	if err == nil && epoch != p.Context.AuthzEpoch {
		err = principalFault
	}
	if err != nil {
		s.end(ctx, r)
		return Grant{}, err
	}
	old, nextSID := r, secret()
	now = time.Now().UnixMilli()
	at := milliseconds(reply.AuthenticatedAt) * 1000
	r.Hash, r.CSRF, r.Pending = secretHash(nextSID), secret(), nil
	r.Created, r.LastSeen, r.Checked, r.Fresh, r.FreshMethod = ms(now), ms(now), ms(now), new(ms(at)), reply.Method
	r.Idle = ms(min(now+s.options.Lifetime.Idle.Milliseconds(), milliseconds(r.Absolute)))
	version, _ := strconv.ParseUint(r.Version, 10, 64)
	r.Version = strconv.FormatUint(version+1, 10)
	r.Confirmation = &Confirmation{p.Context.OperationID, p.Hash, old.Hash}
	r.NativeBox = s.sealSID(sid, r)
	guard := min(milliseconds(p.Deadline), milliseconds(reply.ExpiresAt)*1000, milliseconds(old.Idle))
	r, err = s.put(ctx, &old, r, guard, at)
	if err != nil {
		return Grant{}, err
	}
	return Grant{s, r.view(), nextSID}, nil
}
