package platform

import (
 "context"
 "errors"

 "github.com/jackc/pgx/v5"
)

var ErrMaintenanceActive = errors.New("MAINTENANCE_ACTIVE")

// RequireNoMaintenance linearizes acceptance of NEW work with Ops activation.
// Call inside the same READ COMMITTED transaction as the new effect. Recovery,
// settlement, cashout and already accepted work must remain able to finish.
func RequireNoMaintenance(ctx context.Context,tx pgx.Tx,scopes ...string) error {
 if tx==nil||len(scopes)==0{return ErrInvalidMutation}
 if _,err:=tx.Exec(ctx,`SELECT ops.lock_write_scopes($1::text[])`,scopes);err!=nil{return err}
 var active bool
 // This statement obtains a new snapshot after any guard-lock wait.
 if err:=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM unnest($1::text[]) s WHERE ops.is_maintenance_scope_active(s))`,scopes).Scan(&active);err!=nil{return err}
 if active{return ErrMaintenanceActive};return nil
}
