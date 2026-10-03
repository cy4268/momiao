package main

import (
	"context"
	"time"

	"github.com/cy4268/momiao/internal/botgames"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Resolve never initializes an account/profile or acknowledges a notice. Native
// status and uniqueness are evaluated before the existing platform admission.
type botGamesResolver struct {
	native      *pgxpool.Pool
	platform    *platform.Store
	declaration *accessDeclaration
}

func (r botGamesResolver) Resolve(ctx context.Context, subject string) (int64, error) {
	fail := func(code string) (int64, error) { return 0, botgames.Fault{Code: code} }
	if !botGamesSubject(subject) {
		return fail("INVALID_REQUEST")
	}
	if ctx == nil || r.native == nil || r.platform == nil || r.declaration == nil {
		return fail("UPSTREAM_UNAVAILABLE")
	}
	tx, e := r.native.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if e != nil {
		return fail("UPSTREAM_UNAVAILABLE")
	}
	defer tx.Rollback(ctx)
	// Do not prefilter disabled/deleted rows: they still make a binding ambiguous.
	rows, e := tx.Query(ctx, `SELECT id,status,deleted_at FROM public.users WHERE discord_id=$1 LIMIT 2`, subject)
	if e != nil {
		return fail("UPSTREAM_UNAVAILABLE")
	}
	var user int64
	count := 0
	restricted := false
	for rows.Next() {
		var id int64
		var status int
		var deleted *time.Time
		if rows.Scan(&id, &status, &deleted) != nil {
			rows.Close()
			return fail("UPSTREAM_UNAVAILABLE")
		}
		count++
		user = id
		restricted = restricted || id <= 0 || status != 1 || deleted != nil
	}
	e = rows.Err()
	rows.Close()
	if e != nil || tx.Commit(ctx) != nil {
		return fail("UPSTREAM_UNAVAILABLE")
	}
	if count == 0 {
		return fail("NOT_LINKED")
	}
	if count != 1 || restricted {
		return fail("ACCOUNT_RESTRICTED")
	}
	exists, e := r.platform.AccountRefExists(ctx, user)
	if e != nil {
		return fail("UPSTREAM_UNAVAILABLE")
	}
	if !exists {
		return fail("ACCOUNT_NOT_READY")
	}
	profile, e := r.platform.ReadProfile(ctx, user)
	if e != nil {
		return fail("UPSTREAM_UNAVAILABLE")
	}
	if profile.Status != "COMPLETE" {
		return fail("ACCOUNT_NOT_READY")
	}
	notice, e := r.platform.ReadMigrationNotice(ctx, user, r.declaration.MigrationApplicability == "NO_MIGRATION_APPLICABLE")
	if e != nil {
		return fail("UPSTREAM_UNAVAILABLE")
	}
	if notice.State != "NOT_REQUIRED" && notice.State != "ACKNOWLEDGED" {
		return fail("ACCOUNT_NOT_READY")
	}
	if r.declaration.Resources["EXPERIENCE"] != "AVAILABLE" {
		return fail("MAINTENANCE")
	}
	return user, nil
}
