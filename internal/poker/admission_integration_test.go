package poker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/jackc/pgx/v5"
)

type admissionFixture struct {
	f       *controlFixture
	table   string
	barrier *lobbyQueryBarrier
	run     func() (Receipt, error)
	refs    map[int]ControlRef
}

func prepareAdmissionFixture(t *testing.T, kind string, before bool) admissionFixture {
	t.Helper()
	f := newControlFixture(t)
	s, b := tracedLobbyService(t, f, "SELECT ops.lock_poker_admission_scopes()", before, false)
	b.used.Store(true) // Setup uses the real domain, without pausing it.
	f.service.Close()
	f.service = s
	ctx := context.Background()
	table := f.table(t)
	a, err := s.actor(ctx, table, false)
	if err != nil {
		t.Fatal(err)
	}
	<-a.ready
	x := admissionFixture{f: f, table: table, barrier: b}
	key := "admission-business-" + uuid()
	switch kind {
	case "create":
		x.run = func() (Receipt, error) {
			return s.CreateTable(ctx, CreateTableCommand{UserID: 910003, Key: key, Name: "Admission race", BlindPreset: "5-10", MaxSeats: 2})
		}
	case "reserve":
		x.run = func() (Receipt, error) {
			return s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910002, Key: key, TableID: table, Seat: 2})
		}
	case "buyin":
		r, e := s.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910002, Key: "admission-reservation-" + uuid(), TableID: table, Seat: 2})
		if e != nil {
			t.Fatal(e)
		}
		x.run = func() (Receipt, error) {
			return s.BuyIn(ctx, BuyInCommand{UserID: 910002, Key: key, TableID: table, ReservationID: r.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
		}
	case "hand":
		f.buy(t, table, 910001, 1)
		f.buy(t, table, 910002, 2)
		if _, err = f.owner.Exec(ctx, "UPDATE poker.tables SET allow_new_hands=false WHERE table_id=$1", table); err != nil {
			t.Fatal(err)
		}
		x.refs = map[int]ControlRef{1: connected(t, s, fixtureRef(table, 910001), true).Ref, 2: connected(t, s, fixtureRef(table, 910002), true).Ref}
		if _, err = f.owner.Exec(ctx, "UPDATE poker.tables SET allow_new_hands=true WHERE table_id=$1", table); err != nil {
			t.Fatal(err)
		}
		x.run = func() (Receipt, error) { return s.mutate(ctx, table, s.prepareHand(910001, key)) }
	}
	b.used.Store(false)
	return x
}
func maintenanceWindow(t *testing.T, f *controlFixture, scope string) string {
	t.Helper()
	ctx := context.Background()
	id, op := uuid(), uuid()
	if _, err := f.owner.Exec(ctx, `INSERT INTO ops.admin_operations(operation_id,actor_kind,newapi_user_id,action,request_hash,details,result) VALUES($1,'OFFLINE',910001,'SYNTHETIC_ADMISSION_FIXTURE',repeat('a',64),'{}','{}')`, op); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, `INSERT INTO ops.maintenance_windows(maintenance_id,state,reason,impact_snapshot,impact_hash,environment,scheduled_start_at,created_by,operation_id) VALUES($1,'SCHEDULED','synthetic local admission race','{}',decode(repeat('a',64),'hex'),'STAGING',clock_timestamp(),910001,$2)`, id, op); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, `INSERT INTO ops.maintenance_window_scopes(maintenance_id,scope) VALUES($1,$2)`, id, scope); err != nil {
		t.Fatal(err)
	}
	return id
}

const lockAdmissionExclusive = `SELECT scope FROM ops.maintenance_scope_guards WHERE scope IN ('CHALDEA_USER_WRITES','POKER_NEW_TABLES_NEW_HANDS') ORDER BY scope FOR UPDATE`

func activateAdmissionWindow(t *testing.T, tx pgx.Tx, id string) {
	t.Helper()
	if _, err := tx.Exec(context.Background(), `UPDATE ops.maintenance_windows SET state='ACTIVE',activated_at=clock_timestamp(),activated_by=910001,state_version=state_version+1 WHERE maintenance_id=$1`, id); err != nil {
		t.Fatal(err)
	}
}

