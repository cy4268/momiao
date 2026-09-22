package poker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type catalogDeniedLease struct{ calls int }

func (l *catalogDeniedLease) Acquire(context.Context, string, int, string, time.Time) (bool, error) {
	l.calls++
	return false, ErrDenied
}
func (l *catalogDeniedLease) Valid(context.Context, string, int, string, time.Time) (bool, error) {
	l.calls++
	return false, ErrDenied
}
func (l *catalogDeniedLease) Release(context.Context, string, int, string) error {
	l.calls++
	return ErrDenied
}

type catalogTrace struct {
	queries    []string
	failCommit bool
}

func (q *catalogTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	q.queries = append(q.queries, strings.ToLower(strings.TrimSpace(d.SQL)))
	if q.failCommit && d.SQL == "commit" {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		return cancelled
	}
	return ctx
}
func (*catalogTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestPublicCatalogRuntime(t *testing.T) {
	for _, tc := range []struct {
		name, change, scope, want string
		failure                   bool
	}{
		{"ready", "", "", "PLAY", false},
		{"global maintenance", "", "CHALDEA_USER_WRITES", "MAINTENANCE", false},
		{"poker maintenance", "", "POKER_NEW_TABLES_NEW_HANDS", "MAINTENANCE", false},
		{"unrelated maintenance", "", "WALLET_EXCHANGE", "PLAY", false},
		{"inactive rules", "UPDATE poker.ruleset_versions SET active=false", "", "TEMPORARILY_UNAVAILABLE", false},
		{"unsupported rules", "UPDATE poker.ruleset_versions SET evaluator_version='unknown'", "", "TEMPORARILY_UNAVAILABLE", false},
		{"incomplete before maintenance", "UPDATE poker.ruleset_versions SET evaluator_version='unknown'", "POKER_NEW_TABLES_NEW_HANDS", "TEMPORARILY_UNAVAILABLE", false},
		{"missing rules", "DELETE FROM poker.ruleset_versions", "", "TEMPORARILY_UNAVAILABLE", false},
		{"zero presets", "DELETE FROM poker.blind_preset_versions", "", "TEMPORARILY_UNAVAILABLE", false},
		{"overflow preset", "UPDATE poker.blind_preset_versions SET small_blind_units=100000000000000000,big_blind_units=200000000000000000", "", "", true},
		{"too many presets", "INSERT INTO poker.blind_preset_versions SELECT 'extra-'||i,2500000,5000000,40,100 FROM generate_series(1,101) i", "", "", true},
		{"query error", "", "", "", true},
		{"commit error", "", "", "", true},
		{"closed", "", "", "", true},
		{"cancelled", "", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner, domain := localPokerDB(t)
			ctx := context.Background()
			f := &controlFixture{owner: owner, pool: domain}
			lifecycleTables := map[string]string{}
			if tc.name == "ready" {
				for _, row := range []struct {
					state string
					owner int64
				}{{"PAUSED", 910001}, {"RECOVERING", 910002}, {"CLOSING", 910003}, {"CLOSED", 910001}} {
					id := uuid()
					if _, err := owner.Exec(ctx, `INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version,lifecycle_state,accepting_players,allow_new_hands,closed_at)
					 VALUES($1,$2,$3,2,'5-10','poker-cash-v1-20260906',$4,$5,$5,CASE WHEN $4='CLOSED' THEN clock_timestamp() END)`, id, row.owner, "Lifecycle "+row.state, row.state, row.state != "CLOSING" && row.state != "CLOSED"); err != nil {
						t.Fatal(err)
					}
					if _, err := owner.Exec(ctx, `INSERT INTO poker.seats(table_id,seat_no) SELECT $1,n FROM generate_series(1,2) n`, id); err != nil {
						t.Fatal(err)
					}
					lifecycleTables[id] = row.state
				}
			}
			if tc.change != "" {
				if _, err := owner.Exec(ctx, tc.change); err != nil {
					t.Fatal(err)
				}
			}
			if tc.scope != "" {
				id := maintenanceWindow(t, f, tc.scope)
				if _, err := owner.Exec(ctx, "UPDATE ops.maintenance_windows SET state='ACTIVE' WHERE maintenance_id=$1", id); err != nil {
					t.Fatal(err)
				}
			}
			trace := &catalogTrace{failCommit: tc.name == "commit error"}
			config := domain.Config()
			config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
			config.ConnConfig.Tracer = trace
			pool, err := pgxpool.NewWithConfig(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			var readOnly string
			if err = pool.QueryRow(ctx, "SHOW transaction_read_only").Scan(&readOnly); err != nil || readOnly != "on" {
				t.Fatal("real read-only pool required", readOnly, err)
			}
			lease := &catalogDeniedLease{}
			s, err := New(Options{Pool: pool, Leases: lease, Keyring: Keyring{Current: "catalog", Keys: map[string][]byte{"catalog": make([]byte, 32)}}})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if tc.name == "query error" {
				if _, err = owner.Exec(ctx, "REVOKE SELECT ON poker.ruleset_versions FROM "+pgx.Identifier{config.ConnConfig.User}.Sanitize()); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name == "closed" {
				s.Close()
			}
			if tc.name == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			before := receiptFacts(t, f)
			trace.queries = nil
			got, err := s.CatalogRuntime(ctx)
			if (err != nil) != tc.failure || !tc.failure && got != tc.want {
				t.Fatalf("public runtime=%q error=%v, want=%q failure=%v", got, err, tc.want, tc.failure)
			}
			if lease.calls != 0 || len(s.actors) != 0 || len(s.connections) != 0 || receiptFacts(t, f) != before {
				t.Fatal("public read allocated authority, called leases or changed private facts")
			}
			began := false
			for _, sql := range trace.queries {
				if sql == "begin isolation level repeatable read read only" {
					began = true
					continue
				}
				if sql == "commit" || sql == "rollback" || sql == "set local statement_timeout='2s'" {
					continue
				}
				if strings.Contains(sql, "from (select 1) anchor left join poker.ruleset_versions") || strings.HasPrefix(sql, "select version,small_blind_units,big_blind_units,minimum_buyin_bb,maximum_buyin_bb from poker.blind_preset_versions") {
					continue
				}
				t.Fatal("public read crossed its SQL boundary", sql)
			}
			if !tc.failure {
				if !began {
					t.Fatal("public facts were not one real repeatable-read/read-only transaction")
				}
				v, err := s.Lobby(ctx, 910001, LobbyFilter{})
				want := map[string]string{"PLAY": "READY", "MAINTENANCE": "MAINTENANCE", "TEMPORARILY_UNAVAILABLE": "CONFIG_INCOMPLETE"}[tc.want]
				if err != nil || v.Service.State != want {
					t.Fatal("public readiness drifted from whole Lobby", v.Service, err)
				}
				if tc.name == "ready" {
					got := map[string]string{}
					for _, table := range v.Tables {
						if state, tracked := lifecycleTables[table.TableID]; tracked {
							got[table.TableID] = state
						}
					}
					if len(got) != 2 {
						t.Fatalf("find-table lobby exposed a closing/closed row or hid a live row: %+v", got)
					}
					for id, state := range lifecycleTables {
						_, visible := got[id]
						if visible != (state == "PAUSED" || state == "RECOVERING") {
							t.Fatalf("lifecycle visibility %s=%t", state, visible)
						}
						var durable bool
						if err := owner.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM poker.tables WHERE table_id=$1)`, id).Scan(&durable); err != nil || !durable {
							t.Fatalf("lobby projection deleted durable %s table: %v", state, err)
						}
					}
					for _, state := range []string{"CLOSING", "CLOSED"} {
						hidden, err := s.Lobby(ctx, 910001, LobbyFilter{LifecycleState: state})
						if err != nil || len(hidden.Tables) != 0 {
							t.Fatalf("explicit %s filter escaped find-table terminal exclusion: %+v %v", state, hidden.Tables, err)
						}
					}
				}
			}
			t.Logf("runtime=%s failure=%t transaction_read_only=%s private_facts_unchanged=true lease_calls=0", got, tc.failure, readOnly)
		})
	}
}
