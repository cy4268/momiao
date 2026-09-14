package poker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

const ControlLeaseDuration = 30 * time.Second
const ControlRenewInterval = 10 * time.Second
const controlIOTimeout = 2 * time.Second
const maxLiveConnections = 4096

// AuthSession is built by the trusted transport/platform adapter. The required
// validator checks this full tuple against the ONLINE authentication authority.
type AuthSession struct {
	UserID                        int64
	SessionIDHash                 string
	SessionVersion, SecurityEpoch uint64
}

func (AuthSession) String() string   { return "poker.AuthSession<redacted>" }
func (AuthSession) GoString() string { return "poker.AuthSession<redacted>" }

type ControlRef struct {
	UserID         int64  `json:"user_id,string"`
	SessionIDHash  string `json:"auth_session_hash"`
	SessionVersion uint64 `json:"session_version,string"`
	SecurityEpoch  uint64 `json:"security_epoch,string"`
	ConnectionID   string `json:"connection_id"`
	TableID        string `json:"table_id"`
	SessionID      string `json:"session_id"`
	RuntimeEpoch   uint64 `json:"runtime_epoch,string"`
	ControlEpoch   uint64 `json:"control_epoch,string"`
}

func (ControlRef) String() string   { return "poker.ControlRef<redacted>" }
func (ControlRef) GoString() string { return "poker.ControlRef<redacted>" }
func (r ControlRef) auth() AuthSession {
	return AuthSession{r.UserID, r.SessionIDHash, r.SessionVersion, r.SecurityEpoch}
}

type SocketControlView struct {
	ConnectionID string `json:"connection_id"`
	SessionID    string `json:"session_id,omitempty"`
	Mode         string `json:"mode"`
	ControlEpoch string `json:"control_epoch"`
}
type ControlResult struct {
	Receipt *Receipt          `json:"receipt,omitempty"`
	Control SocketControlView `json:"control"`
	Ref     ControlRef        `json:"-"`
}
type TakeOverCommand struct {
	RequestID, SessionID, TargetConnectionID string
	Auth                                     AuthSession
}
type ControlStore interface {
	ControlLoad(context.Context, string) (string, error)
	ControlAssign(context.Context, string, string, string) (bool, error)
	ControlRenew(context.Context, string, string) (bool, error)
	ControlRelease(context.Context, string, string) (bool, error)
}

