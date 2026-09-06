package platform

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCatalogOpsSameSubjectReplayAndDifferentSubjectConflict(t *testing.T) {
	s, p, model := catalogOpsFixture(t)
	ctx := context.Background()
	metadata := catalogEditableMetadata()
	c := catalogCommand(t, s, p, model, "SAVE")
	c.Metadata = &metadata
	preview, err := s.PrepareCatalog(ctx, p.UserID, c, nil, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	type reply struct {
		result CatalogResult
		err    error
	}
	done := make(chan reply, 2)
	for range 2 {
		go func() {
			result, err := s.ExecuteCatalog(ctx, p.UserID, c, preview.ID, true, nil, catalogTestPolicy)
			done <- reply{result, err}
		}()
	}
	a, b := <-done, <-done
	if a.err != nil || b.err != nil || a.result != b.result {
		t.Fatal("same subject did not receive one durable result", a.err, b.err)
	}
	var count int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM ops.admin_operations WHERE operation_id=$1`, c.OperationID).Scan(&count); err != nil || count != 1 {
		t.Fatal("operation produced multiple audit records", err)
	}
	otherUser := p.UserID + 4
	if err = s.seedAnnouncementPrincipal(ctx, otherUser, "SUPER_ADMIN", false); err != nil {
		t.Fatal(err)
	}
	other, _ := s.CatalogAuthority(ctx, otherUser)
	c.Epoch = other.Epoch
	if _, err = s.ExecuteCatalog(ctx, otherUser, c, preview.ID, true, nil, catalogTestPolicy); !errors.Is(err, ErrCatalogOperation) {
		t.Fatal("different subject replayed operation", err)
	}
	_, model, _ = s.OpsCatalogModel(ctx, p.UserID, model.ModelID, catalogTestPolicy)
	left := catalogCommand(t, s, p, model, "SAVE")
	leftMetadata := catalogEditableMetadata()
	leftMetadata.DisplayName = "Left version"
	left.Metadata = &leftMetadata
	right := catalogCommand(t, s, other, model, "SAVE")
	rightMetadata := catalogEditableMetadata()
	rightMetadata.DisplayName = "Right version"
	right.Metadata = &rightMetadata
	lp, err := s.PrepareCatalog(ctx, p.UserID, left, nil, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	rp, err := s.PrepareCatalog(ctx, otherUser, right, nil, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		result, err := s.ExecuteCatalog(ctx, p.UserID, left, lp.ID, true, nil, catalogTestPolicy)
		done <- reply{result, err}
	}()
	go func() {
		result, err := s.ExecuteCatalog(ctx, otherUser, right, rp.ID, true, nil, catalogTestPolicy)
		done <- reply{result, err}
	}()
	a, b = <-done, <-done
	if !((a.err == nil && errors.Is(b.err, ErrCatalogConflict)) || (b.err == nil && errors.Is(a.err, ErrCatalogConflict))) {
		t.Fatal("two editors overwrote one target version", a.err, b.err)
	}
}
func TestCatalogOpsRechecksScopeAfterPrincipalLockWait(t *testing.T) {
	s, p, model := catalogOpsFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	operator := p.UserID + 5
	if err := s.seedAnnouncementPrincipal(ctx, operator, "OPERATOR", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO ops.admin_principal_scopes(admin_principal_id,scope) SELECT admin_principal_id,'MODELS' FROM ops.admin_principals WHERE newapi_user_id=$1`, operator); err != nil {
		t.Fatal(err)
	}
	op, err := s.CatalogAuthority(ctx, operator)
	if err != nil {
		t.Fatal(err)
	}
	c := catalogCommand(t, s, op, model, "SAVE")
	metadata := catalogEditableMetadata()
	c.Metadata = &metadata
	preview, err := s.PrepareCatalog(ctx, operator, c, nil, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	revoker, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(revoker)
	if _, err = revoker.Exec(ctx, `SELECT admin_principal_id FROM ops.admin_principals WHERE newapi_user_id=$1 FOR UPDATE`, operator); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := s.ExecuteCatalog(ctx, operator, c, preview.ID, true, nil, catalogTestPolicy)
		done <- err
	}()
	waiting := false
	for !waiting && ctx.Err() == nil {
		if err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND wait_event='transactionid')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if !waiting {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if !waiting {
		t.Fatal("write never waited on principal lock")
	}
	if _, err = revoker.Exec(ctx, `DELETE FROM ops.admin_principal_scopes WHERE admin_principal_id=(SELECT admin_principal_id FROM ops.admin_principals WHERE newapi_user_id=$1) AND scope='MODELS'`, operator); err != nil {
		t.Fatal(err)
	}
	if err = revoker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, ErrCatalogForbidden) {
		t.Fatal("lock waiter used old scope snapshot", err)
	}
	_, after, err := s.OpsCatalogModel(ctx, p.UserID, model.ModelID, catalogTestPolicy)
	if err != nil || after.Version != model.Version {
		t.Fatal("revoked write changed target", err)
	}
}
func TestCatalogOpsSyncBindsPreviewAndRetainsFailedReceipt(t *testing.T) {
	s, p, model := catalogOpsFixture(t)
	ctx := context.Background()
	status, err := s.CatalogSyncStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c := CatalogCommand{OperationID: announcementID(t), Epoch: p.Epoch, Action: "SYNC", ExpectedCatalogVersion: status.Version, Reason: "Synthetic explicit source sync"}
	source := catalogStoreSnapshot(t, model.ModelID, "new/"+announcementID(t))
	read := func(context.Context) (NativeCatalog, error) { return source, nil }
	preview, err := s.PrepareCatalog(ctx, p.UserID, c, read, catalogTestPolicy)
	if err != nil || preview.Impact.NewModels != 1 || preview.Impact.ObservedCount != 2 {
		t.Fatal("sync preview did not describe real source impact", err)
	}
	changed := catalogStoreSnapshot(t, "different/"+announcementID(t))
	if _, err = s.ExecuteCatalog(ctx, p.UserID, c, preview.ID, true, func(context.Context) (NativeCatalog, error) { return changed, nil }, catalogTestPolicy); !errors.Is(err, ErrCatalogSourceChanged) {
		t.Fatal("different native snapshot committed under old preview", err)
	}
	result, err := s.ExecuteCatalog(ctx, p.UserID, c, preview.ID, true, read, catalogTestPolicy)
	if err != nil || result.Sync == nil || result.Sync.Status != "VERIFIED" {
		t.Fatal("explicit sync failed", err)
	}
	_, err = s.ExecuteCatalog(ctx, p.UserID, c, "", false, func(context.Context) (NativeCatalog, error) {
		t.Fatal("confirmed sync re-read native provider")
		return NativeCatalog{}, nil
	}, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	status, _ = s.CatalogSyncStatus(ctx)
	c.OperationID = announcementID(t)
	c.ExpectedCatalogVersion = status.Version
	preview, err = s.PrepareCatalog(ctx, p.UserID, c, read, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := s.ExecuteCatalog(ctx, p.UserID, c, preview.ID, true, func(context.Context) (NativeCatalog, error) {
		return NativeCatalog{}, errors.New("SECRET_PROVIDER_ERROR")
	}, catalogTestPolicy)
	if err != nil || failed.Sync == nil || failed.Sync.Status != "FAILED" || failed.Sync.FailureCode != "CATALOG_READ_FAILED" {
		t.Fatal("source failure has no durable sanitized receipt", err)
	}
	current, _ := s.CatalogSyncStatus(ctx)
	if current.SourceHash != source.Hash || current.Version != status.Version {
		t.Fatal("failed explicit sync replaced last-good")
	}
}
func TestCatalogOpsCommitFailureDoesNotPartiallyPublishOrAudit(t *testing.T) {
	s, p, model := catalogOpsFixture(t)
	ctx := context.Background()
	metadata := catalogEditableMetadata()
	save := catalogCommand(t, s, p, model, "SAVE")
	save.Metadata = &metadata
	catalogConfirm(t, s, p, save)
	_, model, _ = s.OpsCatalogModel(ctx, p.UserID, model.ModelID, catalogTestPolicy)
	c := catalogCommand(t, s, p, model, "PUBLISH")
	preview, err := s.PrepareCatalog(ctx, p.UserID, c, nil, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `CREATE FUNCTION catalog.test_reject_ops_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic Ops commit failure'; END $$; CREATE CONSTRAINT TRIGGER test_ops_commit_failure AFTER INSERT ON ops.admin_operations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION catalog.test_reject_ops_commit()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := s.pool.Exec(context.Background(), `DROP TRIGGER test_ops_commit_failure ON ops.admin_operations; DROP FUNCTION catalog.test_reject_ops_commit()`); err != nil {
			t.Error(err)
		}
	})
	if _, err = s.ExecuteCatalog(ctx, p.UserID, c, preview.ID, true, nil, catalogTestPolicy); err == nil {
		t.Fatal("commit failure was hidden")
	}
	if _, err = s.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy); !errors.Is(err, ErrCatalogNotFound) {
		t.Fatal("failed commit partially published", err)
	}
	var audit bool
	if err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ops.admin_operations WHERE operation_id=$1)`, c.OperationID).Scan(&audit); err != nil || audit {
		t.Fatal("failed commit partially audited", err)
	}
}
func TestCatalogOpsRunsUnderRuntimeGrants(t *testing.T) {
	owner, p, model := catalogOpsFixture(t)
	runtimeStore := catalogRuntimeStore(t, owner)
	ctx := context.Background()
	metadata := catalogEditableMetadata()
	save := catalogCommand(t, owner, p, model, "SAVE")
	save.Metadata = &metadata
	catalogConfirm(t, runtimeStore, p, save)
	_, model, err := runtimeStore.OpsCatalogModel(ctx, p.UserID, model.ModelID, catalogTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	catalogConfirm(t, runtimeStore, p, catalogCommand(t, owner, p, model, "PUBLISH"))
	if _, err = runtimeStore.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy); err != nil {
		t.Fatal("runtime public projection failed", err)
	}
}
