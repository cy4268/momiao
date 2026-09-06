package platform

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// Test-only PostgreSQL wire fault: drop the server's first COMMIT completion
// after it has committed, before the application receives its acknowledgement.
func lostCommitStore(t *testing.T, original *Store) (*Store, *atomic.Bool) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := original.pool.Config().Copy()
	target := net.JoinHostPort(cfg.ConnConfig.Host, strconv.Itoa(int(cfg.ConnConfig.Port)))
	var lost atomic.Bool
	go func() {
		for {
			down, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				up, err := net.DialTimeout("tcp", target, 2*time.Second)
				if err != nil {
					_ = down.Close()
					return
				}
				defer up.Close()
				defer down.Close()
				go func() { _, _ = io.Copy(up, down); _ = up.Close() }()
				for {
					var header [5]byte
					if _, err := io.ReadFull(up, header[:]); err != nil {
						return
					}
					size := int(binary.BigEndian.Uint32(header[1:]))
					if size < 4 || size > 16<<20 {
						return
					}
					body := make([]byte, size-4)
					if _, err := io.ReadFull(up, body); err != nil {
						return
					}
					if header[0] == 'C' && string(body) == "COMMIT\x00" && lost.CompareAndSwap(false, true) {
						return
					}
					if _, err := down.Write(header[:]); err != nil {
						return
					}
					if _, err := down.Write(body); err != nil {
						return
					}
				}
			}()
		}
	}()
	cfg.ConnConfig.Host = "127.0.0.1"
	cfg.ConnConfig.Port = uint16(ln.Addr().(*net.TCPAddr).Port)
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		_ = ln.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); _ = ln.Close() })
	return &Store{pool: pool}, &lost
}

func TestRegistrationActualCommitResponseLoss(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	cursor, err := s.RegistrationCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	user := time.Now().UnixMicro()
	r := RegistrationReceipt{cursor + 1, fmt.Sprintf("42684268-0000-4000-8000-%012x", cursor+1), user, fmt.Sprint(900000000000000000 + user), "NEW_DISCORD_REGISTRATION", "v1", time.Now().UTC().Truncate(time.Microsecond)}
	ingest, lost := lostCommitStore(t, s)
	if err = ingest.IngestRegistrationPage(ctx, cursor, RegistrationPage{[]RegistrationReceipt{r}, r.Ordinal}); err == nil || !lost.Load() {
		t.Fatal("source COMMIT response was not lost")
	}
	got, err := s.RegistrationCursor(ctx)
	if err != nil || got != r.Ordinal {
		t.Fatal("committed source cursor lost", err)
	}
	if err = s.IngestRegistrationPage(ctx, cursor, RegistrationPage{[]RegistrationReceipt{r}, r.Ordinal}); err != nil {
		t.Fatal("receipt could not reconcile", err)
	}
	grant, lost := lostCommitStore(t, s)
	found, err := grant.RecoverRegistrationGrant(ctx)
	if !found || err == nil || !lost.Load() {
		t.Fatal("grant COMMIT response was not lost")
	}
	state, err := s.ReadAdmission(ctx, user)
	if err != nil || state.GrantStatus != "CONFIRMED" || state.TransactionID == nil {
		t.Fatal("unknown commit did not resolve original confirmation", err)
	}
	found, err = s.RecoverRegistrationGrant(ctx)
	if err != nil || found {
		t.Fatal("unknown commit issued again", err)
	}
	w, err := s.ReadWallet(ctx, user, ReserveAPICredit)
	if err != nil || w.BalanceUnits != 500000000 || w.LedgerSeq != 1 {
		t.Fatal("unknown commit duplicated wallet", err)
	}
	var count int
	if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM rewards.registration_issuances WHERE newapi_user_id=$1", user).Scan(&count); err != nil || count != 1 {
		t.Fatal("issuance duplicated", err)
	}
	t.Log("actual source and grant COMMIT acknowledgements dropped; original cursor and exactly one complete issuance recovered")
}

func TestRegistrationConnectionCrashRollsBackAndRecovers(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	cursor, err := s.RegistrationCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	user := time.Now().UnixMicro()
	r := RegistrationReceipt{cursor + 1, fmt.Sprintf("42684268-0000-4000-8000-%012x", cursor+1), user, fmt.Sprint(900000000000000000 + user), "NEW_DISCORD_REGISTRATION", "v1", time.Now().UTC().Truncate(time.Microsecond)}
	if err = s.IngestRegistrationPage(ctx, cursor, RegistrationPage{[]RegistrationReceipt{r}, r.Ordinal}); err != nil {
		t.Fatal(err)
	}
	hold, err := s.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Release()
	if _, err = hold.Exec(ctx, "SELECT pg_advisory_lock(682620)"); err != nil {
		t.Fatal(err)
	}
	defer hold.Exec(ctx, "SELECT pg_advisory_unlock(682620)")
	if _, err = s.pool.Exec(ctx, `CREATE FUNCTION rewards.m2b_pause_issuance() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(682620); RETURN NEW; END $$;
 CREATE TRIGGER m2b_pause_issuance BEFORE INSERT ON rewards.registration_issuances FOR EACH ROW EXECUTE FUNCTION rewards.m2b_pause_issuance()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), "DROP TRIGGER IF EXISTS m2b_pause_issuance ON rewards.registration_issuances; DROP FUNCTION IF EXISTS rewards.m2b_pause_issuance()")
	})
	cfg := s.pool.Config().Copy()
	app := fmt.Sprintf("m2b-crash-%d", user)
	cfg.ConnConfig.RuntimeParams["application_name"] = app
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	crashed := &Store{pool: pool}
	done := make(chan error, 1)
	go func() { _, e := crashed.RecoverRegistrationGrant(ctx); done <- e }()
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err = s.pool.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE datname=current_database() AND application_name=$1 AND wait_event_type='Lock' AND query LIKE 'INSERT INTO rewards.registration_issuances%'`, app).Scan(&pid)
		if err == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("worker did not reach held issuance after its uncommitted ledger")
	}
	var killed bool
	if err = s.pool.QueryRow(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE pid=$1 AND datname=current_database() AND application_name=$2`, pid, app).Scan(&killed); err != nil || !killed {
		t.Fatal("test connection not terminated", err)
	}
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("terminated worker did not fail")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("terminated worker did not return")
	}
	_, _ = hold.Exec(ctx, "SELECT pg_advisory_unlock(682620)")
	if _, err = s.pool.Exec(ctx, "DROP TRIGGER m2b_pause_issuance ON rewards.registration_issuances; DROP FUNCTION rewards.m2b_pause_issuance()"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM economy.wallet_ledger WHERE newapi_user_id=$1", user).Scan(&count); err != nil || count != 0 {
		t.Fatal("crashed transaction leaked ledger", err)
	}
	if _, err = s.pool.Exec(ctx, "UPDATE platform_meta.registration_grant_jobs SET next_attempt_at=clock_timestamp() WHERE claim_id=(SELECT claim_id FROM rewards.registration_grants WHERE newapi_user_id=$1)", user); err != nil {
		t.Fatal(err)
	}
	found, err := s.RecoverRegistrationGrant(ctx)
	if err != nil || !found {
		t.Fatal("crash recovery failed", err)
	}
	w, err := s.ReadWallet(ctx, user, ReserveAPICredit)
	if err != nil || w.BalanceUnits != 500000000 || w.LedgerSeq != 1 {
		t.Fatal("crash recovery did not issue exactly once", err)
	}
	t.Log("terminated only the named M2b test backend while issuance was blocked; no partial ledger survived and the durable job recovered")
}