func controlValue(ref ControlRef) string { b, _ := json.Marshal(ref); return string(b) }
func parseControl(value string) (ControlRef, error) {
	var r ControlRef
	if len(value) > 2048 || json.Unmarshal([]byte(value), &r) != nil || !validIdentity(r) || r.SessionID == "" || r.RuntimeEpoch == 0 || r.ControlEpoch == 0 {
		return r, ErrNoControl
	}
	if controlValue(r) != value {
		return r, ErrNoControl
	}
	return r, nil
}
func isHex(value string, n int) bool {
	if len(value) != n {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func validIdentity(r ControlRef) bool {
	return r.UserID > 0 && isHex(r.SessionIDHash, 64) && isHex(r.ConnectionID, 32) && r.SessionVersion > 0 && len(r.TableID) == 36
}
func sameAuth(a, b AuthSession) bool {
	return a.UserID == b.UserID && a.SessionIDHash == b.SessionIDHash && a.SessionVersion == b.SessionVersion && a.SecurityEpoch == b.SecurityEpoch
}
func sameConnection(a, b ControlRef) bool {
	return sameAuth(a.auth(), b.auth()) && a.ConnectionID == b.ConnectionID && a.TableID == b.TableID && a.SessionID == b.SessionID && a.RuntimeEpoch == b.RuntimeEpoch
}
func (s *Service) live(ref ControlRef, requireGrant bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.connections[ref.ConnectionID]
	return !s.closed && ok && sameConnection(current, ref) && (!requireGrant || current.ControlEpoch == ref.ControlEpoch && ref.ControlEpoch > 0)
}
func (s *Service) validateAuth(ctx context.Context, ref ControlRef) error {
	if !validIdentity(ref) || s.opts.ValidateSession == nil {
		return ErrNoControl
	}
	bounded, cancel := context.WithTimeout(ctx, controlIOTimeout)
	defer cancel()
	if err := s.opts.ValidateSession(bounded, ref.auth()); err != nil {
		return errors.Join(ErrNoControl, err)
	}
	if bounded.Err() != nil {
		return errors.Join(ErrNoControl, bounded.Err())
	}
	return nil
}
func (s *Service) loadOwner(ctx context.Context, session string) (string, error) {
	if s.opts.Controls == nil {
		return "", ErrNoControl
	}
	bounded, cancel := context.WithTimeout(ctx, controlIOTimeout)
	defer cancel()
	value, err := s.opts.Controls.ControlLoad(bounded, session)
	if err != nil {
		return "", errors.Join(ErrNoControl, err)
	}
	if bounded.Err() != nil {
		return "", errors.Join(ErrNoControl, bounded.Err())
	}
	if value != "" {
		if _, err = parseControl(value); err != nil {
			return "", err
		}
	}
	return value, nil
}
func readonlyControl(ref ControlRef) SocketControlView {
	return SocketControlView{ConnectionID: ref.ConnectionID, SessionID: ref.SessionID, Mode: "READ_ONLY", ControlEpoch: strconv.FormatUint(ref.ControlEpoch, 10)}
}
func (s *Service) notifyControl(ref ControlRef) {
	if s.opts.AfterControlChanged != nil {
		s.opts.AfterControlChanged(ref.TableID, ref.ConnectionID)
	}
}
func (s *Service) loseGrant(ref ControlRef) {
	s.mu.Lock()
	current, ok := s.connections[ref.ConnectionID]
	changed := ok && sameConnection(current, ref) && current.ControlEpoch > 0
	if changed {
		current.ControlEpoch = 0
		s.connections[ref.ConnectionID] = current
	}
	s.mu.Unlock()
	if changed {
		s.notifyControl(ref)
	}
}
func (s *Service) grant(ref ControlRef) bool {
	var changed []ControlRef
	s.mu.Lock()
	current, ok := s.connections[ref.ConnectionID]
	if s.closed || !ok || !sameConnection(current, ref) {
		s.mu.Unlock()
		return false
	}
	for id, other := range s.connections {
		if id != ref.ConnectionID && other.SessionID == ref.SessionID && other.ControlEpoch > 0 {
			other.ControlEpoch = 0
			s.connections[id] = other
			changed = append(changed, other)
		}
	}
	s.connections[ref.ConnectionID] = ref
	s.mu.Unlock()
	for _, r := range changed {
		s.notifyControl(r)
	}
	s.notifyControl(ref)
	return true
}

// Connected only accepts the transport's authenticated server-generated ID.
// Rebinding the SAME live socket after BuyIn is supported and idempotent. claim
// is derived from validated ControlIntent, not an untrusted message field.
func (s *Service) Connected(ctx context.Context, ref ControlRef, claim bool) (ControlResult, error) {
	result := ControlResult{Ref: ref, Control: readonlyControl(ref)}
	if err := s.AuthorizeTable(ctx, ref.TableID, ref.auth()); err != nil {
		return result, err
	}
	if err := s.validateAuth(ctx, ref); err != nil {
		return result, err
	}
	_, err := s.mutate(ctx, ref.TableID, func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		if err := s.authorizeTableRow(ctx, tx, t, ref.auth()); err != nil {
			return Receipt{}, err
		}
		if ref.RuntimeEpoch != 0 && ref.RuntimeEpoch != t.Epoch {
			return Receipt{}, ErrFenced
		}
		ref.RuntimeEpoch = t.Epoch
		seat, e := userSeat(ctx, tx, t.ID, ref.UserID)
		if e != nil && !errors.Is(e, ErrDenied) {
			return Receipt{}, e
		}
		if e == nil {
			if ref.SessionID != "" && ref.SessionID != seat.Session {
				return Receipt{}, ErrDenied
			}
			ref.SessionID = seat.Session
			ref.ControlEpoch = seat.Control
		}
		if e != nil {
			// The persisted owner may hold a read-only host connection even when
			// ordinary spectators are disabled. This does not grant a seat or lease.
			if !t.AllowSpectators && ref.UserID != t.Owner {
				return Receipt{}, ErrDenied
			}
			ref.SessionID = ""
			ref.ControlEpoch = 0
		}
		s.mu.Lock()
		old, exists := s.connections[ref.ConnectionID]
		if s.closed || exists && (!sameAuth(old.auth(), ref.auth()) || old.TableID != ref.TableID || s.claimEligible[ref.ConnectionID] != claim) || !exists && len(s.connections) >= maxLiveConnections {
			s.mu.Unlock()
			return Receipt{}, ErrDenied
		}
		// An observer's cached epoch is not a grant. Preserve only an exact prior one.
		stored := ref
		stored.ControlEpoch = 0
		if exists && sameConnection(old, ref) {
			stored.ControlEpoch = old.ControlEpoch
		}
		s.connections[ref.ConnectionID] = stored
		s.claimEligible[ref.ConnectionID] = claim
		s.mu.Unlock()
		result.Ref = ref
		result.Control = readonlyControl(ref)
		if ref.SessionID == "" || !claim {
			return Receipt{Status: "NOOP"}, nil
		}
		return s.claimControl(ctx, tx, t, seat, ref, false, "connected:"+ref.ConnectionID, &result)
	})
	return result, err
}

func (s *Service) TakeOver(ctx context.Context, c TakeOverCommand) (ControlResult, error) {
	s.mu.Lock()
	ref, ok := s.connections[c.TargetConnectionID]
	eligible := s.claimEligible[c.TargetConnectionID]
	s.mu.Unlock()
	result := ControlResult{Ref: ref, Control: readonlyControl(ref)}
	if !ok || !eligible || ref.SessionID != c.SessionID || !sameAuth(ref.auth(), c.Auth) || c.RequestID == "" {
		return result, ErrNoControl
	}
	if err := s.validateAuth(ctx, ref); err != nil {
		return result, err
	}
	if err := s.AuthorizeTable(ctx, ref.TableID, ref.auth()); err != nil {
		return result, err
	}
	r, err := s.mutate(ctx, ref.TableID, func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		if err := s.authorizeTableRow(ctx, tx, t, ref.auth()); err != nil {
			return Receipt{}, err
		}
		if !s.live(ref, false) || ref.RuntimeEpoch != t.Epoch {
			return Receipt{}, ErrNoControl
		}
		seat, e := userSeat(ctx, tx, t.ID, ref.UserID)
		if e != nil {
			return Receipt{}, e
		}
		if seat.Session != c.SessionID {
			return Receipt{}, ErrDenied
		}
		ref.ControlEpoch = seat.Control
		return s.claimControl(ctx, tx, t, seat, ref, true, c.RequestID, &result)
	})
	if err == nil && r.Status != "NOOP" {
		result.Receipt = &r
	}
	return result, err
}

