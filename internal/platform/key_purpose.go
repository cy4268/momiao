package platform

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrKeyPurposeVersionStale = errors.New("key purpose version stale")
	ErrKeyPurposePending      = errors.New("key purpose synchronization pending")
	ErrKeyNotOwned            = errors.New("key is not owned by user")
)

type KeyPurposeItem struct {
	TokenID     int64      `json:"token_id,string"`
	Purpose     KeyPurpose `json:"purpose"`
	Version     int64      `json:"version,string"`
	EffectiveAt *time.Time `json:"effective_at"`
	SyncState   string     `json:"sync_state"`
}

type KeyPurposeCommand struct {
	OperationID     string
	UserID          int64
	TokenID         int64
	Purpose         KeyPurpose
	ExpectedVersion int64
}

type KeyPurposeOperation struct {
	Item            KeyPurposeItem
	ExpectedVersion int64
}

const keyPurposeItemSelect = `SELECT e.token_id,e.purpose,e.version,e.effective_at,
 CASE WHEN o.status='SYNCED' THEN 'EFFECTIVE' ELSE 'PENDING' END
 FROM economy.api_key_purpose_events e
 JOIN platform_meta.api_key_purpose_outbox o USING(operation_id)`

func scanKeyPurposeItem(row pgx.Row) (KeyPurposeItem, error) {
	var item KeyPurposeItem
	var effective time.Time
	err := row.Scan(&item.TokenID, &item.Purpose, &item.Version, &effective, &item.SyncState)
	if err == nil {
		effective = effective.UTC()
		item.EffectiveAt = &effective
	}
	return item, err
}

func (s *Store) KeyPurposes(ctx context.Context, user int64) ([]KeyPurposeItem, error) {
	if !validateQuotaUser(user) {
		return nil, ErrInvalidMutation
	}
	rows, err := s.pool.Query(ctx, `SELECT p.token_id,p.purpose,p.version,p.effective_at,
 CASE WHEN o.status='SYNCED' THEN 'EFFECTIVE' ELSE 'PENDING' END
 FROM economy.api_key_purposes p
 JOIN economy.api_key_purpose_events e ON e.token_id=p.token_id AND e.version=p.version
 JOIN platform_meta.api_key_purpose_outbox o USING(operation_id)
 WHERE p.newapi_user_id=$1 ORDER BY p.token_id`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []KeyPurposeItem{}
	for rows.Next() {
		item, err := scanKeyPurposeItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) KeyPurposeOperation(ctx context.Context, user int64, operationID string) (*KeyPurposeOperation, error) {
	if !validateQuotaUser(user) || !ValidOperationKey(operationID) {
		return nil, ErrInvalidMutation
	}
	item, err := scanKeyPurposeItem(s.pool.QueryRow(ctx, keyPurposeItemSelect+" WHERE e.operation_id=$1 AND e.newapi_user_id=$2", operationID, user))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var expected int64
	if err = s.pool.QueryRow(ctx, `SELECT expected_version FROM economy.api_key_purpose_events WHERE operation_id=$1 AND newapi_user_id=$2`, operationID, user).Scan(&expected); err != nil {
		return nil, err
	}
	return &KeyPurposeOperation{Item: item, ExpectedVersion: expected}, nil
}

func (s *Store) SetKeyPurpose(ctx context.Context, command KeyPurposeCommand) (KeyPurposeItem, error) {
	if !ValidOperationKey(command.OperationID) || !validateQuotaUser(command.UserID) || !validateQuotaUser(command.TokenID) ||
		!validWritablePurpose(command.Purpose) || command.ExpectedVersion < 0 {
		return KeyPurposeItem{}, ErrInvalidMutation
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return KeyPurposeItem{}, err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "key-purpose", command.UserID, command.TokenID); err != nil {
		return KeyPurposeItem{}, err
	}

	prior, priorErr := scanKeyPurposeItem(tx.QueryRow(ctx, keyPurposeItemSelect+" WHERE e.operation_id=$1", command.OperationID))
	if priorErr == nil {
		var priorUser, expected int64
		var priorPurpose KeyPurpose
		if err = tx.QueryRow(ctx, `SELECT newapi_user_id,expected_version,purpose FROM economy.api_key_purpose_events WHERE operation_id=$1`, command.OperationID).Scan(&priorUser, &expected, &priorPurpose); err != nil {
			return KeyPurposeItem{}, err
		}
		if priorUser != command.UserID || prior.TokenID != command.TokenID || expected != command.ExpectedVersion || priorPurpose != command.Purpose {
			return KeyPurposeItem{}, ErrIdempotencyConflict
		}
		return prior, tx.Commit(ctx)
	}
	if !errors.Is(priorErr, pgx.ErrNoRows) {
		return KeyPurposeItem{}, priorErr
	}
	if err = RequireNoMaintenance(ctx, tx, "CHALDEA_USER_WRITES"); err != nil {
		return KeyPurposeItem{}, err
	}

	var currentVersion int64
	currentErr := tx.QueryRow(ctx, `SELECT version FROM economy.api_key_purposes WHERE token_id=$1 AND newapi_user_id=$2 FOR UPDATE`, command.TokenID, command.UserID).Scan(&currentVersion)
	if currentErr != nil && !errors.Is(currentErr, pgx.ErrNoRows) {
		return KeyPurposeItem{}, currentErr
	}
	if errors.Is(currentErr, pgx.ErrNoRows) {
		if command.ExpectedVersion != 0 {
			return KeyPurposeItem{}, ErrKeyPurposeVersionStale
		}
	} else {
		if currentVersion != command.ExpectedVersion {
			return KeyPurposeItem{}, ErrKeyPurposeVersionStale
		}
		var status string
		if err = tx.QueryRow(ctx, `SELECT o.status FROM economy.api_key_purpose_events e JOIN platform_meta.api_key_purpose_outbox o USING(operation_id) WHERE e.token_id=$1 AND e.version=$2`, command.TokenID, currentVersion).Scan(&status); err != nil {
			return KeyPurposeItem{}, err
		}
		if status != "SYNCED" {
			return KeyPurposeItem{}, ErrKeyPurposePending
		}
	}

	version := command.ExpectedVersion + 1
	var effectiveAt time.Time
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&effectiveAt); err != nil {
		return KeyPurposeItem{}, err
	}
	effectiveAt = effectiveAt.UTC()
	_, err = tx.Exec(ctx, `INSERT INTO economy.api_key_purpose_events(operation_id,newapi_user_id,token_id,expected_version,version,purpose,effective_at)
 VALUES($1,$2,$3,$4,$5,$6,$7)`, command.OperationID, command.UserID, command.TokenID, command.ExpectedVersion, version, command.Purpose, effectiveAt)
	if err != nil {
		return KeyPurposeItem{}, err
	}
	if command.ExpectedVersion == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO economy.api_key_purposes(token_id,newapi_user_id,purpose,version,effective_at) VALUES($1,$2,$3,$4,$5)`, command.TokenID, command.UserID, command.Purpose, version, effectiveAt)
	} else {
		tag, updateErr := tx.Exec(ctx, `UPDATE economy.api_key_purposes SET purpose=$3,version=$4,effective_at=$5,updated_at=clock_timestamp() WHERE token_id=$1 AND newapi_user_id=$2 AND version=$6`, command.TokenID, command.UserID, command.Purpose, version, effectiveAt, command.ExpectedVersion)
		err = updateErr
		if err == nil && tag.RowsAffected() != 1 {
			err = ErrKeyPurposeVersionStale
		}
	}
	if err != nil {
		return KeyPurposeItem{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform_meta.api_key_purpose_outbox(operation_id,newapi_user_id,token_id) VALUES($1,$2,$3)`, command.OperationID, command.UserID, command.TokenID)
	if err != nil {
		return KeyPurposeItem{}, err
	}
	item := KeyPurposeItem{TokenID: command.TokenID, Purpose: command.Purpose, Version: version, EffectiveAt: &effectiveAt, SyncState: "PENDING"}
	return item, tx.Commit(ctx)
}

