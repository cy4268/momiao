package platform

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRegistrationMigrationUpgrade(t *testing.T) {
	anchor := integrationStore(t)
	ctx := context.Background()
	name := fmt.Sprintf("momiao_test_m2b_upgrade_%d", time.Now().UnixMicro())
	if !strings.HasPrefix(name, "momiao_test_m2b_upgrade_") {
		t.Fatal("unsafe database name")
	}
	if _, err := anchor.pool.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE template0"); err != nil {
		t.Fatal(err)
	}
	config := anchor.pool.Config().Copy()
	config.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	s := &Store{pool: pool}
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil || len(files) < 6 || files[5] != "migrations/0006_admission.sql" {
		t.Fatal("migration manifest")
	}
	admissionSQL, err := migrations.ReadFile(files[5])
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(admissionSQL)) != "6466e21dfc6332dffd785016f704269d95d687a13677468076f0ef82f8e8a4e9" {
		t.Fatal("approved 0006 changed")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, `CREATE SCHEMA platform_meta;CREATE TABLE platform_meta.schema_migrations(version BIGINT PRIMARY KEY,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	for i, file := range files[:5] {
		raw, err := migrations.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, string(raw), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatal(err)
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(raw))
		if _, err = tx.Exec(ctx, "INSERT INTO platform_meta.schema_migrations(version,checksum) VALUES($1,$2)", i+1, hash); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	const user int64 = 62001
	if _, err = s.InitializeProfile(ctx, user, 0, "Upgrade-before", "system-default"); err != nil {
		t.Fatal(err)
	}
	renamed := "Upgrade-after"
	before, err := s.UpdateProfile(ctx, user, ProfilePatch{ExpectedVersion: 1, DisplayName: &renamed})
	if err != nil || before.ProfileVersion != 2 || before.NextRenameAt == nil {
		t.Fatal("pre-upgrade profile", err)
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var latest int
	if err = pool.QueryRow(ctx, "SELECT max(version) FROM platform_meta.schema_migrations").Scan(&latest); err != nil || latest != len(files) {
		t.Fatal("upgrade did not apply actual latest manifest", err)
	}
	after, err := s.EnsureProvisionalProfile(ctx, user)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("upgrade/ensure changed complete profile")
	}
	renamed = "Must-wait"
	if _, err = s.UpdateProfile(ctx, user, ProfilePatch{ExpectedVersion: 2, DisplayName: &renamed}); err != ErrRenameCooldown {
		t.Fatal("upgrade bypassed cooldown", err)
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM identity.master_profile_name_history WHERE newapi_user_id=$1", user).Scan(&count); err != nil || count != 2 {
		t.Fatal("history changed", err)
	}
	for _, u := range []int64{62002, 62003} {
		p, err := s.EnsureProvisionalProfile(ctx, u)
		if err != nil || p.ProfileVersion != 0 {
			t.Fatal("provisional collision", err)
		}
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal("migration replay", err)
	}
	var hash string
	if err = pool.QueryRow(ctx, "SELECT checksum FROM platform_meta.schema_migrations WHERE version=5").Scan(&hash); err != nil || hash != "e110e85e17b31318789e321dcfd89ff17ac7ca4cc9c9c8fa73f1ce56e93d515e" {
		t.Fatal("approved 0005 changed")
	}
	for _, table := range []string{"identity.native_registration_inbox", "rewards.registration_issuances", "rewards.registration_grants"} {
		if _, err = pool.Exec(ctx, "TRUNCATE "+table+" CASCADE"); err == nil {
			t.Fatal("immutable registration table truncated")
		}
	}
	t.Logf("verified V5 to V6 upgrade in %s; complete v2, history and cooldown preserved", name)
}

func TestRegistrationConfirmationRequiresCompleteIssuance(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	user := time.Now().UnixMicro()
	cursor, err := s.RegistrationCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r := RegistrationReceipt{cursor + 1, fmt.Sprintf("42684268-0000-4000-8000-%012x", cursor+1), user, fmt.Sprint(900000000000000000 + user), "NEW_DISCORD_REGISTRATION", "v1", time.Now().UTC().Truncate(time.Microsecond)}
	if err = s.IngestRegistrationPage(ctx, cursor, RegistrationPage{[]RegistrationReceipt{r}, r.Ordinal}); err != nil {
		t.Fatal(err)
	}
	var claim, biz string
	if err = s.pool.QueryRow(ctx, "SELECT claim_id::text,biz_id FROM rewards.registration_grants WHERE newapi_user_id=$1", user).Scan(&claim, &biz); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"confirmed_without_issuance", "issuance_without_confirmation"} {
		t.Run(mode, func(t *testing.T) {
			tx, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rollback(tx)
			if _, err = tx.Exec(ctx, "INSERT INTO economy.wallet_balances(newapi_user_id,asset_type) VALUES($1,'RESERVE_API_CREDIT')", user); err != nil {
				t.Fatal(err)
			}
			entry, err := applyInTx(ctx, tx, Mutation{user, ReserveAPICredit, RegistrationGrantAmount, "INITIAL_GRANT_REGISTRATION", biz, "INITIAL_GRANT_REGISTRATION", biz})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "confirmed_without_issuance" {
				_, err = tx.Exec(ctx, "UPDATE rewards.registration_grants SET status='CONFIRMED',transaction_id=$2,confirmed_at=now() WHERE claim_id=$1", claim, entry.TransactionID)
			} else {
				_, err = tx.Exec(ctx, `INSERT INTO rewards.registration_issuances(claim_id,newapi_user_id,biz_id,direction,amount_units,asset_type,policy_version,transaction_id,ledger_entry_id) VALUES($1,$2,$3,'ISSUE',500000000,'RESERVE_API_CREDIT','v1',$4,$5)`, claim, user, biz, entry.TransactionID, entry.ID)
			}
			if err != nil {
				t.Fatal("test did not reach deferred commit check", err)
			}
			if err = tx.Commit(ctx); err == nil {
				t.Fatal("partial issuance committed")
			}
			var count int
			if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM economy.wallet_ledger WHERE newapi_user_id=$1", user).Scan(&count); err != nil || count != 0 {
				t.Fatal("partial ledger escaped", err)
			}
		})
	}
	found, err := s.RecoverRegistrationGrant(ctx)
	if err != nil || !found {
		t.Fatal("valid confirmation did not recover", err)
	}
}
