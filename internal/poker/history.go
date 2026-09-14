package poker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HistoryReader struct {
	pool *pgxpool.Pool
	keys Keyring
}

func NewHistoryReader(pool *pgxpool.Pool, keys Keyring) (*HistoryReader, error) {
	if pool == nil {
		return nil, historyaccess.ErrInvalid
	}
	r := &HistoryReader{pool: pool, keys: Keyring{Keys: map[string][]byte{}}}
	for id, key := range keys.Keys {
		if id == "" || len(id) > 255 || len(key) != 32 {
			return nil, historyaccess.ErrInvalid
		}
		r.keys.Keys[id] = append([]byte(nil), key...)
	}
	return r, nil
}

func (r *HistoryReader) read(ctx context.Context, access historyaccess.Access, id string, fn func(context.Context, pgx.Tx, int64, int64) error) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	viewer, subject, err := access.Check(ctx)
	if err != nil {
		return err
	}
	if !platform.ValidOperationKey(id) {
		return historyaccess.ErrInvalid
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return platform.HistoryReadError(ctx, err)
	}
	defer rollback(tx)
	if err = platform.HistoryReadOnly(ctx, tx); err == nil {
		err = fn(ctx, tx, viewer, subject)
	}
	return platform.HistoryReadError(ctx, err)
}

func historyMetadata(ctx context.Context, tx pgx.Tx, subject int64, kind, id string) (HistoryMetadata, error) {
	var raw []byte
	var m HistoryMetadata
	if err := tx.QueryRow(ctx, "SELECT games.history_record_snapshot($1,$2,$3)", subject, kind, id).Scan(&raw); err != nil {
		return m, err
	}
	if json.Unmarshal(raw, &m) != nil || m.SnapshotID == "" || m.Origin == "" || m.CapturedAt.IsZero() {
		return m, historyaccess.ErrUnavailable
	}
	return m, nil
}

func historyConfiguration() HistoryConfiguration {
	return HistoryConfiguration{OriginalReferenceStatus: "NOT_RECORDED", AccessModeStatus: "NOT_RECORDED", BlindOrigin: "NOT_RECORDED", Ante: "NO_ANTE", AnteOrigin: "frozen_v1_rule"}
}

type historyCursor struct {
	Version         int
	Subject         int64
	Parent, Kind    string
	UpperID, LastID string
	UpperAt, LastAt time.Time
}

func historyLimit(n int) (int, error) {
	if n == 0 {
		return 50, nil
	}
	if n < 1 || n > 100 {
		return 0, historyaccess.ErrInvalid
	}
	return n, nil
}