// claimControl advances durable authority before attempting the ephemeral CAS.
// A confirmed EPOCH_ADVANCED receipt is deliberately not a live-control grant.
func (s *Service) claimControl(ctx context.Context, tx pgx.Tx, t *tableRow, seat seatRow, ref ControlRef, takeover bool, key string, result *ControlResult) (Receipt, error) {
	if s.opts.Controls == nil || !s.live(ref, false) {
		return Receipt{}, ErrNoControl
	}
	current, err := s.loadOwner(ctx, seat.Session)
	if err != nil {
		return Receipt{}, err
	}
	request := struct {
		Auth                AuthSession
		Connection, Session string
		Takeover            bool
	}{ref.auth(), ref.ConnectionID, seat.Session, takeover}
	prior, found, err := findReceipt(ctx, tx, ref.UserID, "poker.control.v1", key, request)
	if err != nil {
		return Receipt{}, err
	}
	if found {
		epoch, e := strconv.ParseUint(prior.ControlEpoch, 10, 64)
		if e != nil {
			return Receipt{}, ErrNoControl
		}
		result.Receipt = &prior
		if epoch != seat.Control {
			return prior, nil
		}
		ref.ControlEpoch = epoch
		s.confirmControl(ctx, ref, current, result)
		return prior, nil
	}
	if current != "" {
		owner, e := parseControl(current)
		if e != nil {
			return Receipt{}, e
		}
		if owner.RuntimeEpoch > t.Epoch || owner.ControlEpoch > seat.Control {
			return Receipt{}, ErrFenced
		}
		if !takeover && sameConnection(owner, ref) && owner.ControlEpoch == seat.Control && s.live(ref, false) {
			ref.ControlEpoch = seat.Control
			if err = s.validateAuth(ctx, ref); err != nil {
				return Receipt{}, err
			}
			if s.grant(ref) {
				result.Ref = ref
				result.Control = readonlyControl(ref)
				result.Control.Mode = "CONTROLLER"
			}
			return Receipt{Status: "NOOP"}, nil
		}
		if !takeover && owner.RuntimeEpoch == t.Epoch && owner.ControlEpoch == seat.Control {
			return Receipt{Status: "NOOP"}, nil
		}
	}
	if err = s.validateAuth(ctx, ref); err != nil {
		return Receipt{}, err
	}
	ref.ControlEpoch = seat.Control + 1
	if _, err = tx.Exec(ctx, "UPDATE poker.sessions SET control_epoch=$2 WHERE session_id=$1", seat.Session, ref.ControlEpoch); err != nil {
		return Receipt{}, err
	}
	_, hash := hashes(key, request)
	identityHash := sha256.Sum256([]byte(controlValue(ref)))
	details, _ := json.Marshal(struct {
		Epoch    string `json:"control_epoch"`
		Identity string `json:"connection_identity_hash"`
	}{strconv.FormatUint(ref.ControlEpoch, 10), hex.EncodeToString(identityHash[:])})
	if _, err = tx.Exec(ctx, `INSERT INTO poker.audit_events(event_id,table_id,session_id,actor_user_id,actor_kind,event_type,request_hash,details) VALUES($1,$2,$3,$4,'USER','POKER_CONTROL_TAKEN_OVER',$5,$6)`, uuid(), t.ID, seat.Session, ref.UserID, hash[:], details); err != nil {
		return Receipt{}, err
	}
	if err = s.connection(ctx, tx, t, seat, true); err != nil {
		return Receipt{}, err
	}
	r := Receipt{TableID: t.ID, SessionID: seat.Session, Status: "CONTROL_EPOCH_ADVANCED", Version: t.Version + 1, ControlEpoch: strconv.FormatUint(ref.ControlEpoch, 10)}
	if err = saveReceipt(ctx, tx, ref.UserID, "poker.control.v1", key, request, r); err != nil {
		return Receipt{}, err
	}
	t.beforeCommit = func(ctx context.Context, _ *Receipt) error {
		if !s.live(ref, false) {
			return ErrNoControl
		}
		if err := s.authorizeTableRow(ctx, tx, t, ref.auth()); err != nil {
			return err
		}
		return s.validateAuth(ctx, ref)
	}
	t.onCommit = func(ctx context.Context) { result.Receipt = &r; s.confirmControl(ctx, ref, current, result) }
	return r, nil
}
func (s *Service) confirmControl(ctx context.Context, ref ControlRef, expected string, result *ControlResult) {
	result.Ref = ref
	result.Control = readonlyControl(ref)
	if !s.live(ref, false) || s.validateAuth(ctx, ref) != nil || s.AuthorizeTable(ctx, ref.TableID, ref.auth()) != nil {
		return
	}
	bounded, cancel := context.WithTimeout(ctx, controlIOTimeout)
	defer cancel()
	ok, err := s.opts.Controls.ControlAssign(bounded, ref.SessionID, expected, controlValue(ref))
	if err != nil || !ok || bounded.Err() != nil {
		return
	}
	// A newer actor can fence us after COMMIT. Never grant from a stale CAS.
	var valid bool
	err = s.opts.Pool.QueryRow(bounded, `SELECT EXISTS(SELECT 1 FROM poker.sessions s JOIN poker.tables t USING(table_id) WHERE s.session_id=$1 AND s.newapi_user_id=$2 AND s.state='ACTIVE' AND s.control_epoch=$3 AND t.runtime_epoch=$4 AND t.lifecycle_state<>'CLOSED')`, ref.SessionID, ref.UserID, ref.ControlEpoch, ref.RuntimeEpoch).Scan(&valid)
	if err != nil || !valid || s.validateAuth(bounded, ref) != nil || !s.live(ref, false) {
		return
	}
	if s.grant(ref) {
		result.Control.Mode = "CONTROLLER"
	}
}

