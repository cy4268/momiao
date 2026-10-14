package games

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/roulette"
)

func TestLocalOpsGames(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires the existing isolated dev database")
	}
	dsn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store, err := platform.Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev database")
	}
	defer store.Close()
	ctx := t.Context()
	tx, err := store.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var admin, regular int64
	if err = tx.QueryRow(ctx, `SELECT min(id) FILTER(WHERE role>=10),min(id) FILTER(WHERE role<10) FROM identity.users WHERE status=1 AND deleted_at IS NULL`).Scan(&admin, &regular); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET role=10 WHERE id=$1`, admin); err != nil {
		t.Fatal(err)
	}
	service := &Service{store: store}
	before, err := loadOpsRegistryState(ctx, tx, "scratch", false)
	if err != nil {
		t.Fatal(err)
	}
	source, err := loadOpsConfigRecord(ctx, tx, "scratch", *before.ActiveConfigVersionID, false)
	if err != nil {
		t.Fatal(err)
	}
	input := OpsGameInput{Title: before.Title + " (dev validation)", SortOrder: before.SortOrder, PublicationState: before.PublicationState, ConfiguredRuntimeState: "MAINTENANCE"}
	if _, err = service.saveOpsGame(ctx, tx, regular, "scratch", input); !errors.Is(err, platform.ErrCatalogForbidden) {
		t.Fatal("regular settings", err)
	}
	if _, err = service.changeOpsConfig(ctx, tx, regular, "scratch", source.ID, "clone", OpsEditableConfig{}); !errors.Is(err, platform.ErrCatalogForbidden) {
		t.Fatal("regular config", err)
	}
	updated, err := service.saveOpsGame(ctx, tx, admin, "scratch", input)
	if err != nil || updated.Title != input.Title || updated.ConfiguredRuntimeState != "MAINTENANCE" {
		t.Fatal("direct settings", updated, err)
	}
	draft, err := service.changeOpsConfig(ctx, tx, admin, "scratch", source.ID, "clone", OpsEditableConfig{})
	if err != nil {
		t.Fatal("clone", err)
	}
	if draft.Status != "DRAFT" || draft.EditableConfig == nil || draft.ConfigVersionID == source.ID {
		t.Fatal("new draft", draft)
	}
	if _, err = service.changeOpsConfig(ctx, tx, admin, "scratch", draft.ConfigVersionID, "activate", OpsEditableConfig{}); !errors.Is(err, platform.ErrOpsConflict) {
		t.Fatal("unverified activation", err)
	}
	editable := *draft.EditableConfig
	editable.PrizeTableVersion = "scratch-ops-local"
	weight, _ := strconv.ParseInt(editable.Prizes[0].Weight, 10, 64)
	editable.Prizes[0].Weight = strconv.FormatInt(weight-1, 10)
	weight, _ = strconv.ParseInt(editable.Prizes[1].Weight, 10, 64)
	editable.Prizes[1].Weight = strconv.FormatInt(weight+1, 10)
	saved, err := service.changeOpsConfig(ctx, tx, admin, "scratch", draft.ConfigVersionID, "save", editable)
	if err != nil || saved.ConfigHash == draft.ConfigHash {
		t.Fatal("save weights", err)
	}
	validated, err := service.changeOpsConfig(ctx, tx, admin, "scratch", draft.ConfigVersionID, "validate", OpsEditableConfig{})
	if err != nil || validated.Status != "VALIDATED" {
		t.Fatal("real math verification", err)
	}
	if _, err = service.changeOpsConfig(ctx, tx, admin, "scratch", draft.ConfigVersionID, "save", editable); !errors.Is(err, platform.ErrOpsConflict) {
		t.Fatal("validated config mutation", err)
	}
	active, err := service.changeOpsConfig(ctx, tx, admin, "scratch", draft.ConfigVersionID, "activate", OpsEditableConfig{})
	if err != nil || active.Status != "ACTIVE" {
		t.Fatal("direct activation", err)
	}
	if active.PreviewedAt == nil || active.ActivatedAt == nil {
		t.Fatal("existing lifecycle timestamps")
	}
	if _, err = service.changeOpsConfig(ctx, tx, admin, "scratch", draft.ConfigVersionID, "activate", OpsEditableConfig{}); !errors.Is(err, platform.ErrOpsConflict) {
		t.Fatal("duplicate activation", err)
	}
	old, err := loadOpsConfigRecord(ctx, tx, "scratch", source.ID, false)
	if err != nil || old.Status != "SUPERSEDED" || string(old.Canonical) != string(source.Canonical) {
		t.Fatal("old binding immutable", err)
	}
	current, err := loadOpsRegistryState(ctx, tx, "scratch", false)
	if err != nil || *current.ActiveConfigVersionID != draft.ConfigVersionID {
		t.Fatal("active pointer", err)
	}
	var audits int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log WHERE actor_user_id=$1 AND (target=$2 OR (target='scratch' AND before_state->>'version'=$3))`, admin, draft.ConfigVersionID, before.Version).Scan(&audits); err != nil || audits != 5 {
		t.Fatal("same transaction audit", audits, err)
	}
	var originalPrize, newPrize string
	if err = tx.QueryRow(ctx, `SELECT before_state->'editable_config'->>'prize_table_version',after_state->'editable_config'->>'prize_table_version' FROM ops.audit_log WHERE target=$1 AND action='GAME_CONFIG_SAVE'`, draft.ConfigVersionID).Scan(&originalPrize, &newPrize); err != nil || originalPrize != draft.EditableConfig.PrizeTableVersion || newPrize != editable.PrizeTableVersion {
		t.Fatal("before/after prizes", err)
	}
	// A completed real round must never be refunded again, even without a client version.
	var roundID, slug string
	if err = tx.QueryRow(ctx, `SELECT round_id::text,game_slug FROM roulette.rounds WHERE state='FINISHED' LIMIT 1`).Scan(&roundID, &slug); err != nil {
		t.Fatal(err)
	}
	if err = roulette.SystemVoidInTx(ctx, tx, regular, slug, roundID); !errors.Is(err, platform.ErrCatalogForbidden) {
		t.Fatal("regular refund", err)
	}
	if err = roulette.SystemVoidInTx(ctx, tx, admin, slug, roundID); !errors.Is(err, platform.ErrOpsConflict) {
		t.Fatal("terminal refund", err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var remains int
	if err = store.Pool().QueryRow(ctx, `SELECT (SELECT count(*) FROM games.game_config_versions WHERE config_version_id=$1::uuid)+(SELECT count(*) FROM games.game_validation_artifacts WHERE config_version_id=$1::uuid)+(SELECT count(*) FROM ops.audit_log WHERE target=$1::text)`, draft.ConfigVersionID).Scan(&remains); err != nil || remains != 0 {
		t.Fatal("rollback", remains, err)
	}
	originalVersion, _ := strconv.ParseInt(before.Version, 10, 64)
	restored, err := service.ReadOpsGame(ctx, "scratch", OpsConfigFilter{})
	if err != nil || restored.Game.Version != originalVersion || restored.Game.Title != before.Title || restored.Game.ActiveConfig.ConfigVersionID != source.ID {
		t.Fatal("registry restored", err)
	}
	t.Log("LOCAL_GAMES_TX: actor_role=10; settings/clone/save/verify/activate; previous binding immutable; audits=5; terminal refund rejected; rollback configs/artifacts/audit=0; registry restored")
}
