package poker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewHandReadinessRequiresTableGrant(t *testing.T) {
	unavailable := errors.New("controlled grant store unavailable")
	for _, tc := range []struct {
		name    string
		granted bool
		failure error
	}{
		{"grant present", true, nil}, {"grant absent", false, nil}, {"grant unavailable", false, unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			table := &tableRow{ID: "00000000-0000-4000-8000-000000000001", AccessMode: "PASSWORD", Epoch: 1}
			ref := ControlRef{UserID: 1, SessionIDHash: strings.Repeat("a", 64), SessionVersion: 1, ConnectionID: strings.Repeat("b", 32), TableID: table.ID, SessionID: "session", RuntimeEpoch: 1, ControlEpoch: 1}
			grant := &finalFundingGrant{granted: tc.granted, err: tc.failure}
			s := &Service{connections: map[string]ControlRef{ref.ConnectionID: ref}, opts: Options{
				Password: runtimeForTest(t, runtimePolicy()), TableAccess: grant, PasswordLimits: PasswordLimits{1, time.Second, 1, time.Second},
				Controls:        &fixtureControls{values: map[string]string{ref.SessionID: controlValue(ref)}},
				ValidateSession: func(context.Context, AuthSession) error { return nil },
				SessionDeadline: func(context.Context, AuthSession) (time.Time, error) { return time.Now().Add(time.Hour), nil },
			}}
			seat := seatRow{Connected: true, Session: ref.SessionID, User: ref.UserID, Control: ref.ControlEpoch}
			ready, err := s.readySeat(context.Background(), &finalFundingTx{&fundingCallbackTx{}}, table, seat)
			if tc.failure != nil {
				if ready || !errors.Is(err, unavailable) {
					t.Fatal("grant infrastructure failure allowed a new hand")
				}
			} else if ready != tc.granted || err != nil {
				t.Fatal("readiness ignored current table grant")
			}
			if tc.failure == nil && !tc.granted && s.live(ref, true) {
				t.Fatal("lost grant retained live controller")
			}
		})
	}
}
