package poker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/poker/engine"
	"github.com/cy4268/momiao/internal/poker/redislease"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func receiptReader(t *testing.T, f *controlFixture) *Service {
	t.Helper()
	config := f.pool.Config()
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var readOnly string
	if err = pool.QueryRow(context.Background(), "SHOW default_transaction_read_only").Scan(&readOnly); err != nil || readOnly != "on" {
		t.Fatal("reader pool did not enforce PostgreSQL read-only transactions", err)
	}
	opts := f.options()
	opts.Pool = pool
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s // No Connected call: this reader has no Controller or Actor.
}

func receiptFacts(t *testing.T, f *controlFixture) [32]byte {
	t.Helper()
	ctx := context.Background()
	var facts []string
	for _, table := range []string{"poker.tables", "poker.seats", "poker.sessions", "poker.actions", "poker.request_receipts", "poker.funding_operations", "poker.audit_events", "economy.wallet_balances", "economy.wallet_ledger"} {
		var digest string
		if err := f.owner.QueryRow(ctx, "SELECT md5(coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text)::text,'[]')) FROM "+table+" t").Scan(&digest); err != nil {
			t.Fatal(err)
		}
		facts = append(facts, digest)
	}
	var values []string
	for _, key := range f.keys {
		var value string
		var err error
		if strings.HasPrefix(key, redislease.ControlKeyPrefix) {
			value, err = f.store.ControlLoad(ctx, strings.TrimPrefix(key, redislease.ControlKeyPrefix))
		} else {
			value, err = f.admin.Get(ctx, key).Result()
		}
		if err != nil && err != redis.Nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	body, err := json.Marshal([]any{facts, values, f.notices.Load()})
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(body) // Never print seed, encrypted state or Redis identities.
}

func TestReceiptLookupCommittedActionWithoutController(t *testing.T) {
	f, table, refs := playableFixture(t)
	ctx := context.Background()
	v, err := f.service.View(ctx, table, 910001)
	if err != nil {
		t.Fatal(err)
	}
	ref := refs[v.Hand.ActorSeat]
	c := controlledSession(t, f, ref)
	c.Key = "receipt-real-action-01"
	original, err := f.service.Act(ctx, ActCommand{UserID: ref.UserID, Key: c.Key, TableID: table, HandID: c.HandID, ControlEpoch: ref.ControlEpoch, HandVersion: c.ExpectedHandVersion, TableVersion: c.ExpectedTableVersion, Kind: engine.Call, Control: &ref})
	if err != nil {
		t.Fatal(err)
	}
	incoming := connected(t, f.service, fixtureRef(table, ref.UserID), true)
	if incoming.Control.Mode != "READ_ONLY" || f.service.AuthorizeControl(ctx, incoming.Ref) == nil {
		t.Fatal("receipt recovery connection unexpectedly controls table")
	}
	reader, before := receiptReader(t, f), receiptFacts(t, f)
	q := ReceiptQuery{TableID: table, Kind: "action", MutationID: c.Key}
	got, err := reader.LookupReceipt(ctx, ref.UserID, q)
	if err != nil || got.State != "FOUND" || got.Receipt == nil || *got.Receipt != original || got.TableID != table || got.Kind != q.Kind || got.MutationID != q.MutationID {
		t.Fatal("committed action ACK lost despite durable original receipt", got, err)
	}
	if receiptFacts(t, f) != before || len(reader.actors) != 0 || len(reader.connections) != 0 {
		t.Fatal("read-only recovery wrote facts or created control/actor state")
	}
	take := TakeOverCommand{RequestID: "receipt-real-takeover-01", SessionID: ref.SessionID, TargetConnectionID: incoming.Ref.ConnectionID, Auth: incoming.Ref.auth()}
	took, err := f.service.TakeOver(ctx, take)
	if err != nil || took.Receipt == nil || took.Receipt.ControlEpoch == "" || f.service.AuthorizeControl(ctx, ref) == nil {
		t.Fatal("actual explicit TakeOver did not produce a fenced receipt", err)
	}
	before = receiptFacts(t, f)
	got, err = reader.LookupReceipt(ctx, ref.UserID, ReceiptQuery{TableID: table, Kind: "takeover", MutationID: take.RequestID})
	if err != nil || got.Receipt == nil || *got.Receipt != *took.Receipt || receiptFacts(t, f) != before {
		t.Fatal("historical TakeOver receipt changed metadata or facts", got, err)
	}
	body, err := json.Marshal(got.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("TAKEOVER_RECEIPT_JSON=%s", body) // Actual PG+Redis result, synthetic auth only.
}

func TestReceiptLookupOwnerTargetAndVisibility(t *testing.T) {
	f := newControlFixture(t)
	ctx := context.Background()
	table := f.table(t)
	old := f.buy(t, table, 910001, 1)
	ref := connected(t, f.service, fixtureRef(table, 910001), true).Ref
	receipts := map[ReceiptQuery]Receipt{}
	add := func(kind string, run func(context.Context, SessionCommand) (Receipt, error)) {
		c := controlledSession(t, f, ref)
		c.TargetSessionID, c.Key = old, "receipt-real-"+kind
		r, err := run(ctx, c)
		if err != nil {
			t.Fatal(kind, err)
		}
		receipts[ReceiptQuery{TableID: table, Kind: kind, MutationID: c.Key}] = r
	}
	add("sitout", f.service.RequestSitOut)
	add("nextseed", func(ctx context.Context, c SessionCommand) (Receipt, error) {
		return f.service.SetNextSeed(ctx, NextSeedCommand{SessionCommand: c, Contribution: "private synthetic receipt contribution"})
	})
	add("resume", f.service.ResumeSeat)
	add("topup", func(ctx context.Context, c SessionCommand) (Receipt, error) {
		return f.service.RequestTopUp(ctx, TopUpCommand{UserID: c.UserID, TableID: table, TargetSessionID: old, Key: c.Key, AmountUnits: 10 * engine.UnitsPerChip})
	})
	add("leave", f.service.RequestSafeLeave)
	if next := f.buy(t, table, 910001, 1); next == old {
		t.Fatal("real cashout/rebuy did not change Session")
	}
	reader, before := receiptReader(t, f), receiptFacts(t, f)
	for q, want := range receipts {
		got, err := reader.LookupReceipt(ctx, 910001, q)
		if err != nil || got.State != "FOUND" || got.Receipt == nil || *got.Receipt != want || got.Receipt.SessionID != old {
			t.Fatal("original target receipt lost after Session rollover", q.Kind, got, err)
		}
	}
	for _, query := range []struct {
		user int64
		q    ReceiptQuery
	}{
		{910002, ReceiptQuery{table, "topup", "receipt-real-topup"}},
		{910001, ReceiptQuery{uuid(), "topup", "receipt-real-topup"}},
		{910001, ReceiptQuery{table, "action", "receipt-real-topup"}},
		{910001, ReceiptQuery{table, "topup", "receipt-missing-key"}},
	} {
		got, err := reader.LookupReceipt(ctx, query.user, query.q)
		body, marshalErr := json.Marshal(got)
		var wire map[string]json.RawMessage
		decodeErr := json.Unmarshal(body, &wire)
		_, hasReceipt := wire["receipt"]
		if err != nil || marshalErr != nil || decodeErr != nil || hasReceipt || got.State != "NOT_FOUND" || got.Receipt != nil || got.TableID != query.q.TableID || got.Kind != query.q.Kind || got.MutationID != query.q.MutationID {
			t.Fatal("owner/table/kind/not-found boundary", got, err)
		}
	}
	for _, q := range []ReceiptQuery{{table, "host", "receipt-real-topup"}, {table, "", "receipt-real-topup"}, {table, "topup", "short"}, {table, "topup", "key contains spaces"}, {table, "topup", strings.Repeat("x", 129)}, {"AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA", "topup", "receipt-real-topup"}} {
		if _, err := reader.LookupReceipt(ctx, 910001, q); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid query accepted", err)
		}
	}
	if _, err := reader.LookupReceipt(ctx, 0, ReceiptQuery{table, "topup", "receipt-real-topup"}); !errors.Is(err, ErrInvalid) || receiptFacts(t, f) != before {
		t.Fatal("identity boundary or zero-write check failed", err)
	}
	// Only this row is a controlled MVCC fixture, not a claimed business operation.
	tx, err := f.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	q := ReceiptQuery{table, "topup", "receipt-mvcc-controlled"}
	want := Receipt{TableID: table, SessionID: old, Status: "PENDING", Version: 1}
	if err = saveReceipt(ctx, tx, 910001, "poker.topup.v1", q.MutationID, struct{}{}, want); err != nil {
		t.Fatal(err)
	}
	if got, err := reader.LookupReceipt(ctx, 910001, q); err != nil || got.State != "NOT_FOUND" || got.Receipt != nil {
		t.Fatal("uncommitted receipt became visible", got, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	before = receiptFacts(t, f)
	if got, err := reader.LookupReceipt(ctx, 910001, q); err != nil || got.Receipt == nil || *got.Receipt != want || receiptFacts(t, f) != before {
		t.Fatal("committed receipt stayed missing or lookup wrote", got, err)
	}
	// Append a separate malformed fixture; existing receipt history stays immutable.
	q.MutationID = "receipt-malformed-fixture"
	kh := sha256.Sum256([]byte(q.MutationID))
	if _, err = f.owner.Exec(ctx, "INSERT INTO poker.request_receipts(newapi_user_id,scope,key_hash,request_hash,table_id,response) VALUES(910001,'poker.topup.v1',$1,$1,$2,'{}')", kh[:], table); err != nil {
		t.Fatal(err)
	}
	before = receiptFacts(t, f)
	if got, err := reader.LookupReceipt(ctx, 910001, q); err == nil || errors.Is(err, ErrInvalid) || got.Receipt != nil || receiptFacts(t, f) != before {
		t.Fatal("corrupt durable response disguised as a valid query result", got, err)
	}
	t.Log("five real Session receipts retain original targets; owner/table/kind isolation, MVCC and zero-write reads verified")
}
