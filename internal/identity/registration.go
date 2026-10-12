// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
// Adapted from official new-api@f1164142 controller/oauth.go findOrCreateOAuthUser:
// existing provider ID wins; otherwise preserve provider name/display name and
// common-user defaults. momiao adds a local admission receipt in the same tx.
package identity

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) RegisterDiscord(ctx context.Context, sessions Sessions, browser Session, profile DiscordUser, policy string) (Session, *Challenge, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Session{}, nil, err
	}
	defer tx.Rollback(ctx)
	id, err := registerDiscord(ctx, tx, profile, policy)
	if err != nil {
		return Session{}, nil, err
	}
	return finishLogin(ctx, tx, sessions, browser, id)
}

func registerDiscord(ctx context.Context, tx pgx.Tx, profile DiscordUser, policy string) (int64, error) {
	// The receipt counter already serializes new registrations. Recheck the
	// provider after locking so simultaneous callbacks never create two grants.
	var ordinal int64
	if err := tx.QueryRow(ctx, `SELECT ordinal FROM momiao_admission.receipt_counter WHERE id=1 FOR UPDATE`).Scan(&ordinal); err != nil {
		return 0, err
	}
	var id int64
	var status int
	var deleted *time.Time
	err := tx.QueryRow(ctx, `SELECT id,status,deleted_at FROM identity.users WHERE discord_id=$1 FOR UPDATE`, profile.ID).Scan(&id, &status, &deleted)
	if err == nil {
		if status != 1 || deleted != nil {
			return 0, ErrCredentials
		}
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	displayName := profile.DisplayName
	if displayName == "" {
		displayName = profile.Username
	}
	// Unlike max(id)+1 in upstream, the existing identity sequence allocates IDs.
	err = tx.QueryRow(ctx, `WITH next_user AS (SELECT nextval('identity.users_id_seq') AS id)
 INSERT INTO identity.users(id,username,display_name,discord_id)
 SELECT id,CASE WHEN octet_length($1)<=20 AND NOT EXISTS(SELECT 1 FROM identity.users WHERE username=$1)
 THEN $1 ELSE 'discord_'||id::text END,$2,$3 FROM next_user RETURNING id`, profile.Username, displayName, profile.ID).Scan(&id)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity.account_refs(newapi_user_id) VALUES($1)`, id); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO momiao_admission.registration_receipts
 (ordinal,operation_id,native_user_id,discord_subject,source,policy_version,created_at)
 VALUES($1,gen_random_uuid(),$2,$3,'NEW_DISCORD_REGISTRATION',$4,now())`, ordinal+1, id, profile.ID, policy); err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, `UPDATE momiao_admission.receipt_counter SET ordinal=$1 WHERE id=1`, ordinal+1)
	return id, err
}
