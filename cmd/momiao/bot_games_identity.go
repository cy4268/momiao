package main

import (
	"context"
	"errors"
	"time"

	"github.com/cy4268/momiao/internal/botgames"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

// Resolve reads the same local identity as browser login. It never initializes
// an account/profile or acknowledges a notice. Discord uniqueness is a DB constraint.
type botGamesResolver struct {
	platform    *platform.Store
	declaration *accessDeclaration
}

func (r botGamesResolver) Resolve(ctx context.Context, subject string) (int64, error) {
	fail := func(code string) (int64, error) { return 0, botgames.Fault{Code: code} }
	if !botGamesSubject(subject) {
		return fail("INVALID_REQUEST")
	}
	if r.platform == nil || r.declaration == nil {
		return fail("UPSTREAM_UNAVAILABLE")
	}
	var user int64
	var status int
	var deleted *time.Time
	e := r.platform.Pool().QueryRow(ctx, `SELECT id,status,deleted_at FROM identity.users WHERE discord_id=$1`, subject).Scan(&user, &status, &deleted)
	if errors.Is(e, pgx.ErrNoRows) {
		return fail("NOT_LINKED")
	}
	if e != nil {
		return fail("UPSTREAM_UNAVAILABLE")
	}
	if status != 1 || deleted != nil {
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
