package platform

import (
	"context"
	"errors"
)

// AuditOpsRecordAccess checks the live local role and durably records the read
// before a source reader opens its connection. Existing private-card and seed
// release rules still run in that source reader, using the actual viewer.
func (s *Store) AuditOpsRecordAccess(ctx context.Context, actor, subject int64, kind, recordType, recordID string, query any) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = catalogAdmin(ctx, tx, actor, true); err != nil {
		if errors.Is(err, ErrCatalogForbidden) {
			return ErrOpsForbidden
		}
		return err
	}
	if err = relayOpsAudit(ctx, tx, actor, "RECORD_READ_"+kind, recordType, recordID, nil,
		map[string]any{"subject_user_id": subject, "query": query}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