func (s *Service) validateControl(ctx context.Context, tx pgx.Tx, t *tableRow, seat seatRow, ref *ControlRef) error {
	if ref == nil || ref.UserID != seat.User || ref.SessionID != seat.Session || ref.TableID != t.ID || ref.RuntimeEpoch != t.Epoch || ref.ControlEpoch != seat.Control || !s.live(*ref, true) {
		return ErrNoControl
	}
	if err := s.validateAuth(ctx, *ref); err != nil {
		s.loseGrant(*ref)
		return err
	}
	if err := s.authorizeTableRow(ctx, tx, t, ref.auth()); err != nil {
		s.loseGrant(*ref)
		return err
	}
	owner, err := s.loadOwner(ctx, ref.SessionID)
	if err != nil || owner != controlValue(*ref) {
		s.loseGrant(*ref)
		if err != nil {
			return err
		}
		return ErrNoControl
	}
	return nil
}
func (s *Service) guardMutation(ctx context.Context, tx pgx.Tx, t *tableRow, seat seatRow, ref *ControlRef, tableVersion, handVersion uint64, handID string) error {
	if err := s.validateControl(ctx, tx, t, seat, ref); err != nil {
		return err
	}
	if tableVersion == 0 || tableVersion != t.Version || handID != t.HandID {
		return ErrStaleVersion
	}
	if handID != "" {
		var version uint64
		if err := tx.QueryRow(ctx, "SELECT hand_version FROM poker.hands WHERE hand_id=$1", handID).Scan(&version); err != nil {
			return err
		}
		if version != handVersion {
			return ErrStaleVersion
		}
	} else if handVersion != 0 {
		return ErrStaleVersion
	}
	t.beforeCommit = func(ctx context.Context, _ *Receipt) error { return s.validateControl(ctx, tx, t, seat, ref) }
	return nil
}
func (s *Service) AuthorizeControl(ctx context.Context, ref ControlRef) error {
	tx, err := s.opts.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer rollback(tx)
	t, err := loadTable(ctx, tx, ref.TableID, false)
	if err != nil {
		return err
	}
	if t.NeedsReview {
		return ErrNeedsReview
	}
	seat, err := userSeat(ctx, tx, t.ID, ref.UserID)
	if err != nil {
		return err
	}
	return s.validateControl(ctx, tx, &t, seat, &ref)
}
func (s *Service) RenewControl(ctx context.Context, ref ControlRef) (SocketControlView, error) {
	view := readonlyControl(ref)
	bounded, cancel := context.WithTimeout(ctx, controlIOTimeout)
	defer cancel()
	if err := s.AuthorizeControl(bounded, ref); err != nil {
		s.loseGrant(ref)
		return view, err
	}
	ok, err := s.opts.Controls.ControlRenew(bounded, ref.SessionID, controlValue(ref))
	if err != nil || !ok || bounded.Err() != nil {
		s.loseGrant(ref)
		return view, ErrNoControl
	}
	view.Mode = "CONTROLLER"
	return view, nil
}
func (s *Service) renewTable(ctx context.Context, table string) {
	s.mu.Lock()
	var refs []ControlRef
	for _, r := range s.connections {
		if r.TableID == table && r.ControlEpoch > 0 {
			refs = append(refs, r)
		}
	}
	s.mu.Unlock()
	for _, r := range refs {
		if ctx.Err() != nil {
			return
		}
		_, _ = s.RenewControl(ctx, r)
	}
}
func (s *Service) Disconnected(ctx context.Context, ref ControlRef) error {
	resolved, e := s.connectionRef(ref)
	if e != nil {
		return nil
	}
	ref = resolved
	s.mu.Lock()
	current, ok := s.connections[ref.ConnectionID]
	if !ok || !sameConnection(current, ref) {
		s.mu.Unlock()
		return nil
	}
	delete(s.connections, ref.ConnectionID)
	delete(s.claimEligible, ref.ConnectionID)
	s.mu.Unlock()
	s.notifyControl(ref)
	if current.SessionID == "" {
		return nil
	}
	ref = current
	_, err := s.mutate(ctx, ref.TableID, func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		seat, e := userSeat(ctx, tx, t.ID, ref.UserID)
		if e != nil {
			return Receipt{}, e
		}
		if t.Epoch != ref.RuntimeEpoch || seat.Session != ref.SessionID {
			return Receipt{Status: "NOOP"}, nil
		}
		value, e := s.loadOwner(ctx, ref.SessionID)
		if e != nil {
			return Receipt{}, e
		}
		if value == "" {
			return Receipt{Status: "NOOP"}, nil
		}
		owner, e := parseControl(value)
		if e != nil {
			return Receipt{}, e
		}
		if !sameConnection(owner, ref) || owner.ControlEpoch != seat.Control {
			return Receipt{Status: "NOOP"}, nil
		}
		ref = owner
		if e = s.connection(ctx, tx, t, seat, false); e != nil {
			return Receipt{}, e
		}
		t.onCommit = func(ctx context.Context) {
			bounded, cancel := context.WithTimeout(ctx, controlIOTimeout)
			defer cancel()
			_, _ = s.opts.Controls.ControlRelease(bounded, ref.SessionID, controlValue(ref))
		}
		return Receipt{TableID: t.ID, SessionID: seat.Session, Status: "DISCONNECTED"}, nil
	})
	if err == nil || !errors.Is(err, ErrDenied) && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	terminal, terminalErr := s.detachedSettledSession(ctx, ref)
	if terminalErr != nil {
		return errors.Join(err, terminalErr)
	}
	if !terminal {
		return err
	}
	return s.releaseDetachedControl(ctx, ref)
}

