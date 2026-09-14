package main

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker"
	"github.com/cy4268/momiao/internal/poker/engine"
	pt "github.com/cy4268/momiao/internal/poker/transport"
)

// Pure synthetic DTOs test the adapter boundary; these are not PG evidence.
func TestPokerApplicationSnapshotUsesReturnedAuthoritativeMetadata(t *testing.T) {
	view := poker.TableView{RuntimeEpoch: 8, TableID: "019a0000-0000-7000-8000-000000000001", TableVersion: "12", ServerNow: time.Now().UTC(), Hand: &poker.HandView{HandID: "019a0000-0000-7000-8000-000000000002", HandVersion: "15", BoardCards: []int{}}, Viewer: poker.ViewerView{SessionID: "019a0000-0000-7000-8000-000000000003", ControlEpoch: "3"}}
	view.Viewer.Control = &poker.SocketControlView{ConnectionID: strings.Repeat("a", 32), SessionID: view.Viewer.SessionID, Mode: "READ_ONLY", ControlEpoch: "3"}
	s, e := pokerTransportSnapshot(view)
	if e != nil || s.TableVersion != 12 || s.RuntimeEpoch != 8 || s.HandVersion == nil || *s.HandVersion != 15 || s.HandID == nil || *s.HandID != view.Hand.HandID || !s.ServerTime.Equal(view.ServerNow) || s.Control == nil || s.Control.ConnectionID != view.Viewer.Control.ConnectionID {
		t.Fatal("returned metadata mismatch", e)
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(s.Payload, &raw) != nil || string(raw["table_version"]) != `"12"` || raw["RuntimeEpoch"] != nil || raw["runtime_epoch"] != nil {
		t.Fatal("snapshot relabelled or leaked internal runtime")
	}
	view.Viewer.Control = nil
	s, e = pokerTransportSnapshot(view)
	if e != nil || s.Control != nil || strings.Contains(string(s.Payload), `"control":`) {
		t.Fatal("HTTP snapshot fabricated socket grant", e)
	}
	for _, bad := range []string{"", "01", "+1", "1.0", "-1", strconv.FormatUint(pt.MaxSafeInteger+1, 10), "18446744073709551616"} {
		view.TableVersion = bad
		if _, e = pokerTransportSnapshot(view); e == nil {
			t.Fatal("invalid envelope version accepted", bad)
		}
	}
}

func TestPokerApplicationControlMappingPreservesFullServerTuple(t *testing.T) {
	p := pt.Principal{UserID: 9007199254740993, SessionIDHash: strings.Repeat("b", 64), SessionVersion: 4, SecurityEpoch: 0, ControlIntent: "CLAIM_CONTROL"}
	c := pt.ConnectionRef{ID: strings.Repeat("c", 32), TableID: "019a0000-0000-7000-8000-000000000004"}
	identity := pokerIdentity(p, c)
	if identity.UserID != p.UserID || identity.SessionIDHash != p.SessionIDHash || identity.SessionVersion != 4 || identity.SecurityEpoch != 0 || identity.ConnectionID != c.ID || identity.TableID != c.TableID || identity.SessionID != "" || identity.RuntimeEpoch != 0 || identity.ControlEpoch != 0 {
		t.Fatal("identity changed or invented a grant")
	}
	full := pt.ControlRef{UserID: p.UserID, SessionIDHash: p.SessionIDHash, SessionVersion: 4, SecurityEpoch: 0, ConnectionID: c.ID, TableID: c.TableID, SessionID: "019a0000-0000-7000-8000-000000000005", RuntimeEpoch: 6, ControlEpoch: 7}
	if pokerTransportControl(pokerDomainControl(full)) != full {
		t.Fatal("full control reference lost fields")
	}
	hand := "019a0000-0000-7000-8000-000000000006"
	cmd := pokerSessionCommand(p, pt.SessionAction{ActionID: "action-000000001", TableID: c.TableID, ExpectedTableVersion: 9, ExpectedHandVersion: 10, ControlEpoch: 7, HandID: &hand, Control: full})
	if cmd.UserID != p.UserID || cmd.Key != "action-000000001" || cmd.TableID != c.TableID || cmd.ExpectedTableVersion != 9 || cmd.ExpectedHandVersion != 10 || cmd.HandID != hand || cmd.Control == nil || pokerTransportControl(*cmd.Control) != full {
		t.Fatal("WS command lost transaction guards")
	}
	observer := p
	observer.ControlIntent = "READ_ONLY"
	if !samePokerAuth(p, observer) {
		t.Fatal("HTTP auth intent incorrectly treated as credential mismatch")
	}
	observer.SessionVersion++
	if samePokerAuth(p, observer) {
		t.Fatal("different native session version matched")
	}
}

func TestPokerApplicationDomainErrorsAreStableAndRedacted(t *testing.T) {
	for _, entry := range []struct {
		err    error
		status int
		code   string
	}{
		{poker.ErrNoControl, 403, "POKER_CONTROL_NOT_OWNED"},
		{poker.ErrDenied, 403, "POKER_COMMAND_DENIED"},
		{poker.ErrStaleVersion, 409, "POKER_STALE_VERSION"},
		{poker.ErrConflict, 409, "POKER_REQUEST_CONFLICT"},
		{engine.ErrIllegal, 409, "POKER_ACTION_ILLEGAL"},
		{errors.New("secret SQL password"), 503, "POKER_SERVICE_UNAVAILABLE"},
	} {
		var fault *pt.Fault
		if !errors.As(pokerDomainError(entry.err), &fault) || fault.Status != entry.status || fault.Code != entry.code || strings.Contains(fault.Error(), "secret") {
			t.Fatal("domain error mapping")
		}
	}
}