func TestLobbyAdmissionMaintenanceLockOrder(t *testing.T) {
	for _, scope := range []string{"CHALDEA_USER_WRITES", "POKER_NEW_TABLES_NEW_HANDS"} {
		for _, maintenanceFirst := range []bool{true, false} {
			for _, kind := range []string{"create", "reserve", "buyin", "hand"} {
				order := "business-first"
				if maintenanceFirst {
					order = "maintenance-first"
				}
				t.Run(scope+"/"+order+"/"+kind, func(t *testing.T) {
					x := prepareAdmissionFixture(t, kind, maintenanceFirst)
					ctx := context.Background()
					id := maintenanceWindow(t, x.f, scope)
					op, err := x.f.owner.Begin(ctx)
					if err != nil {
						t.Fatal(err)
					}
					defer rollback(op)
					var opPID int
					if err = op.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&opPID); err != nil {
						t.Fatal(err)
					}
					if maintenanceFirst {
						if _, err = op.Exec(ctx, lockAdmissionExclusive); err != nil {
							t.Fatal(err)
						}
						activateAdmissionWindow(t, op, id)
					}
					before := receiptFacts(t, x.f)
					type result struct {
						r Receipt
						e error
					}
					done := make(chan result, 1)
					go func() { r, e := x.run(); done <- result{r, e} }()
					var pid int
					select {
					case pid = <-x.barrier.reached:
					case r := <-done:
						t.Fatalf("actual %s completed without admission scope lock: status=%s error=%v business_changed=%v", kind, r.r.Status, r.e, receiptFacts(t, x.f) != before)
					case <-time.After(2 * time.Second):
						t.Fatal("domain admission query not reached")
					}
					if maintenanceFirst {
						close(x.barrier.resume)
						waitLobbyBlocked(t, ctx, x.f.owner, pid)
						if err = op.Commit(ctx); err != nil {
							t.Fatal(err)
						}
						r := <-done
						if !errors.Is(r.e, ErrMaintenance) || receiptFacts(t, x.f) != before {
							t.Fatalf("post-lock fresh guard must reject without new work: status=%s error=%v", r.r.Status, r.e)
						}
					} else {
						locked := make(chan error, 1)
						go func() { _, e := op.Exec(ctx, lockAdmissionExclusive); locked <- e }()
						waitLobbyBlocked(t, ctx, x.f.owner, opPID)
						close(x.barrier.resume)
						r := <-done
						if r.e != nil || r.r.Status == "" {
							t.Fatal("prior business did not commit", r.r, r.e)
						}
						if err = <-locked; err != nil {
							t.Fatal(err)
						}
						var pending Receipt
						if kind == "hand" {
							pending, err = x.f.service.RequestTopUp(ctx, TopUpCommand{UserID: 910001, Key: "pre-maintenance-topup-" + uuid(), TableID: x.table, TargetSessionID: x.refs[1].SessionID, AmountUnits: 10 * engine.UnitsPerChip})
							if err != nil || pending.Status != "PENDING" {
								t.Fatal("accepted pre-maintenance funding not pending", pending, err)
							}
						}
						activateAdmissionWindow(t, op, id)
						if err = op.Commit(ctx); err != nil {
							t.Fatal(err)
						}
						if again, e := x.run(); e != nil || !again.Duplicate || again.Version != r.r.Version {
							t.Fatal("maintenance blocked original durable duplicate", again, e)
						}
						if kind == "hand" {
							if _, e := x.f.service.mutate(ctx, x.table, x.f.service.dealCommitted); e != nil {
								t.Fatal("maintenance blocked COMMITTED deal", e)
							}
							foldFromSocket(t, x.f, x.table, x.refs)
							var fundingState string
							if err = x.f.owner.QueryRow(ctx, "SELECT state FROM poker.funding_operations WHERE funding_operation_id=$1", pending.FundingID).Scan(&fundingState); err != nil || fundingState != "CONFIRMED" {
								t.Fatal("maintenance blocked accepted funding settlement", fundingState, err)
							}
							for _, ref := range x.refs {
								if _, e := x.f.service.RequestSafeLeave(ctx, SessionCommand{UserID: ref.UserID, TableID: x.table, Key: "maintenance-safe-leave-" + uuid(), TargetSessionID: ref.SessionID}); e != nil {
									t.Fatal("maintenance blocked safe leave/cashout", e)
								}
							}
						}
					}
					view, err := receiptReader(t, x.f).Lobby(ctx, 910003, LobbyFilter{})
					if err != nil || view.Service.State != "MAINTENANCE" || len(view.Service.MaintenanceScopes) != 1 || view.Service.MaintenanceScopes[0] != scope || view.Viewer.CanCreate || view.Viewer.CanJoin {
						t.Fatal("read-only lobby lost maintenance parity", view.Service, err)
					}
				})
			}
		}
	}
}