func (s *Service) detachedSettledSession(ctx context.Context, ref ControlRef) (bool, error) {
	var terminal bool
	err := s.opts.Pool.QueryRow(ctx, `SELECT s.state='SETTLED' AND s.current_stack_units=0
	 AND s.final_cashout_units IS NOT NULL AND s.ended_at IS NOT NULL AND NOT EXISTS(
	 SELECT 1 FROM poker.seats seat WHERE seat.table_id=s.table_id AND seat.session_id=s.session_id)
	 FROM poker.sessions s WHERE s.session_id=$1 AND s.newapi_user_id=$2 AND s.table_id=$3`, ref.SessionID, ref.UserID, ref.TableID).Scan(&terminal)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return terminal, err
}

func (s *Service) releaseDetachedControl(ctx context.Context, ref ControlRef) error {
	if s.opts.Controls == nil {
		return ErrNoControl
	}
	owner, err := s.loadOwner(ctx, ref.SessionID)
	if err != nil || owner == "" {
		return err
	}
	current, err := parseControl(owner)
	if err != nil {
		return err
	}
	if !sameConnection(current, ref) || ref.ControlEpoch > 0 && current.ControlEpoch != ref.ControlEpoch {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, controlIOTimeout)
	defer cancel()
	released, err := s.opts.Controls.ControlRelease(bounded, ref.SessionID, owner)
	if err != nil || bounded.Err() != nil {
		return errors.Join(ErrNoControl, err, bounded.Err())
	}
	if released {
		return nil
	}
	owner, err = s.loadOwner(ctx, ref.SessionID)
	if err != nil || owner == "" {
		return err
	}
	current, err = parseControl(owner)
	if err != nil {
		return err
	}
	if !sameConnection(current, ref) || ref.ControlEpoch > 0 && current.ControlEpoch != ref.ControlEpoch {
		return nil
	}
	return ErrNoControl
}

