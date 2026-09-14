package platform

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func validOpsRecordAccess(kind, recordType string) bool {
	if !containsString([]string{"LIST", "DETAIL", "VERIFY"}, kind) {
		return false
	}
	switch recordType {
	case "HISTORY_LIST":
		return kind == "LIST"
	case "DIRECT_PLAY_ROUND", "POKER_HAND":
		return kind == "DETAIL" || kind == "VERIFY"
	case "POKER_SESSION", "TRANSACTION":
		return kind == "DETAIL"
	default:
		return false
	}
}

// AuditOpsRecordAccess rechecks the current principal and exact authorization
// epoch, then durably records the intended read before any restricted source
// reader acquires a connection. The caller must still apply the subject filter.
func (s *Store) AuditOpsRecordAccess(ctx context.Context, actor, epoch, subject int64,
	kind, recordType, recordID string) error {
	if s == nil || s.pool == nil {
		return ErrOpsUnavailable
	}
	if actor <= 0 || epoch <= 0 || subject <= 0 || !validOpsRecordAccess(kind, recordType) ||
		!validOpsText(recordID, 512, true) {
		return ErrOpsInvalid
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer rollback(tx)
	principal, err := requireOpsPermission(ctx, tx, actor, epoch, "records.read", true)
	if err != nil {
		return err
	}
	accessID, err := uuidV7()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit.record_access_events(
	 access_id,actor_newapi_user_id,actor_role,actor_scopes_snapshot,actor_authz_epoch,
	 subject_newapi_user_id,access_kind,record_type,record_id)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, accessID, principal.UserID, principal.Role,
		principal.Scopes, principal.Epoch, subject, kind, recordType, recordID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