// The relation and projection below are compile-time SQL, never request input.
func historyPage[T any](ctx context.Context, tx pgx.Tx, subject int64, parent, kind, cursor string, limit int, relation, idColumn, projection string) ([]T, string, error) {
	n, err := historyLimit(limit)
	c := historyCursor{Version: 1, Subject: subject, Parent: parent, Kind: kind}
	if err != nil || len(cursor) > 2048 {
		return nil, "", historyaccess.ErrInvalid
	}
	if cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(cursor)
		if e != nil || len(raw) > 1024 || json.Unmarshal(raw, &c) != nil ||
			c.Version != 1 || c.Subject != subject || c.Parent != parent || c.Kind != kind ||
			!platform.ValidOperationKey(c.UpperID) || !platform.ValidOperationKey(c.LastID) ||
			c.UpperAt.IsZero() || c.LastAt.IsZero() || c.LastAt.After(c.UpperAt) ||
			(c.LastAt.Equal(c.UpperAt) && c.LastID > c.UpperID) {
			return nil, "", historyaccess.ErrInvalid
		}
		var valid bool
		e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 "+relation+" AND ("+idColumn+",h.created_at)=($3,$4)) AND EXISTS(SELECT 1 "+relation+" AND ("+idColumn+",h.created_at)=($5,$6))",
			parent, subject, c.UpperID, c.UpperAt, c.LastID, c.LastAt).Scan(&valid)
		if !valid && e == nil {
			e = historyaccess.ErrInvalid
		}
		if e != nil {
			return nil, "", e
		}
	}
	rows, err := tx.Query(ctx, "SELECT "+idColumn+"::text,h.created_at,"+projection+" "+relation+
		" AND ($3::text='' OR (h.created_at,"+idColumn+")<=($4,NULLIF($3,'')::uuid)) AND ($5::text='' OR (h.created_at,"+idColumn+")<($6,NULLIF($5,'')::uuid)) ORDER BY h.created_at DESC,"+idColumn+" DESC LIMIT $7",
		parent, subject, c.UpperID, c.UpperAt, c.LastID, c.LastAt, n+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []T{}
	more := false
	for rows.Next() {
		var id string
		var at time.Time
		var raw []byte
		if err = rows.Scan(&id, &at, &raw); err != nil {
			return nil, "", err
		}
		if len(out) == n {
			more = true
			break
		}
		var item T
		if err = json.Unmarshal(raw, &item); err != nil {
			return nil, "", err
		}
		if c.UpperID == "" {
			c.UpperID, c.UpperAt = id, at
		}
		c.LastID, c.LastAt = id, at
		out = append(out, item)
	}
	if more {
		raw, _ := json.Marshal(c)
		return out, base64.RawURLEncoding.EncodeToString(raw), nil
	}
	return out, "", rows.Err()
}