func (s *Store) ProcessKeyPurposeOutbox(ctx context.Context, writer NativePurposeWriter) (bool, error) {
	if writer == nil {
		return false, ErrNativeAttributionDependency
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollback(tx)
	var change NativeKeyPurposeChange
	var attempt int
	err = tx.QueryRow(ctx, `SELECT e.operation_id::text,e.newapi_user_id,e.token_id,e.purpose,e.version,e.effective_at,o.attempt_count
 FROM platform_meta.api_key_purpose_outbox o JOIN economy.api_key_purpose_events e USING(operation_id)
 WHERE o.status='PENDING' AND o.next_attempt_at<=clock_timestamp()
 ORDER BY o.next_attempt_at,o.operation_id FOR UPDATE OF o SKIP LOCKED LIMIT 1`).Scan(
		&change.OperationID, &change.UserID, &change.TokenID, &change.Purpose, &change.Version, &change.EffectiveAt, &attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	receipt, callErr := writer.ApplyKeyPurpose(ctx, change)
	if callErr != nil {
		delay := time.Duration(1<<min(attempt, 8)) * time.Second
		_, err = tx.Exec(ctx, `UPDATE platform_meta.api_key_purpose_outbox SET attempt_count=attempt_count+1,next_attempt_at=clock_timestamp()+$2::interval,last_error_code=$3,updated_at=clock_timestamp() WHERE operation_id=$1`, change.OperationID, delay.String(), "DEPENDENCY_UNAVAILABLE")
		if err != nil {
			return true, err
		}
		if err = tx.Commit(ctx); err != nil {
			return true, err
		}
		return true, callErr
	}
	status, code := "SYNCED", ""
	if receipt.Result == "STALE_VERSION" || receipt.Result == "ACCOUNT_RESTRICTED" {
		status, code = "REJECTED", receipt.Result
	}
	_, err = tx.Exec(ctx, `UPDATE platform_meta.api_key_purpose_outbox SET status=$2,attempt_count=attempt_count+1,last_error_code=$3,
 synced_at=CASE WHEN $2='SYNCED' THEN clock_timestamp() ELSE NULL END,updated_at=clock_timestamp() WHERE operation_id=$1`, change.OperationID, status, code)
	if err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}

func RunKeyPurposeWorker(ctx context.Context, store *Store, writer NativePurposeWriter) {
	if store == nil || writer == nil {
		return
	}
	for {
		worked, err := store.ProcessKeyPurposeOutbox(ctx, writer)
		if ctx.Err() != nil {
			return
		}
		if worked && err == nil {
			continue
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func ParseKeyPurposeVersion(value string) (int64, error) {
	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil || version < 0 || strconv.FormatInt(version, 10) != value {
		return 0, ErrInvalidMutation
	}
	return version, nil
}