// RevokeSessionControls drops ephemeral authority for one revoked Native SID.
// The caller must revoke the Native session first: every new connection and
// command still passes ValidateSession. Seats, hands and funds are untouched.
func (s *Service) RevokeSessionControls(ctx context.Context, user int64, nativeHash string) error {
	if s == nil || ctx == nil || user <= 0 || !isHex(nativeHash, 64) || s.opts.Controls == nil {
		return ErrNoControl
	}
	s.mu.Lock()
	var refs []ControlRef
	for _, ref := range s.connections {
		if ref.UserID == user && ref.SessionIDHash == nativeHash {
			refs = append(refs, ref)
		}
	}
	s.mu.Unlock()
	var result error
	sessions := map[string]struct{}{}
	for _, ref := range refs {
		result = errors.Join(result, s.Disconnected(ctx, ref))
		if ref.SessionID != "" {
			sessions[ref.SessionID] = struct{}{}
		}
	}
	for session := range sessions {
		value, err := s.loadOwner(ctx, session)
		if err == nil && value != "" {
			var owner ControlRef
			owner, err = parseControl(value)
			if err == nil && owner.UserID == user && owner.SessionIDHash == nativeHash {
				// Disconnected's post-commit release may fail or still be in
				// flight. Do not report a confirmed revocation in that case.
				err = ErrNoControl
			}
		}
		result = errors.Join(result, err)
	}
	return errors.Join(result, ctx.Err())
}

