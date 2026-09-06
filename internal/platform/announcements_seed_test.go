package platform

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Synthetic seed only. This file is excluded from production builds.
func (s *Store) seedAnnouncementPrincipal(ctx context.Context, userID int64, role string, scope bool) error {
	if userID <= 0 || !slices.Contains([]string{"SUPER_ADMIN", "AUDITOR", "OPERATOR"}, role) || scope && role != "OPERATOR" {
		return ErrAnnouncementInvalid
	}
	if cfg := s.pool.Config().ConnConfig; cfg.Host != "127.0.0.1" || (!strings.HasPrefix(cfg.Database, "m3_announcements_") && !strings.HasPrefix(cfg.Database, "m3_catalog_platform_")) {
		return ErrAnnouncementInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	id, err := uuidV7()
	if err != nil {
		return err
	}
	op, err := uuidV7()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ops.admin_principals(admin_principal_id,newapi_user_id,base_role) VALUES($1,$2,$3)`, id, userID, role); err != nil {
		return announcementDBError(err)
	}
	if scope {
		if _, err = tx.Exec(ctx, `INSERT INTO ops.admin_principal_scopes VALUES($1,'ANNOUNCEMENTS')`, id); err != nil {
			return err
		}
	}
	details := map[string]any{"user_id": fmt.Sprint(userID), "role": role, "announcements_scope": scope}
	if _, err = tx.Exec(ctx, `INSERT INTO ops.admin_operations(operation_id,actor_kind,newapi_user_id,action,request_hash,details,result) VALUES($1,'OFFLINE',$2,'SYNTHETIC_PRINCIPAL_SEED',$3,$4,$4)`, op, userID, announcementHash(details), details); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
