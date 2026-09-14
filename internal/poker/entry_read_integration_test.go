package poker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/cy4268/momiao/internal/poker/redislease"
)

func entryFacts(t *testing.T, f *controlFixture) [32]byte {
	t.Helper()
	var reservations string
	if err := f.owner.QueryRow(context.Background(), `SELECT md5(coalesce(jsonb_agg(to_jsonb(r) ORDER BY reservation_id)::text,'[]')) FROM poker.seat_reservations r`).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	base := receiptFacts(t, f)
	return sha256.Sum256(append(base[:], []byte(reservations)...))
}

func entryReadDeadline(t *testing.T, f *controlFixture, relation string, read func(context.Context) error) {
	t.Helper()
	before := entryFacts(t, f)
	tx, err := f.owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	if _, err = tx.Exec(context.Background(), "LOCK TABLE "+relation+" IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	err = read(ctx) // Real SELECT waits for this isolated fixture's relation lock.
	elapsed := time.Since(start)
	rollback(tx)
	if err == nil || errors.Is(err, ErrInvalid) || elapsed < 2700*time.Millisecond || elapsed > 4*time.Second || entryFacts(t, f) != before {
		t.Fatal("read did not honor its own 3-second total deadline", relation, elapsed, err)
	}
	t.Log("real PG lock wait ended at read deadline", relation, elapsed)
}

func TestEntryReceiptOriginalOwnerAndVisibility(t *testing.T) {
	f := newControlFixture(t)
	ctx := context.Background()
	table := f.table(t)
	created, err := f.service.CreateTable(ctx, CreateTableCommand{UserID: 910002, Key: "entry-real-create-01", Name: "Original create", BlindPreset: "5-10", MaxSeats: 2})
	if err != nil {
		t.Fatal(err)
	}
	reserved, err := f.service.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910001, Key: "entry-real-reserve-01", TableID: table, Seat: 1})
	if err != nil {
		t.Fatal(err)
	}
	bought, err := f.service.BuyIn(ctx, BuyInCommand{UserID: 910001, Key: "entry-real-buyin-01", TableID: table, ReservationID: reserved.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
	if err != nil || bought.Status != "CONFIRMED" {
		t.Fatal(bought, err)
	}
	if _, err = f.service.RequestSafeLeave(ctx, SessionCommand{UserID: 910001, Key: "entry-real-leave-01", TableID: table, TargetSessionID: bought.SessionID}); err != nil {
		t.Fatal(err)
	}
	if f.buy(t, table, 910001, 1) == bought.SessionID {
		t.Fatal("real cashout/rebuy kept original Session")
	}
	reader, before := receiptReader(t, f), entryFacts(t, f)
	for _, test := range []struct {
		user int64
		q    EntryReceiptQuery
		want Receipt
	}{
		{910002, EntryReceiptQuery{"create", "entry-real-create-01", nil}, created},
		{910001, EntryReceiptQuery{"reserve", "entry-real-reserve-01", &table}, reserved},
		{910001, EntryReceiptQuery{"buyin", "entry-real-buyin-01", &table}, bought},
	} {
		got, e := reader.LookupEntryReceipt(ctx, test.user, test.q)
		if e != nil || got.UserID != decimal(test.user) || got.Kind != test.q.Kind || got.MutationID != test.q.MutationID || got.State != "FOUND" || got.Receipt == nil || *got.Receipt != test.want {
			t.Error("lost original committed entry receipt after ACK loss/rollover", test.q.Kind, got, e)
		}
		if got, e = reader.LookupEntryReceipt(ctx, 910003, test.q); e != nil || got.State != "NOT_FOUND" || got.Receipt != nil {
			t.Error("other owner observed original entry", got, e)
		}
	}
	wrong := uuid()
	for _, q := range []EntryReceiptQuery{{"buyin", "entry-real-buyin-01", &wrong}, {"reserve", "entry-real-buyin-01", &table}, {"create", "entry-not-found-01", nil}} {
		got, e := reader.LookupEntryReceipt(ctx, 910001, q)
		body, _ := json.Marshal(got)
		if e != nil || got.State != "NOT_FOUND" || strings.Contains(string(body), `"receipt"`) {
			t.Error("entry not-found boundary", got, e)
		}
	}
	upper := "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA"
	for _, q := range []EntryReceiptQuery{{"create", "entry-real-create-01", &table}, {"reserve", "entry-real-reserve-01", nil}, {"buyin", "entry-real-buyin-01", nil}, {"action", "entry-real-buyin-01", &table}, {"create", "short", nil}, {"buyin", "entry-real-buyin-01", &upper}} {
		if _, e := reader.LookupEntryReceipt(ctx, 910001, q); !errors.Is(e, ErrInvalid) {
			t.Error("invalid entry union accepted", q.Kind, e)
		}
	}
	if _, e := reader.LookupEntryReceipt(ctx, 0, EntryReceiptQuery{"create", "entry-real-create-01", nil}); !errors.Is(e, ErrInvalid) || entryFacts(t, f) != before {
		t.Error("entry identity or zero-write boundary", e)
	}
	for _, kind := range []string{"action", "sitout", "resume", "nextseed", "topup", "leave", "takeover"} {
		if _, e := reader.LookupReceipt(ctx, 910001, ReceiptQuery{Kind: kind, MutationID: "entry-real-buyin-01"}); !errors.Is(e, ErrInvalid) {
			t.Error("entry read broadened existing table-scoped receipt query", kind, e)
		}
	}
	// Controlled MVCC row only, not a claimed second business operation.
	tx, err := f.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	q := EntryReceiptQuery{"create", "entry-controlled-mvcc", nil}
	if err = saveReceipt(ctx, tx, 910002, "poker.create.v1", q.MutationID, struct{}{}, created); err != nil {
		t.Fatal(err)
	}
	if got, e := reader.LookupEntryReceipt(ctx, 910002, q); e != nil || got.State != "NOT_FOUND" {
		t.Error("uncommitted original key became visible", got, e)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	before = entryFacts(t, f)
	if got, e := reader.LookupEntryReceipt(ctx, 910002, q); e != nil || got.Receipt == nil || *got.Receipt != created || entryFacts(t, f) != before {
		t.Error("committed original key stayed missing or wrote", got, e)
	}
	// New malformed rows only; no original immutable receipt is rewritten.
	for _, malformed := range []string{`{}`, `{"table_id":"` + table + `","status":"WAITING"}`, `{"table_id":"` + upper + `","status":"WAITING"}`} {
		key := "entry-malformed-" + uuid()
		kh := sha256.Sum256([]byte(key))
		if _, err = f.owner.Exec(ctx, `INSERT INTO poker.request_receipts(newapi_user_id,scope,key_hash,request_hash,table_id,response) VALUES(910002,'poker.create.v1',$1,$1,$2,$3)`, kh[:], created.TableID, malformed); err != nil {
			t.Fatal(err)
		}
		before = entryFacts(t, f)
		if got, e := reader.LookupEntryReceipt(ctx, 910002, EntryReceiptQuery{"create", key, nil}); e == nil || errors.Is(e, ErrInvalid) || got.Receipt != nil || entryFacts(t, f) != before {
			t.Error("invalid original row/receipt table projection was hidden", got, e)
		}
	}
	entryReadDeadline(t, f, "poker.request_receipts", func(ctx context.Context) error {
		_, e := reader.LookupEntryReceipt(ctx, 910002, EntryReceiptQuery{"create", "entry-real-create-01", nil})
		return e
	})
	if len(reader.actors) != 0 || len(reader.connections) != 0 {
		t.Fatal("entry read allocated Actor/control")
	}
	t.Log("three actual entry mutations retain original receipts after real cashout/rebuy; separate controlled MVCC row proves visibility only")
}

// Pause after the real TLS Redis check so an actual BuyIn can consume the row.
type entryLeaseBarrier struct {
	LeaseStore
	reached chan struct{}
	resume  chan struct{}
}

func (b *entryLeaseBarrier) Valid(ctx context.Context, table string, seat int, token string, now time.Time) (bool, error) {
	valid, err := b.LeaseStore.Valid(ctx, table, seat, token, now)
	close(b.reached)
	select {
	case <-b.resume:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return valid, err
}

func TestReservationCurrentLeaseAndConsumedRace(t *testing.T) {
	f := newControlFixture(t)
	ctx := context.Background()
	table := f.table(t)
	r, err := f.service.ReserveSeat(ctx, ReserveSeatCommand{UserID: 910001, Key: "entry-current-reserve", TableID: table, Seat: 1})
	if err != nil {
		t.Fatal(err)
	}
	var expires time.Time
	var token string
	if err = f.owner.QueryRow(ctx, `SELECT expires_at,lease_token FROM poker.seat_reservations WHERE reservation_id=$1`, r.ReservationID).Scan(&expires, &token); err != nil {
		t.Fatal(err)
	}
	reader := receiptReader(t, f)
	key := redislease.KeyPrefix + table + ":1"
	check := func(valid bool, state string) {
		t.Helper()
		before := entryFacts(t, f)
		ttlBefore, e := f.admin.PTTL(ctx, key).Result()
		if e != nil {
			t.Fatal(e)
		}
		got, e := reader.Reservation(ctx, 910001, table, r.ReservationID)
		if e != nil || got.UserID != "910001" || got.TableID != table || got.ReservationID != r.ReservationID || got.State != "FOUND" || got.Reservation == nil {
			t.Fatal("current reservation fact missing", got, e)
		}
		v := got.Reservation
		body, _ := json.Marshal(got)
		if v.SeatNo != 1 || v.DurableState != state || v.Valid != valid || !v.ExpiresAt.Equal(expires) || v.CheckedAt.IsZero() || strings.Contains(string(body), token) || entryFacts(t, f) != before {
			t.Fatal("lease authority/deadline/zero-write projection", v)
		}
		if ttlAfter, e := f.admin.PTTL(ctx, key).Result(); e != nil || (ttlBefore > 0 && ttlAfter > ttlBefore) {
			t.Fatal("read renewed the isolated seat lease", e)
		}
	}
	check(true, "LEASE_ACTIVE")
	for _, query := range []struct {
		user         int64
		table, resID string
	}{{910002, table, r.ReservationID}, {910001, uuid(), r.ReservationID}, {910001, table, uuid()}} {
		before := entryFacts(t, f)
		got, e := reader.Reservation(ctx, query.user, query.table, query.resID)
		body, _ := json.Marshal(got)
		if e != nil || got.State != "NOT_FOUND" || got.Reservation != nil || strings.Contains(string(body), `"reservation"`) || entryFacts(t, f) != before {
			t.Fatal("reservation owner/target/no-row boundary", got, e)
		}
	}
	if _, e := reader.Reservation(ctx, 0, table, r.ReservationID); !errors.Is(e, ErrInvalid) {
		t.Fatal("reservation unverified identity accepted", e)
	}
	if _, e := reader.Reservation(ctx, 910001, "", r.ReservationID); !errors.Is(e, ErrInvalid) {
		t.Fatal("reservation malformed target accepted", e)
	}
	if _, e := reader.Reservation(ctx, 910001, table, "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA"); !errors.Is(e, ErrInvalid) {
		t.Fatal("noncanonical reservation target accepted", e)
	}
	for _, mode := range []string{"missing", "wrong-token", "no-ttl", "expired-ttl"} {
		switch mode {
		case "missing":
			err = f.admin.Del(ctx, key).Err()
		case "wrong-token":
			err = f.admin.Set(ctx, key, strings.Repeat("f", 64), time.Second).Err()
		case "no-ttl":
			err = f.admin.Set(ctx, key, token, 0).Err()
		case "expired-ttl":
			err = f.admin.Set(ctx, key, token, time.Millisecond).Err()
			time.Sleep(10 * time.Millisecond)
		}
		if err != nil {
			t.Fatal(mode, err)
		}
		check(false, "LEASE_ACTIVE")
	}
	if err = f.admin.Set(ctx, key, token, time.Until(expires)).Err(); err != nil {
		t.Fatal(err)
	}
	// Remove only this fixture user's EVAL permission: the actual TLS call errors.
	if err = f.admin.Do(ctx, "ACL", "SETUSER", f.username, "-eval").Err(); err != nil {
		t.Fatal(err)
	}
	beforeError := entryFacts(t, f)
	failed, readErr := reader.Reservation(ctx, 910001, table, r.ReservationID)
	if err = f.admin.Do(ctx, "ACL", "SETUSER", f.username, "+eval").Err(); err != nil {
		t.Fatal(err)
	}
	if readErr == nil || errors.Is(readErr, ErrInvalid) || failed.Reservation != nil || entryFacts(t, f) != beforeError {
		t.Fatal("real Redis failure disguised as a validity result", failed, readErr)
	}
	barrier := &entryLeaseBarrier{LeaseStore: f.store, reached: make(chan struct{}), resume: make(chan struct{})}
	reader.opts.Leases = barrier
	type result struct {
		view ReservationLookup
		err  error
	}
	done := make(chan result, 1)
	go func() { v, e := reader.Reservation(ctx, 910001, table, r.ReservationID); done <- result{v, e} }()
	select {
	case <-barrier.reached:
	case <-time.After(2 * time.Second):
		t.Fatal("real lease check barrier not reached")
	}
	bought, err := f.service.BuyIn(ctx, BuyInCommand{UserID: 910001, Key: "entry-consume-buyin", TableID: table, ReservationID: r.ReservationID, AmountUnits: 400 * engine.UnitsPerChip})
	before := entryFacts(t, f)
	close(barrier.resume)
	got := <-done
	if err != nil || bought.Status != "CONFIRMED" || got.err != nil || got.view.Reservation == nil || got.view.Reservation.Valid || got.view.Reservation.DurableState != "CONSUMED" || entryFacts(t, f) != before {
		t.Fatal("second PG read missed actual BuyIn committed after Redis check", got, err)
	}
	reader.opts.Leases = f.store
	check(false, "CONSUMED")
	if _, err = f.service.RequestSafeLeave(ctx, SessionCommand{UserID: 910001, Key: "entry-consumed-leave", TableID: table, TargetSessionID: bought.SessionID}); err != nil {
		t.Fatal(err)
	}
	if f.buy(t, table, 910001, 1) == bought.SessionID {
		t.Fatal("consumed reservation did not cross a real Session rollover")
	}
	check(false, "CONSUMED")
	// Mutable reservation corruption fixture, after the real consumption proof.
	if _, err = f.owner.Exec(ctx, `UPDATE poker.seat_reservations SET lease_token='' WHERE reservation_id=$1`, r.ReservationID); err != nil {
		t.Fatal(err)
	}
	before = entryFacts(t, f)
	if got, e := reader.Reservation(ctx, 910001, table, r.ReservationID); e == nil || errors.Is(e, ErrInvalid) || got.Reservation != nil || entryFacts(t, f) != before {
		t.Fatal("corrupt reservation projection became false/not-found", got, e)
	}
	if _, err = f.owner.Exec(ctx, `UPDATE poker.seat_reservations SET lease_token=$2 WHERE reservation_id=$1`, r.ReservationID, token); err != nil {
		t.Fatal(err)
	}
	entryReadDeadline(t, f, "poker.seat_reservations", func(ctx context.Context) error {
		_, e := reader.Reservation(ctx, 910001, table, r.ReservationID)
		return e
	})
	t.Log("real TLS Redis token/TTL checks and post-Redis committed BuyIn use current PG facts without renewal or mutation")
}