func (r *HistoryReader) SessionDetail(ctx context.Context, access historyaccess.Access, id string, q HistorySessionQuery) (HistorySessionDetail, error) {
	var d HistorySessionDetail
	err := r.read(ctx, access, id, func(ctx context.Context, tx pgx.Tx, _, subject int64) error {
		var valid bool
		var raw []byte
		e := tx.QueryRow(ctx, `SELECT jsonb_build_object('session_id',s.session_id,'table_id',s.table_id,'seat_no',s.seat_no,
		 'state',s.state,'started_at',s.started_at,'ended_at',s.ended_at,'end_reason',s.end_reason,
		 'initial_buyin_units',s.initial_buyin_units::text,'confirmed_topup_units',f.topup::text,'confirmed_rebuy_units',f.rebuy::text,
		 'final_cashout_units',CASE WHEN s.state='SETTLED' THEN s.final_cashout_units::text END,
		 'realized_pl_units',CASE WHEN s.state='SETTLED' THEN s.realized_pl_units::text END,
		 'funding_count',f.total::text,'hand_count',p.total::text,'read_at',clock_timestamp()),
		 f.owned AND p.owned AND f.buyins=1 AND f.initial=s.initial_buyin_units AND f.topup+f.rebuy=s.total_topup_units
		 AND ((s.state='SETTLED')=(s.ended_at IS NOT NULL))
		 AND CASE WHEN s.state='SETTLED' THEN f.cashouts=1 AND s.ended_at IS NOT NULL AND s.final_cashout_units=f.cashout
		  AND s.realized_pl_units=f.cashout-f.initial-f.topup-f.rebuy ELSE f.cashouts=0 AND s.final_cashout_units IS NULL AND s.realized_pl_units IS NULL END
		 FROM poker.sessions s CROSS JOIN LATERAL(
		  SELECT count(*) total,coalesce(bool_and(h.newapi_user_id=s.newapi_user_id AND h.table_id=s.table_id AND h.seat_no=s.seat_no
		   AND (h.state<>'CONFIRMED' OR (h.confirmed_at IS NOT NULL AND h.confirmed_transaction_id=h.planned_transaction_id))),false) owned,
		   count(*) FILTER(WHERE kind='BUY_IN' AND state='CONFIRMED') buyins,
		   count(*) FILTER(WHERE kind='CASH_OUT' AND state='CONFIRMED') cashouts,
		   coalesce(sum(amount_units) FILTER(WHERE kind='BUY_IN' AND state='CONFIRMED'),0) initial,
		   coalesce(sum(amount_units) FILTER(WHERE kind='TOP_UP' AND state='CONFIRMED'),0) topup,
		   coalesce(sum(amount_units) FILTER(WHERE kind='REBUY' AND state='CONFIRMED'),0) rebuy,
		   coalesce(sum(amount_units) FILTER(WHERE kind='CASH_OUT' AND state='CONFIRMED'),0) cashout
		  FROM poker.funding_operations h WHERE h.session_id=s.session_id) f CROSS JOIN LATERAL(
		  SELECT count(*) total,coalesce(bool_and(p.newapi_user_id=s.newapi_user_id AND p.seat_no=s.seat_no AND h.table_id=s.table_id),true) owned
		  FROM poker.hand_participants p LEFT JOIN poker.hands h USING(hand_id) WHERE p.session_id=s.session_id) p
		 WHERE s.session_id=$1 AND s.newapi_user_id=$2`, id, subject).Scan(&raw, &valid)
		if errors.Is(e, pgx.ErrNoRows) {
			return historyaccess.ErrNotFound
		}
		if e != nil {
			return e
		}
		if !valid || json.Unmarshal(raw, &d) != nil {
			return historyaccess.ErrUnavailable
		}
		d.Configuration = historyConfiguration()
		if d.Metadata, e = historyMetadata(ctx, tx, subject, "POKER_SESSION", id); e != nil {
			return e
		}
		type funding struct {
			HistoryFunding
			TransactionID string `json:"transaction_id"`
		}
		page, next, e := historyPage[funding](ctx, tx, subject, id, "funding", q.FundingCursor, q.FundingLimit,
			"FROM poker.funding_operations h WHERE h.session_id=$1 AND h.newapi_user_id=$2", "h.funding_operation_id",
			`jsonb_build_object('funding_operation_id',h.funding_operation_id,'kind',h.kind,'state',h.state,'amount_units',h.amount_units::text,
			 'created_at',h.created_at,'confirmed_at',h.confirmed_at,'failure_code',h.failure_code,'transaction_id',h.confirmed_transaction_id)`)
		if e != nil {
			return e
		}
		ids := []string{}
		for _, f := range page {
			if f.State == "CONFIRMED" {
				ids = append(ids, f.TransactionID)
			}
		}
		if e = tx.QueryRow(ctx, "SELECT poker.history_funding_transactions($1,$2,$3)", subject, id, ids).Scan(&raw); e != nil {
			return e
		}
		var transactions []platform.HistoryTransaction
		if json.Unmarshal(raw, &transactions) != nil || len(transactions) != len(ids) {
			return historyaccess.ErrUnavailable
		}
		d.Funding, d.NextFundingCursor = []HistoryFunding{}, next
		at := 0
		for _, f := range page {
			if f.State == "CONFIRMED" {
				if transactions[at].ID != f.TransactionID || transactions[at].Effects == nil || transactions[at].Links == nil {
					return historyaccess.ErrUnavailable
				}
				f.Transaction = &transactions[at]
				at++
			}
			d.Funding = append(d.Funding, f.HistoryFunding)
		}
		d.Hands, d.NextHandCursor, e = historyPage[HistoryHandSummary](ctx, tx, subject, id, "hands", q.HandCursor, q.HandLimit,
			"FROM poker.hands h JOIN poker.hand_participants p USING(hand_id) WHERE p.session_id=$1 AND p.newapi_user_id=$2", "h.hand_id",
			`jsonb_build_object('hand_id',h.hand_id,'hand_no',h.hand_no::text,'state',h.state,'created_at',h.created_at,'settled_at',h.settled_at)`)
		return e
	})
	if err != nil {
		return HistorySessionDetail{}, err
	}
	return d, nil
}