func TestLobbyAdmissionRulesAndEligibility(t *testing.T) {
	f := newControlFixture(t)
	table := f.table(t)
	ctx := context.Background()
	reservation, err := f.service.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910003, Key: "eligibility-reserve-original", TableID: table, Seat: 2})
	if err != nil {
		t.Fatal(err)
	}
	runs := []func() (Receipt, error){
		func() (Receipt, error) {
			return f.service.CreateTable(ctx, CreateTableCommand{UserID: 910003, Key: "eligibility-new-table-01", Name: "Eligible", BlindPreset: "5-10", MaxSeats: 2})
		},
		func() (Receipt, error) {
			return f.service.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910003, Key: "eligibility-new-reserve-01", TableID: table, Seat: 1})
		},
		func() (Receipt, error) {
			return f.service.BuyIn(ctx, BuyInCommand{UserID: 910003, Key: "eligibility-new-buyin-01", TableID: table, ReservationID: reservation.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
		},
		func() (Receipt, error) {
			return f.service.mutate(ctx, table, f.service.prepareHand(910001, "eligibility-new-hand-01"))
		},
	}
	if _, err = f.owner.Exec(ctx, "UPDATE poker.ruleset_versions SET evaluator_version='unsupported'"); err != nil {
		t.Fatal(err)
	}
	before := receiptFacts(t, f)
	for i, run := range runs {
		if _, e := run(); !errors.Is(e, ErrRulesetIncomplete) {
			t.Fatalf("new work %d ignored incomplete approved rules: %v", i, e)
		}
	}
	if receiptFacts(t, f) != before {
		t.Fatal("incomplete rules admitted new durable work")
	}
	if _, err = f.owner.Exec(ctx, "UPDATE poker.ruleset_versions SET evaluator_version=$1", engine.EvaluatorVersion); err != nil {
		t.Fatal(err)
	}
	for _, restriction := range []struct {
		state, access string
		accepting     bool
	}{{"WAITING", "PASSWORD", true}, {"WAITING", "PUBLIC", false}, {"CLOSING", "PUBLIC", true}, {"RECOVERING", "PUBLIC", true}} {
		if _, err = f.owner.Exec(ctx, "UPDATE poker.tables SET lifecycle_state=$2,access_mode=$3,accepting_players=$4 WHERE table_id=$1", table, restriction.state, restriction.access, restriction.accepting); err != nil {
			t.Fatal(err)
		}
		for _, run := range runs[1:3] {
			if _, e := run(); !errors.Is(e, ErrDenied) {
				t.Fatal("real table admission restriction ignored", e)
			}
		}
		v, e := receiptReader(t, f).Lobby(ctx, 910003, LobbyFilter{})
		if e != nil || len(v.Tables) != 1 || v.Tables[0].CanJoin {
			t.Fatal("table can_join diverged from mutation guard", e)
		}
	}
	if _, err = f.owner.Exec(ctx, "UPDATE poker.tables SET lifecycle_state='WAITING',access_mode='PUBLIC',accepting_players=true WHERE table_id=$1", table); err != nil {
		t.Fatal(err)
	}
	if _, err = f.owner.Exec(ctx, "UPDATE identity.master_profiles SET profile_version=0,display_name='',normalized_name=NULL,nickname_changed_at=NULL WHERE newapi_user_id=910003"); err != nil {
		t.Fatal(err)
	}
	before = receiptFacts(t, f)
	for _, run := range runs[:3] {
		if _, e := run(); !errors.Is(e, ErrDenied) {
			t.Fatal("incomplete profile admitted create/reserve/buyin", e)
		}
	}
	v, err := receiptReader(t, f).Lobby(ctx, 910003, LobbyFilter{})
	if err != nil || v.Viewer.ProfileComplete || v.Viewer.CanCreate || v.Viewer.CanJoin || receiptFacts(t, f) != before {
		t.Fatal("incomplete profile read/mutation parity failed", err)
	}
	if _, err = f.service.CreateTable(ctx, CreateTableCommand{UserID: 910001, Key: "eligibility-owned-open-table", Name: "Second", BlindPreset: "5-10", MaxSeats: 2}); !errors.Is(err, ErrDenied) {
		t.Fatal("owned open table not denied by shared eligibility", err)
	}
}
