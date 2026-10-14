package platform

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

// One main workflow, reusing imported dev data; all writes are rolled back.
func TestLocalRelayOps(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires existing dev database")
	}
	dsn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	s, err := Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev database")
	}
	defer s.Close()
	ctx := t.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var admin, regular, id int64
	var audits int
	if err = tx.QueryRow(ctx, `SELECT min(id) FILTER(WHERE role>=10),min(id) FILTER(WHERE role<10) FROM identity.users WHERE status=1 AND deleted_at IS NULL`).Scan(&admin, &regular); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM ops.audit_log`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM relay.channels WHERE enabled AND cardinality(models)>0 ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	channel, err := readOpsChannel(ctx, tx, id, false)
	if err != nil {
		t.Fatal(err)
	}
	var originalKey, advanced string
	if err = tx.QueryRow(ctx, `SELECT key,jsonb_build_array(param_override,channel_info,setting,settings,other)::text FROM relay.channels WHERE id=$1`, id).Scan(&originalKey, &advanced); err != nil {
		t.Fatal(err)
	}
	channel.Name += " edited"
	channel.Remark = "ops round-two test"
	channel.TestModel = channel.Models[0]
	edit := ChannelWrite{ChannelSettings: channel.ChannelSettings}
	if err = ValidateChannel(&edit); err != nil {
		t.Fatal(err)
	}
	saved, err := saveOpsChannel(ctx, tx, admin, edit)
	if err != nil {
		t.Fatal("channel edit", err)
	}
	var keptKey, keptAdvanced string
	if err = tx.QueryRow(ctx, `SELECT key,jsonb_build_array(param_override,channel_info,setting,settings,other)::text FROM relay.channels WHERE id=$1`, id).Scan(&keptKey, &keptAdvanced); err != nil {
		t.Fatal(err)
	}
	if originalKey != keptKey || advanced != keptAdvanced || saved.Name != edit.Name || saved.Remark != edit.Remark || saved.TestModel != edit.TestModel || !saved.KeyConfigured {
		t.Fatal("blank key or advanced configuration changed")
	}
	page, err := listOpsChannels(ctx, tx, OpsRelayFilter{Page: 1, PageSize: 1, Search: edit.Name, Type: int64(edit.Type)})
	if err != nil || page.Total != 1 || len(page.Items) != 1 || !page.Items[0].KeyConfigured {
		t.Fatal("filtered local list", err)
	}
	var calls, failures int64
	if err = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE kind=5 OR status_code>=400) FROM relay.logs WHERE channel_id=$1 AND kind IN(2,5) AND created_at>=now()-interval '24 hours'`, id).Scan(&calls, &failures); err != nil {
		t.Fatal(err)
	}
	if page.Items[0].Requests != calls || page.Items[0].Errors != failures {
		t.Fatal("24 hour channel statistics")
	}
	otherType := int64(1)
	if edit.Type == 1 {
		otherType = 14
	}
	wrongType, err := listOpsChannels(ctx, tx, OpsRelayFilter{Page: 1, PageSize: 1, Search: edit.Name, Type: otherType})
	if err != nil || wrongType.Total != 0 {
		t.Fatal("channel type filter", err)
	}
	priority, noModels := int64(7), []string{}
	partial := ChannelPatch{Priority: &priority, Models: &noModels}
	if err = ValidateChannelPatch(partial); err != nil {
		t.Fatal(err)
	}
	patched, err := patchOpsChannel(ctx, tx, admin, id, partial)
	if err != nil || patched.Priority != 7 || len(patched.Models) != 0 || patched.TestModel != "" || patched.Remark != saved.Remark || patched.Weight != saved.Weight || patched.Name != saved.Name {
		t.Fatal("partial edit overwrote other settings or left a removed default model", err)
	}
	encoded, _ := json.Marshal(saved)
	if strings.Contains(string(encoded), originalKey) || strings.Contains(string(encoded), `"key":`) {
		t.Fatal("secret in channel DTO")
	}
	group := GroupWrite{OpsGroup: OpsGroup{Name: "ops-regression", Description: "transactional main workflow", Ratio: "1.25"}}
	if err = ValidateGroup(&group); err != nil {
		t.Fatal(err)
	}
	if err = saveOpsGroup(ctx, tx, admin, group); err != nil {
		t.Fatal("create group", err)
	}
	create := edit
	create.ID = 0
	create.CopyFrom = id
	create.Name = "ops-regression-channel"
	create.Groups = []string{group.Name}
	create.Key = ""
	if err = ValidateChannel(&create); err != nil {
		t.Fatal("copy without reading a secret into the form", err)
	}
	created, err := saveOpsChannel(ctx, tx, admin, create)
	if err != nil {
		t.Fatal("create channel", err)
	}
	if err = deleteOpsGroup(ctx, tx, admin, group.Name); err != ErrRelayGroupInUse {
		t.Fatal("referenced group deletion", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET group_name=$2 WHERE id=$1`, regular, group.Name); err != nil {
		t.Fatal(err)
	}
	group.OriginalName = group.Name
	group.Name += "-renamed"
	group.Ratio = "0.000000000000000002"
	if err = saveOpsGroup(ctx, tx, admin, group); err != nil {
		t.Fatal("rename group", err)
	}
	var userGroup, ratio string
	if err = tx.QueryRow(ctx, `SELECT u.group_name,g.ratio::text FROM identity.users u JOIN relay.groups g ON g.name=u.group_name WHERE u.id=$1`, regular).Scan(&userGroup, &ratio); err != nil {
		t.Fatal(err)
	}
	if userGroup != group.Name || CompareCatalogDecimal(ratio, group.Ratio) != 0 {
		t.Fatal("rename or exact multiplier lost")
	}
	created, err = readOpsChannel(ctx, tx, created.ID, false)
	if err != nil || !slices.Contains(created.Groups, group.Name) {
		t.Fatal("channel group rename", err)
	}
	model := created.Models[0]
	route, err := selectRelayChannel(ctx, tx, group.Name, model, nil)
	if err != nil || route.ID != created.ID {
		t.Fatal("new channel not immediately routable", err)
	}
	matrix, err := relayAvailability(ctx, tx, OpsRelayFilter{Page: 1, PageSize: 100, Search: model})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range matrix.Items {
		if row.Model == model {
			for _, c := range row.Channels {
				if c.ID == created.ID && c.Enabled && slices.Contains(c.Groups, group.Name) {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("availability matrix differs from routing")
	}
	disabled := false
	if err = changeOpsChannel(ctx, tx, admin, created.ID, &disabled); err != nil {
		t.Fatal(err)
	}
	if _, err = selectRelayChannel(ctx, tx, group.Name, model, nil); err != ErrRelayChannel {
		t.Fatal("disabled channel still routed", err)
	}
	created.Enabled = true
	created.Groups = []string{}
	if _, err = saveOpsChannel(ctx, tx, admin, ChannelWrite{ChannelSettings: created.ChannelSettings}); err != nil {
		t.Fatal(err)
	}
	if _, err = selectRelayChannel(ctx, tx, group.Name, model, nil); err != ErrRelayChannel {
		t.Fatal("unassigned group still routed", err)
	}
	if err = deleteOpsGroup(ctx, tx, admin, group.Name); err != ErrRelayGroupInUse {
		t.Fatal("user reference lost", err)
	}
	if err = changeOpsChannel(ctx, tx, admin, created.ID, nil); err != nil {
		t.Fatal("delete channel", err)
	}
	if _, err = readOpsChannel(ctx, tx, created.ID, false); err != ErrOpsNotFound {
		t.Fatal("deleted channel still visible", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET group_name='default' WHERE id=$1`, regular); err != nil {
		t.Fatal(err)
	}
	if err = deleteOpsGroup(ctx, tx, admin, group.Name); err != nil {
		t.Fatal("delete unused group", err)
	}
	var count int
	var leaked bool
	if err = tx.QueryRow(ctx, `SELECT count(*),coalesce(bool_or(position($1 in before_state::text)>0 OR position($1 in after_state::text)>0),false) FROM ops.audit_log`, originalKey).Scan(&count, &leaked); err != nil {
		t.Fatal(err)
	}
	if count != audits+9 || leaked {
		t.Fatal("audit count or secret exposure", count-audits, leaked)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("channel fields/type filter/partial edit/create/disable/delete; blank key and imported settings preserved; group lifecycle + live routing + 24h metrics; 9 secret-free audits; all writes rolled back")
}
