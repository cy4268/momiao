package identity

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The existing dev database runs the real registration transaction. Rolling it
// back leaves no test account or immutable receipt for the grant worker.
func TestDiscordRegistration(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires the existing dev database")
	}
	dsn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	pool, err := pgxpool.New(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev database")
	}
	defer pool.Close()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var ordinal int64
	if err := tx.QueryRow(t.Context(), `SELECT ordinal FROM momiao_admission.receipt_counter WHERE id=1 FOR UPDATE`).Scan(&ordinal); err != nil {
		t.Fatal(err)
	}
	profile := DiscordUser{ID: "900000000000000001", DisplayName: "Registration transaction check"}
	// Reuse a real username to exercise official OAuth's collision fallback.
	if err := tx.QueryRow(t.Context(), `SELECT username FROM identity.users ORDER BY id LIMIT 1`).Scan(&profile.Username); err != nil {
		t.Fatal(err)
	}
	id, err := registerDiscord(t.Context(), tx, profile, "dev-registration-check")
	if err != nil {
		t.Fatal("register local Discord user", err)
	}
	again, err := registerDiscord(t.Context(), tx, profile, "dev-registration-check")
	if err != nil || again != id {
		t.Fatal("repeat registration did not reuse the account", err)
	}
	var username, subject, policy, operation string
	var current, receiptOrdinal int64
	var defaults bool
	err = tx.QueryRow(t.Context(), `SELECT u.username,u.discord_id,r.policy_version,r.operation_id::text,r.ordinal,c.ordinal,
 u.role=1 AND u.status=1 AND u.password_hash='' AND u.group_name='default'
 FROM identity.users u JOIN identity.account_refs a ON a.newapi_user_id=u.id
 JOIN momiao_admission.registration_receipts r ON r.native_user_id=u.id
 CROSS JOIN momiao_admission.receipt_counter c WHERE u.id=$1 AND c.id=1`, id).
		Scan(&username, &subject, &policy, &operation, &receiptOrdinal, &current, &defaults)
	if err != nil || !defaults || username != "discord_"+strconv.FormatInt(id, 10) || subject != profile.ID || policy != "dev-registration-check" || len(operation) != 36 || operation[14] != '4' || current != ordinal+1 || receiptOrdinal != current {
		t.Fatal("registration user/ref/receipt/counter mismatch", err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	var restored bool
	err = pool.QueryRow(t.Context(), `SELECT ordinal=$2
 AND NOT EXISTS(SELECT 1 FROM identity.users WHERE id=$1)
 AND NOT EXISTS(SELECT 1 FROM identity.account_refs WHERE newapi_user_id=$1)
 AND NOT EXISTS(SELECT 1 FROM momiao_admission.registration_receipts WHERE native_user_id=$1)
 FROM momiao_admission.receipt_counter WHERE id=1`, id, ordinal).Scan(&restored)
	if err != nil || !restored {
		t.Fatal("registration rollback left rows or advanced receipt counter", err)
	}
	t.Log("user + account_ref + receipt atomic; repeat reused ID; rollback restored rows/counter")
}
