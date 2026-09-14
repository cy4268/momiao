package poker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type finalFundingTx struct{ *fundingCallbackTx }

func (t *finalFundingTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return finalFundingCredential{}
}

type finalFundingCredential struct{}

func (finalFundingCredential) Scan(values ...any) error {
	*values[0].(*string) = "PASSWORD"
	*values[1].(*string) = "controlled-immutable-phc"
	return nil
}

type finalFundingGrant struct {
	granted bool
	err     error
	calls   int
}

func (g *finalFundingGrant) AccessValid(context.Context, string, string, string, time.Time) (bool, error) {
	g.calls++
	return g.granted, g.err
}
func (*finalFundingGrant) AccessPut(context.Context, string, string, string, time.Time) (bool, error) {
	panic("unexpected grant write")
}
func (*finalFundingGrant) AccessAttempt(context.Context, int64, string, uint32, time.Duration, uint32, time.Duration) (bool, error) {
	panic("unexpected attempt")
}

// Fake transaction / grant boundary only; not a real wallet rollback proof.
func TestBuyInFinalAccessReceiptBoundary(t *testing.T) {
	unavailable := errors.New("controlled Redis unavailable")
	for _, tc := range []struct {
		name    string
		granted bool
		failure error
		status  string
	}{
		{"still granted", true, nil, "CONFIRMED"},
		{"grant lost", false, nil, "CONFIRMED"},
		{"infrastructure failure", false, unavailable, "CONFIRMED"},
		{"earlier definite failure", false, unavailable, "FAILED_NO_EFFECT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			grant := &finalFundingGrant{granted: tc.granted, err: tc.failure}
			s := &Service{opts: Options{Password: runtimeForTest(t, runtimePolicy()), TableAccess: grant,
				PasswordLimits:  PasswordLimits{1, time.Second, 1, time.Second},
				SessionDeadline: func(context.Context, AuthSession) (time.Time, error) { return time.Now().Add(time.Hour), nil },
			}}
			tx := &fundingCallbackTx{}
			effects := &finalFundingTx{&fundingCallbackTx{}}
			table := &tableRow{ID: "00000000-0000-4000-8000-000000000001", AccessMode: "PASSWORD", Version: 4, onCommit: func(context.Context) {}}
			command := BuyInCommand{UserID: 1, Key: "controlled-mutation-key", Auth: AuthSession{UserID: 1, SessionIDHash: strings.Repeat("a", 64), SessionVersion: 1}}
			receipt := Receipt{TableID: table.ID, FundingID: "funding", SessionID: "successful-session", Status: tc.status, Version: 5}
			err := s.finalizeBuyIn(context.Background(), tx, effects, table, command, &receipt)
			if tc.status != "CONFIRMED" {
				if err != nil || !effects.committed || grant.calls != 0 {
					t.Fatal("earlier failure was reauthorized or lost")
				}
				return
			}
			if grant.calls != 1 {
				t.Fatal("final grant not checked")
			}
			if tc.failure != nil {
				if !errors.Is(err, unavailable) || effects.committed || tx.writes != 0 {
					t.Fatal("infrastructure failure persisted a verdict")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.granted {
				if !effects.committed || effects.rolledBack || tx.writes != 0 || receipt.Status != "CONFIRMED" {
					t.Fatal("valid grant lost effects")
				}
			} else {
				if !effects.rolledBack || effects.committed || tx.writes != 2 || table.onCommit != nil || receipt.Status != "FAILED_NO_EFFECT" || receipt.FailureCode != "TABLE_ACCESS_REQUIRED" || receipt.SessionID != "" || receipt.FundingID != "funding" {
					t.Fatal("access loss leaked success effects/receipt or lease callback")
				}
			}
		})
	}
}
