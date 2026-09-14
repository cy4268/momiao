package poker

import (
	"context"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
)

func TestV06CashoutThenRevokeReleasesOnlyItsController(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		loseGrantBeforeRevoke bool
		readOnlyConnections   int
	}{
		{name: "original-grant"},
		{name: "local-control-zero-after-renew-denial", loseGrantBeforeRevoke: true},
		{name: "live-read-only-connections-share-session", readOnlyConnections: 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner, pool := v06PokerDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}

			s, err := legacyTestNew(Options{
				Pool:            pool,
				Keyring:         Keyring{Current: "v06-disconnect", Keys: map[string][]byte{"v06-disconnect": make([]byte, 32)}},
				Leases:          &fixtureLeases{entries: map[string]leaseEntry{}},
				MailboxCapacity: 8,
			})
			must(err)
			defer s.Close()

			created, err := s.CreateTable(ctx, CreateTableCommand{
				UserID: 910001, Key: "v06-disconnect-table", Name: "Disconnect closure",
				BlindPreset: "5-10", MaxSeats: 2, AllowSpectators: true,
			})
			must(err)
			reserved, err := s.ReserveSeat(ctx, ReserveSeatCommand{
				UserID: 910002, Key: "v06-disconnect-seat", TableID: created.TableID, Seat: 1,
			})
			must(err)
			bought, err := s.BuyIn(ctx, BuyInCommand{
				UserID: 910002, Key: "v06-disconnect-buyin", TableID: created.TableID,
				ReservationID: reserved.ReservationID, AmountUnits: 400 * engine.UnitsPerChip,
			})
			must(err)
			if bought.Status != "CONFIRMED" || bought.SessionID == "" {
				t.Fatal("funded session missing", bought)
			}

			original := legacyRef(t, s, created.TableID, 910002)
			incoming := connected(t, s, fixtureRef(created.TableID, 910002), true)
			if incoming.Control.Mode != "READ_ONLY" {
				t.Fatal("second connection unexpectedly controlled the session")
			}
			replacement, err := s.TakeOver(ctx, TakeOverCommand{
				RequestID: "v06-disconnect-takeover", SessionID: bought.SessionID,
				TargetConnectionID: incoming.Ref.ConnectionID, Auth: incoming.Ref.auth(),
			})
			must(err)
			if replacement.Control.Mode != "CONTROLLER" || replacement.Ref.ControlEpoch <= original.ControlEpoch {
				t.Fatal("replacement controller was not fenced", replacement.Control)
			}

			controls := s.opts.Controls.(*fixtureControls)
			must(s.Disconnected(ctx, original))
			ownerValue, err := controls.ControlLoad(ctx, bought.SessionID)
			must(err)
			if ownerValue != controlValue(replacement.Ref) {
				t.Fatal("stale disconnect removed or changed the replacement controller")
			}
			for range tc.readOnlyConnections {
				readOnly := connected(t, s, fixtureRef(created.TableID, 910002), true)
				if readOnly.Control.Mode != "READ_ONLY" || readOnly.Ref.SessionID != bought.SessionID {
					t.Fatal("additional connection did not remain read-only on the funded session")
				}
			}

			left, err := s.RequestSafeLeave(ctx, SessionCommand{
				UserID: 910002, Key: "v06-disconnect-leave", TableID: created.TableID,
				TargetSessionID: bought.SessionID,
			})
			must(err)
			if left.Status != "CONFIRMED" {
				t.Fatal("safe leave did not cash out immediately", left)
			}
			var settled bool
			must(owner.QueryRow(ctx, `SELECT state='SETTLED' AND current_stack_units=0
				AND final_cashout_units=$2 AND realized_pl_units=0
				FROM poker.sessions WHERE session_id=$1`, bought.SessionID, 400*engine.UnitsPerChip).Scan(&settled))
			if !settled {
				t.Fatal("cashout did not reach the terminal funded session state")
			}
			if tc.loseGrantBeforeRevoke {
				if _, err = s.RenewControl(ctx, replacement.Ref); err == nil {
					t.Fatal("settled session unexpectedly renewed its controller")
				}
			}

			if err = s.RevokeSessionControls(ctx, 910002, replacement.Ref.SessionIDHash); err != nil {
				t.Fatal("cashout made controller revocation fail", err)
			}
			ownerValue, err = controls.ControlLoad(ctx, bought.SessionID)
			must(err)
			if ownerValue != "" {
				t.Fatal("cashout left an exact controller owner behind")
			}
		})
	}
}
