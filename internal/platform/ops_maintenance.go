package platform

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

var maintenanceScopes = []string{"CHALDEA_USER_WRITES", "WALLET_EXCHANGE", "REWARDS", "DIRECT_PLAY_NEW_ROUNDS", "POKER_NEW_TABLES_NEW_HANDS", "RANKINGS_PUBLISHING", "ANNOUNCEMENTS_SCHEDULING"}

type MaintenanceSettings struct {
	ID      string   `json:"maintenance_id,omitempty"`
	Enabled bool     `json:"enabled"`
	Message string   `json:"message"`
	Scopes  []string `json:"scopes"`
}

// ValidateMaintenance is the HTTP boundary; internal callers use known scopes.
func ValidateMaintenance(value *MaintenanceSettings) error {
	if value.Scopes == nil {
		value.Scopes = slices.Clone(maintenanceScopes)
	}
	if value.Enabled && len(value.Scopes) == 0 {
		return ErrOpsInvalid
	}
	for _, scope := range value.Scopes {
		if !slices.Contains(maintenanceScopes, scope) {
			return ErrOpsInvalid
		}
	}
	slices.Sort(value.Scopes)
	value.Scopes = slices.Compact(value.Scopes)
	value.Message = strings.TrimSpace(value.Message)
	return nil
}

type maintenanceReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readMaintenance(ctx context.Context, q maintenanceReader, environment string) (MaintenanceSettings, error) {
	value := MaintenanceSettings{Scopes: []string{}}
	rows, err := q.Query(ctx, `SELECT w.maintenance_id::text,w.message,
 ARRAY(SELECT scope FROM ops.maintenance_window_scopes WHERE maintenance_id=w.maintenance_id ORDER BY scope)
 FROM ops.maintenance_windows w WHERE state='ACTIVE' AND environment=$1 ORDER BY created_at,maintenance_id`, environment)
	if err != nil {
		return value, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, message string
		var scopes []string
		if err = rows.Scan(&id, &message, &scopes); err != nil {
			return value, err
		}
		value.ID = id
		value.Enabled = true
		if message != "" {
			if value.Message != "" {
				value.Message += "\n"
			}
			value.Message += message
		}
		value.Scopes = append(value.Scopes, scopes...)
	}
	slices.Sort(value.Scopes)
	value.Scopes = slices.Compact(value.Scopes)
	return value, rows.Err()
}
func (s *Store) ReadMaintenance(ctx context.Context, environment string) (MaintenanceSettings, error) {
	return readMaintenance(ctx, s.pool, environment)
}
func (s *Store) SaveMaintenance(ctx context.Context, user int64, environment string, value MaintenanceSettings) (MaintenanceSettings, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MaintenanceSettings{}, err
	}
	defer tx.Rollback(ctx)
	result, err := saveMaintenance(ctx, tx, user, environment, value)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func saveMaintenance(ctx context.Context, tx pgx.Tx, user int64, environment string, value MaintenanceSettings) (MaintenanceSettings, error) {
	// Reuse the existing admission locks: accepted work can finish; new work sees
	// the committed switch.
	// ponytail: lock all seven guards for rare admin saves; narrow to old/new
	// scopes only if settings writes become contended.
	if _, err := tx.Exec(ctx, `SELECT scope FROM ops.maintenance_scope_guards ORDER BY scope FOR UPDATE`); err != nil {
		return value, err
	}
	before, err := readMaintenance(ctx, tx, environment)
	if err != nil {
		return value, err
	}
	if _, err = tx.Exec(ctx, `UPDATE ops.maintenance_windows SET state='COMPLETED',ended_at=clock_timestamp(),ended_by=$1 WHERE environment=$2 AND state='ACTIVE'`, user, environment); err != nil {
		return value, err
	}
	value.ID = ""
	if value.Enabled {
		err = tx.QueryRow(ctx, `INSERT INTO ops.maintenance_windows(maintenance_id,state,message,environment,created_by,activated_by,activated_at)
  VALUES(gen_random_uuid(),'ACTIVE',$1,$2,$3,$3,clock_timestamp()) RETURNING maintenance_id::text`, value.Message, environment, user).Scan(&value.ID)
		if err != nil {
			return value, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO ops.maintenance_window_scopes(maintenance_id,scope) SELECT $1,unnest($2::text[])`, value.ID, value.Scopes); err != nil {
			return value, err
		}
	} else {
		value.Message = ""
		value.Scopes = []string{}
	}
	_, err = tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_user_id,action,target,before_state,after_state) VALUES($1,'MAINTENANCE_SAVE',$2,$3,$4)`, user, environment, before, value)
	return value, err
}