// PG connected alone is not ready after an epoch commit whose Redis CAS failed.
// This is read-only eligibility; frozen COMMITTED hands always retain their deck
// and participants even if a later lease or connection disappears.
func (s *Service) readySeat(ctx context.Context, tx credentialQuerier, t *tableRow, seat seatRow) (bool, error) {
	if !seat.Connected || seat.Session == "" || s.opts.Controls == nil {
		return false, nil
	}
	value, err := s.loadOwner(ctx, seat.Session)
	if err != nil {
		return false, err
	}
	if value == "" {
		return false, nil
	}
	ref, err := parseControl(value)
	if err != nil {
		return false, err
	}
	if ref.UserID != seat.User || ref.TableID != t.ID || ref.SessionID != seat.Session || ref.RuntimeEpoch != t.Epoch || ref.ControlEpoch != seat.Control || !s.live(ref, true) {
		return false, nil
	}
	if err = s.validateAuth(ctx, ref); err != nil {
		s.loseGrant(ref)
		return false, err
	}
	if err = s.authorizeTableRow(ctx, tx, t, ref.auth()); err != nil {
		if errors.Is(err, ErrTableAccessRequired) {
			s.loseGrant(ref)
			return false, nil
		}
		return false, err
	}
	return true, nil
}
func (s *Service) readySeats(ctx context.Context, tx credentialQuerier, t *tableRow, seats []seatRow) ([]seatRow, error) {
	bounded, cancel := context.WithTimeout(ctx, controlIOTimeout)
	defer cancel()
	ready := append([]seatRow{}, seats...)
	for i := range ready {
		if ready[i].State != "ACTIVE" && ready[i].State != "WAITING_BIG_BLIND" {
			continue
		}
		ok, err := s.readySeat(bounded, tx, t, ready[i])
		if err != nil {
			return nil, err
		}
		ready[i].Connected = ok
	}
	return ready, nil
}
