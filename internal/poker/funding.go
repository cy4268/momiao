package poker

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type seatRow struct {
	Seat                                  int
	Session                               string
	User                                  int64
	Display, State                        string
	Stack, Initial, Topups                int64
	Control                               uint64
	Connected                             bool
	Disconnected, SitOutSince, RebuyUntil *time.Time
	Timeouts                              int
	TimeoutSitOut, SitOutNext, Leave      bool
	Contribution                          string
	ContributionVersion                   uint64
	Pending                               int64
}

func loadSeats(ctx context.Context, tx pgx.Tx, table string) ([]seatRow, error) {
	rows, err := tx.Query(ctx, `SELECT seat.seat_no,s.session_id::text,s.newapi_user_id,s.display_name_snapshot,seat.state,s.current_stack_units,s.initial_buyin_units,s.total_topup_units,s.control_epoch,seat.connected,seat.disconnected_since,seat.sit_out_since,seat.rebuy_deadline_at,seat.timeout_count,seat.timeout_sit_out,seat.sit_out_next_hand,seat.leave_after_hand,seat.next_client_seed_contribution,seat.contribution_version,coalesce((SELECT sum(f.amount_units) FROM poker.funding_operations f WHERE f.session_id=s.session_id AND f.kind IN('TOP_UP','REBUY') AND f.state='PENDING'),0) FROM poker.seats seat JOIN poker.sessions s ON s.session_id=seat.session_id WHERE seat.table_id=$1 ORDER BY seat.seat_no`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []seatRow
	for rows.Next() {
		var r seatRow
		if err = rows.Scan(&r.Seat, &r.Session, &r.User, &r.Display, &r.State, &r.Stack, &r.Initial, &r.Topups, &r.Control, &r.Connected, &r.Disconnected, &r.SitOutSince, &r.RebuyUntil, &r.Timeouts, &r.TimeoutSitOut, &r.SitOutNext, &r.Leave, &r.Contribution, &r.ContributionVersion, &r.Pending); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func userSeat(ctx context.Context, tx pgx.Tx, table string, user int64) (seatRow, error) {
	seats, err := loadSeats(ctx, tx, table)
	if err != nil {
		return seatRow{}, err
	}
	for _, r := range seats {
		if r.User == user {
			return r, nil
		}
	}
	return seatRow{}, ErrDenied
}
func ownsHand(ctx context.Context, tx pgx.Tx, session string) (bool, error) {
	var active bool
	err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM poker.hand_participants p JOIN poker.hands h USING(hand_id) WHERE p.session_id=$1 AND h.state<>'SETTLED')", session).Scan(&active)
	return active, err
}

func (s *Service) ReserveSeat(ctx context.Context, c ReserveSeatCommand) (Receipt, error) {
	if c.Auth.UserID != 0 && c.Auth.UserID != c.UserID {
		return Receipt{}, ErrDenied
	}
	if err := s.AuthorizeTable(ctx, c.TableID, c.Auth); err != nil {
		return Receipt{}, err
	}
	return s.mutate(ctx, c.TableID, func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		if err := s.authorizeTableRow(ctx, tx, t, c.Auth); err != nil {
			return Receipt{}, err
		}
		if r, ok, err := findReceipt(ctx, tx, c.UserID, "poker.reserve.v1", c.Key, c); err != nil || ok {
			return r, err
		}
		if err := requireAdmission(ctx, tx, t, c.UserID, ""); err != nil {
			return Receipt{}, err
		}
		if !t.Accepting || c.Seat < 1 || c.Seat > t.MaxSeats {
			return Receipt{}, ErrDenied
		}
		var occupied bool
		if err := tx.QueryRow(ctx, "SELECT session_id IS NOT NULL FROM poker.seats WHERE table_id=$1 AND seat_no=$2", t.ID, c.Seat).Scan(&occupied); err != nil {
			return Receipt{}, err
		}
		if occupied {
			return Receipt{}, ErrDenied
		}
		if _, err := tx.Exec(ctx, "UPDATE poker.seat_reservations SET state='EXPIRED' WHERE table_id=$1 AND seat_no=$2 AND state='LEASE_ACTIVE' AND expires_at<=$3", t.ID, c.Seat, t.Now); err != nil {
			return Receipt{}, err
		}
		// Scope and identity are part of the lease owner, not merely a client key.
		_, kh := hashes(c.Key, struct {
			Scope   string
			Command ReserveSeatCommand
		}{"poker.reserve.v1", c})
		token := hex.EncodeToString(kh[:])
		expires := t.Now.Add(30 * time.Second)
		ok, err := s.opts.Leases.Acquire(ctx, t.ID, c.Seat, token, expires)
		if err != nil {
			return Receipt{}, err
		}
		if !ok {
			return Receipt{}, ErrBusy
		}
		t.onRollback = func(cleanup context.Context) { _ = s.opts.Leases.Release(cleanup, t.ID, c.Seat, token) }
		// The live external lease is authoritative. A vanished prior key allows
		// replacement even when its immutable request receipt has time remaining.
		if _, err = tx.Exec(ctx, "UPDATE poker.seat_reservations SET state='EXPIRED' WHERE table_id=$1 AND seat_no=$2 AND state='LEASE_ACTIVE'", t.ID, c.Seat); err != nil {
			return Receipt{}, err
		}
		r := Receipt{TableID: t.ID, ReservationID: uuid(), Status: "LEASE_ACTIVE", Version: t.Version + 1}
		if _, err = tx.Exec(ctx, "INSERT INTO poker.seat_reservations(reservation_id,table_id,seat_no,newapi_user_id,lease_token,state,expires_at) VALUES($1,$2,$3,$4,$5,'LEASE_ACTIVE',$6)", r.ReservationID, t.ID, c.Seat, c.UserID, token, expires); err != nil {
			return Receipt{}, err
		}
		t.beforeCommit = func(ctx context.Context, _ *Receipt) error { return s.authorizeTableRow(ctx, tx, t, c.Auth) }
		return r, saveReceipt(ctx, tx, c.UserID, "poker.reserve.v1", c.Key, c, r)
	})
}

func insertFunding(ctx context.Context, tx pgx.Tx, t *tableRow, seat int, session string, user int64, kind string, amount int64, reservation string, hash [32]byte) (string, error) {
	id := uuid()
	var res any
	if reservation != "" {
		res = reservation
	}
	_, err := tx.Exec(ctx, `INSERT INTO poker.funding_operations(funding_operation_id,table_id,seat_no,session_id,newapi_user_id,kind,amount_units,reservation_id,request_hash,planned_transaction_id,planned_ledger_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, t.ID, seat, session, user, kind, amount, res, hash[:], uuid(), uuid())
	return id, err
}

var errLeaseLost = errors.New("RESERVATION_LEASE_LOST")

func applyFunding(ctx context.Context, tx pgx.Tx, t *tableRow, id, kind string, afterApply func(context.Context, pgx.Tx) error) (Receipt, error) {
	// SQL identifiers are code-owned constants. No generic wallet mutation exists.
	query := "SELECT economy.poker_buy_in_apply($1,$2,$3)"
	if kind == "TOP_UP" || kind == "REBUY" {
		query = "SELECT economy.poker_top_up_apply($1,$2,$3)"
	}
	if kind == "CASH_OUT" {
		query = "SELECT economy.poker_cash_out_apply($1,$2,$3)"
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		return Receipt{}, err
	}
	var body []byte
	var callbackErr error
	err = sp.QueryRow(ctx, query, t.ID, t.Epoch, id).Scan(&body)
	// A successful gateway has acquired the wallet lock but its effects are
	// still rollbackable. Recheck the external owner here, never before a wait.
	if err == nil && afterApply != nil {
		callbackErr = afterApply(ctx, sp)
		err = callbackErr
	}
	if err != nil {
		rollback(sp)
		if callbackErr != nil && !errors.Is(callbackErr, errLeaseLost) && !errors.Is(callbackErr, ErrTableAccessRequired) {
			return Receipt{}, callbackErr
		}
		code := "POKER_FUNDING_REJECTED"
		if errors.Is(err, errLeaseLost) {
			code = "RESERVATION_LEASE_LOST"
		} else if errors.Is(err, ErrTableAccessRequired) {
			code = ErrTableAccessRequired.Error()
		}
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) {
			switch pgerr.Message {
			case "INSUFFICIENT_CHIPS", "INVALID_BUY_IN", "INVALID_TOP_UP", "INVALID_REBUY", "HAND_UNSETTLED", "RESERVATION_EXPIRED":
				code = pgerr.Message
			}
			if pgerr.Code == "23505" {
				code = "FUNDING_CONFLICT"
			}
		}
		if _, updateErr := tx.Exec(ctx, "UPDATE poker.funding_operations SET state='FAILED_NO_EFFECT',failure_code=$2 WHERE funding_operation_id=$1", id, code); updateErr != nil {
			return Receipt{}, updateErr
		}
		return Receipt{TableID: t.ID, FundingID: id, Status: "FAILED_NO_EFFECT", FailureCode: code, Version: t.Version + 1}, nil
	}
	if err = sp.Commit(ctx); err != nil {
		return Receipt{}, err
	}
	var r Receipt
	if err = json.Unmarshal(body, &r); err != nil {
		return Receipt{}, err
	}
	r.Version = t.Version + 1
	return r, nil
}

func (s *Service) BuyIn(ctx context.Context, c BuyInCommand) (Receipt, error) {
	if c.Auth.UserID != 0 && c.Auth.UserID != c.UserID {
		return Receipt{}, ErrDenied
	}
	if err := s.AuthorizeTable(ctx, c.TableID, c.Auth); err != nil {
		return Receipt{}, err
	}
	return s.mutate(ctx, c.TableID, func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		if err := s.authorizeTableRow(ctx, tx, t, c.Auth); err != nil {
			return Receipt{}, err
		}
		if r, ok, err := findReceipt(ctx, tx, c.UserID, "poker.buyin.v1", c.Key, c); err != nil || ok {
			return r, err
		}
		if err := requireAdmission(ctx, tx, t, c.UserID, ""); err != nil {
			return Receipt{}, err
		}
		if !units(c.AmountUnits) || c.AmountUnits < 40*t.BigBlind || c.AmountUnits > 100*t.BigBlind {
			return Receipt{}, ErrInvalid
		}
		var seat int
		var token, state string
		var expires time.Time
		if err := tx.QueryRow(ctx, "SELECT seat_no,lease_token,state,expires_at FROM poker.seat_reservations WHERE reservation_id=$1 AND table_id=$2 AND newapi_user_id=$3", c.ReservationID, t.ID, c.UserID).Scan(&seat, &token, &state, &expires); err != nil {
			return Receipt{}, ErrDenied
		}
		valid, err := s.opts.Leases.Valid(ctx, t.ID, seat, token, t.Now)
		if err != nil {
			return Receipt{}, err
		}
		if !valid || state != "LEASE_ACTIVE" || !expires.After(t.Now) {
			r := Receipt{TableID: t.ID, Status: "FAILED_NO_EFFECT", FailureCode: "RESERVATION_LEASE_LOST", Version: t.Version + 1}
			if _, err = tx.Exec(ctx, "UPDATE poker.seat_reservations SET state='EXPIRED' WHERE reservation_id=$1 AND state='LEASE_ACTIVE'", c.ReservationID); err != nil {
				return Receipt{}, err
			}
			return r, saveReceipt(ctx, tx, c.UserID, "poker.buyin.v1", c.Key, c, r)
		}
		_, rh := hashes(c.Key, c)
		id, err := insertFunding(ctx, tx, t, seat, uuid(), c.UserID, "BUY_IN", c.AmountUnits, c.ReservationID, rh)
		if err != nil {
			return Receipt{}, err
		}
		// Keep the operation row, but hold every successful effect and receipt
		// in a savepoint until the actor's final access check.
		effects, err := tx.Begin(ctx)
		if err != nil {
			return Receipt{}, err
		}
		r, err := applyFunding(ctx, effects, t, id, "BUY_IN", func(ctx context.Context, locked pgx.Tx) error {
			var fresh time.Time
			if err := locked.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&fresh); err != nil {
				return err
			}
			if !expires.After(fresh) {
				return errLeaseLost
			}
			valid, err := s.opts.Leases.Valid(ctx, t.ID, seat, token, fresh)
			if err != nil {
				return err
			}
			if !valid {
				return errLeaseLost
			}
			return s.authorizeTableRow(ctx, locked, t, c.Auth)
		})
		if err != nil {
			return Receipt{}, err
		}
		if r.Status == "CONFIRMED" {
			// Admission is not a live control connection. The authenticated
			// transport rebinds its original socket via Connected after this commit.
			if _, err = effects.Exec(ctx, "UPDATE poker.seats SET connected=false,disconnected_since=$3 WHERE table_id=$1 AND seat_no=$2", t.ID, seat, t.Now); err != nil {
				return Receipt{}, err
			}
			t.onCommit = func(cleanup context.Context) { _ = s.opts.Leases.Release(cleanup, t.ID, seat, token) }
		}
		t.beforeCommit = func(ctx context.Context, receipt *Receipt) error {
			return s.finalizeBuyIn(ctx, tx, effects, t, c, receipt)
		}
		return r, saveReceipt(ctx, effects, c.UserID, "poker.buyin.v1", c.Key, c, r)
	})
}

func (s *Service) finalizeBuyIn(ctx context.Context, tx, effects pgx.Tx, t *tableRow, c BuyInCommand, receipt *Receipt) error {
	if receipt.Status == "CONFIRMED" {
		if err := s.authorizeTableRow(ctx, effects, t, c.Auth); err != nil {
			if !errors.Is(err, ErrTableAccessRequired) {
				return err
			}
			if err = effects.Rollback(ctx); err != nil {
				return err
			}
			failed := Receipt{TableID: t.ID, FundingID: receipt.FundingID, Status: "FAILED_NO_EFFECT", FailureCode: ErrTableAccessRequired.Error(), Version: t.Version + 1}
			if _, err = tx.Exec(ctx, "UPDATE poker.funding_operations SET state='FAILED_NO_EFFECT',failure_code=$2 WHERE funding_operation_id=$1", failed.FundingID, failed.FailureCode); err != nil {
				return err
			}
			if err = saveReceipt(ctx, tx, c.UserID, "poker.buyin.v1", c.Key, c, failed); err != nil {
				return err
			}
			t.onCommit = nil
			*receipt = failed
			return nil
		}
	}
	return effects.Commit(ctx)
}

func (s *Service) RequestTopUp(ctx context.Context, c TopUpCommand) (Receipt, error) {
	if !canonicalSessionID(c.TargetSessionID) {
		return Receipt{}, ErrInvalid
	}
	intent := topUpIntent{sessionIntent{c.UserID, c.TableID, c.TargetSessionID}, c.AmountUnits}
	return s.mutate(ctx, c.TableID, func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		if r, ok, err := findReceipt(ctx, tx, c.UserID, "poker.topup.v1", c.Key, intent); err != nil || ok {
			return r, err
		}
		if !units(c.AmountUnits) || c.AmountUnits == 0 {
			return Receipt{}, ErrInvalid
		}
		seat, err := userSeat(ctx, tx, t.ID, c.UserID)
		if err != nil {
			return Receipt{}, err
		}
		if seat.Session != c.TargetSessionID {
			return Receipt{}, ErrDenied
		}
		if seat.Leave || t.State == "CLOSING" || t.State == "CLOSED" || c.AmountUnits > 100*t.BigBlind-seat.Stack-seat.Pending {
			return Receipt{}, ErrDenied
		}
		kind := "TOP_UP"
		if seat.State == "REBUY_WINDOW" {
			kind = "REBUY"
			if c.AmountUnits < 40*t.BigBlind || seat.RebuyUntil == nil || !seat.RebuyUntil.After(t.Now) {
				return Receipt{}, ErrDenied
			}
		}
		_, rh := hashes(c.Key, intent)
		id, err := insertFunding(ctx, tx, t, seat.Seat, seat.Session, c.UserID, kind, c.AmountUnits, "", rh)
		if err != nil {
			return Receipt{}, err
		}
		r := Receipt{TableID: t.ID, SessionID: seat.Session, FundingID: id, Status: "PENDING", AmountUnits: decimal(c.AmountUnits), Version: t.Version + 1}
		active, err := ownsHand(ctx, tx, seat.Session)
		if err != nil {
			return Receipt{}, err
		}
		if !active {
			r, err = applyFunding(ctx, tx, t, id, kind, nil)
			if err != nil {
				return Receipt{}, err
			}
		}
		return r, saveReceipt(ctx, tx, c.UserID, "poker.topup.v1", c.Key, intent, r)
	})
}

func cashout(ctx context.Context, tx pgx.Tx, t *tableRow, seat seatRow, reason string) (Receipt, error) {
	if _, err := tx.Exec(ctx, "UPDATE poker.sessions SET end_reason=coalesce(end_reason,$2) WHERE session_id=$1", seat.Session, reason); err != nil {
		return Receipt{}, err
	}
	var id string
	err := tx.QueryRow(ctx, "SELECT funding_operation_id::text FROM poker.funding_operations WHERE session_id=$1 AND kind='CASH_OUT' AND state<>'FAILED_NO_EFFECT'", seat.Session).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		_, rh := hashes("poker_cashout:"+seat.Session, seat.Session)
		id, err = insertFunding(ctx, tx, t, seat.Seat, seat.Session, seat.User, "CASH_OUT", 0, "", rh)
	}
	if err != nil {
		return Receipt{}, err
	}
	return applyFunding(ctx, tx, t, id, "CASH_OUT", nil)
}
func (s *Service) RequestSafeLeave(ctx context.Context, c SessionCommand) (Receipt, error) {
	if !canonicalSessionID(c.TargetSessionID) {
		return Receipt{}, ErrInvalid
	}
	intent := sessionIntent{c.UserID, c.TableID, c.TargetSessionID}
	return s.mutate(ctx, c.TableID, func(ctx context.Context, tx pgx.Tx, t *tableRow) (Receipt, error) {
		if r, ok, err := findReceipt(ctx, tx, c.UserID, "poker.leave.v1", c.Key, intent); err != nil || ok {
			return r, err
		}
		seat, err := userSeat(ctx, tx, t.ID, c.UserID)
		if err != nil {
			return Receipt{}, err
		}
		if seat.Session != c.TargetSessionID {
			return Receipt{}, ErrDenied
		}
		active, err := ownsHand(ctx, tx, seat.Session)
		if err != nil {
			return Receipt{}, err
		}
		r := Receipt{TableID: t.ID, SessionID: seat.Session, Status: "LEAVE_AFTER_HAND", Version: t.Version + 1}
		if active {
			_, err = tx.Exec(ctx, "UPDATE poker.seats SET state='LEAVE_AFTER_HAND',leave_after_hand=true WHERE table_id=$1 AND seat_no=$2", t.ID, seat.Seat)
		} else {
			r, err = cashout(ctx, tx, t, seat, "SAFE_LEAVE")
		}
		if err != nil {
			return Receipt{}, err
		}
		if err = updateEmpty(ctx, tx, t); err != nil {
			return Receipt{}, err
		}
		return r, saveReceipt(ctx, tx, c.UserID, "poker.leave.v1", c.Key, intent, r)
	})
}

func updateEmpty(ctx context.Context, tx pgx.Tx, t *tableRow) error {
	_, err := tx.Exec(ctx, `UPDATE poker.tables SET empty_since=CASE WHEN NOT EXISTS(SELECT 1 FROM poker.seats WHERE table_id=$1 AND session_id IS NOT NULL) AND spectator_count=0 AND NOT EXISTS(SELECT 1 FROM poker.hands WHERE table_id=$1 AND state<>'SETTLED') AND NOT EXISTS(SELECT 1 FROM poker.funding_operations WHERE table_id=$1 AND state='PENDING') THEN coalesce(empty_since,$2) ELSE NULL END WHERE table_id=$1`, t.ID, t.Now)
	return err
}

func (s *Service) boundary(ctx context.Context, tx pgx.Tx, t *tableRow) error {
	seats, err := loadSeats(ctx, tx, t.ID)
	if err != nil {
		return err
	}
	for _, seat := range seats {
		active, err := ownsHand(ctx, tx, seat.Session)
		if err != nil {
			return err
		}
		if active {
			continue
		}
		reason := ""
		if seat.Leave || t.State == "CLOSING" {
			reason = "SAFE_LEAVE"
		} else if seat.State == "REBUY_WINDOW" && seat.RebuyUntil != nil && !seat.RebuyUntil.After(t.Now) {
			reason = "BUST_NO_REBUY"
		} else if (seat.Disconnected != nil && !seat.Disconnected.Add(15*time.Minute).After(t.Now)) || (seat.SitOutSince != nil && !seat.SitOutSince.Add(15*time.Minute).After(t.Now)) {
			reason = "AUTO_SAFE_LEAVE"
		}
		if reason != "" {
			if _, err = tx.Exec(ctx, "UPDATE poker.funding_operations SET state='FAILED_NO_EFFECT',failure_code='SESSION_LEAVING' WHERE session_id=$1 AND state='PENDING' AND kind IN('TOP_UP','REBUY')", seat.Session); err != nil {
				return err
			}
			r, err := cashout(ctx, tx, t, seat, reason)
			if err != nil {
				return err
			}
			if r.Status != "CONFIRMED" {
				return ErrDenied
			}
			continue
		}
		if seat.SitOutNext || seat.TimeoutSitOut || !seat.Connected {
			if _, err = tx.Exec(ctx, "UPDATE poker.seats SET state='SIT_OUT',sit_out_since=coalesce(sit_out_since,$3) WHERE table_id=$1 AND seat_no=$2", t.ID, seat.Seat, t.Now); err != nil {
				return err
			}
		}
	}
	rows, err := tx.Query(ctx, "SELECT funding_operation_id::text,kind FROM poker.funding_operations WHERE table_id=$1 AND state='PENDING' AND kind IN('TOP_UP','REBUY') ORDER BY created_at,funding_operation_id", t.ID)
	if err != nil {
		return err
	}
	type pending struct{ id, kind string }
	var pendingOps []pending
	for rows.Next() {
		var p pending
		if err = rows.Scan(&p.id, &p.kind); err != nil {
			rows.Close()
			return err
		}
		pendingOps = append(pendingOps, p)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, p := range pendingOps {
		if _, err = applyFunding(ctx, tx, t, p.id, p.kind, nil); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE poker.seats seat SET state='REBUY_WINDOW',rebuy_deadline_at=$2::timestamptz+interval '60 seconds' FROM poker.sessions sess WHERE seat.table_id=$1 AND seat.session_id=sess.session_id AND sess.current_stack_units=0 AND seat.state NOT IN('REBUY_WINDOW','LEFT')`, t.ID, t.Now); err != nil {
		return err
	}
	if err = updateEmpty(ctx, tx, t); err != nil {
		return err
	}
	if t.State == "CLOSING" {
		_, err = closeTableIfSettled(ctx, tx, t)
	}
	return err
}

func closeTableIfSettled(ctx context.Context, tx pgx.Tx, t *tableRow) (bool, error) {
	tag, err := tx.Exec(ctx, `UPDATE poker.tables SET lifecycle_state='CLOSED',
	 accepting_players=false,allow_new_hands=false,closed_at=$2
	 WHERE table_id=$1
	 AND NOT EXISTS(SELECT 1 FROM poker.seats WHERE table_id=$1 AND session_id IS NOT NULL)
	 AND NOT EXISTS(SELECT 1 FROM poker.sessions WHERE table_id=$1 AND (state<>'SETTLED' OR current_stack_units<>0))
	 AND NOT EXISTS(SELECT 1 FROM poker.funding_operations WHERE table_id=$1 AND state='PENDING')
	 AND NOT EXISTS(SELECT 1 FROM poker.hands WHERE table_id=$1 AND state<>'SETTLED')`, t.ID, t.Now)
	return tag.RowsAffected() == 1, err
}
