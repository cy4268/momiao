package platform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func catalogStoreSnapshot(t *testing.T, ids ...string) NativeCatalog {
	t.Helper()
	var data map[string]any
	_ = json.Unmarshal([]byte(catalogTestData()), &data)
	original := data["models"].([]any)[0].(map[string]any)
	models := []any{}
	slices.Sort(ids)
	for _, id := range ids {
		copy := map[string]any{}
		for k, v := range original {
			copy[k] = v
		}
		copy["model_id"] = id
		models = append(models, copy)
	}
	data["models"] = models
	raw, _ := json.Marshal(data)
	now := time.Now().UTC()
	envelope := []byte(fmt.Sprintf(`{"success":true,"complete":true,"observed_at":%q,"content_hash":"sha256:%x","data":%s}`, now.Format(time.RFC3339Nano), sha256.Sum256(raw), raw))
	result, err := ParseNativeCatalog(envelope, now)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func catalogReparseSnapshot(t *testing.T, source NativeCatalog) NativeCatalog {
	t.Helper()
	data := nativeCatalogData{Schema: NativeCatalogSchema, Basis: "public_default_reference", BillingAuthority: "native_settlement", Notices: catalogNotices, Models: source.Models}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	envelope := []byte(fmt.Sprintf(`{"success":true,"complete":true,"observed_at":%q,"content_hash":"sha256:%x","data":%s}`, now.Format(time.RFC3339Nano), sha256.Sum256(raw), raw))
	result, err := ParseNativeCatalog(envelope, now)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestCatalogSyncAtomicLastGoodAndHashIdentity(t *testing.T) {
	s := catalogTestStore(t)
	ctx := context.Background()
	id := "sync/" + announcementID(t)
	snapshot := catalogStoreSnapshot(t, id)
	source := func(context.Context) (NativeCatalog, error) { return snapshot, nil }
	first, err := s.SyncCatalog(ctx, source)
	if err != nil || first.Status != "VERIFIED" || !first.Changed || first.ObservedCount != 1 {
		t.Fatal("first complete batch not applied", first, err)
	}
	var publication, availability string
	var metadataVersion, identityCount int64
	if err = s.pool.QueryRow(ctx, `SELECT p.publication_state,a.availability_state,m.metadata_version,(SELECT count(*) FROM catalog.historical_model_identity h WHERE h.model_id=m.model_id) FROM catalog.model_catalog_metadata m JOIN catalog.model_catalog_publication p USING(model_id) JOIN catalog.model_availability_mappings a USING(model_id) WHERE model_id=$1`, id).Scan(&publication, &availability, &metadataVersion, &identityCount); err != nil {
		t.Fatal(err)
	}
	if publication != "PENDING_METADATA" || availability != "CONFIGURED" || metadataVersion != 1 || identityCount != 1 {
		t.Fatal("source invented editorial state", publication, availability, metadataVersion, identityCount)
	}
	status, err := s.CatalogSyncStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.VerifiedAt = time.Now().UTC()
	snapshot.ObservedAt = snapshot.VerifiedAt
	again, err := s.SyncCatalog(ctx, source)
	if err != nil || again.Changed || again.SnapshotID != first.SnapshotID || again.AttemptID == first.AttemptID {
		t.Fatal("repeat hash did not preserve stable snapshot identity", again, err)
	}
	repeated, _ := s.CatalogSyncStatus(ctx)
	if repeated.Version != status.Version || repeated.LastVerifiedAt.Before(*status.LastVerifiedAt) {
		t.Fatal("repeat hash version/freshness invalid")
	}
	failed, err := s.SyncCatalog(ctx, func(context.Context) (NativeCatalog, error) {
		return NativeCatalog{}, errors.New("SECRET_INTERNAL_URL")
	})
	if err != nil || failed.Status != "FAILED" || failed.FailureCode != "CATALOG_READ_FAILED" || strings.Contains(fmt.Sprint(failed), "SECRET") {
		t.Fatal("failure should be durable and sanitized", failed, err)
	}
	after, _ := s.CatalogSyncStatus(ctx)
	if after.Version != status.Version || after.SnapshotID != first.SnapshotID || !after.LastVerifiedAt.Equal(*repeated.LastVerifiedAt) || after.LastAttemptStatus != "FAILED" {
		t.Fatal("failure replaced last-good", after)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE catalog.model_catalog_publication SET publication_state='PUBLISHED',published_at=now() WHERE model_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	empty := catalogStoreSnapshot(t)
	if _, err = s.SyncCatalog(ctx, func(context.Context) (NativeCatalog, error) { return empty, nil }); err != nil {
		t.Fatal(err)
	}
	var facts []byte
	if err = s.pool.QueryRow(ctx, `SELECT p.publication_state,a.availability_state,a.source_facts,m.metadata_version,(SELECT count(*) FROM catalog.historical_model_identity h WHERE h.model_id=m.model_id) FROM catalog.model_catalog_metadata m JOIN catalog.model_catalog_publication p USING(model_id) JOIN catalog.model_availability_mappings a USING(model_id) WHERE model_id=$1`, id).Scan(&publication, &availability, &facts, &metadataVersion, &identityCount); err != nil {
		t.Fatal(err)
	}
	if publication != "PUBLISHED" || availability != "NOT_OBSERVED" || metadataVersion != 1 || identityCount != 1 || !strings.Contains(string(facts), "0.00000000000000000002") {
		t.Fatal("empty success deleted publication/history/last price")
	}
	restarted, err := Open(ctx, osCatalogDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	persisted, err := restarted.CatalogSyncStatus(ctx)
	if err != nil || persisted.LastAttemptStatus != "VERIFIED" || persisted.ObservedCount != 0 {
		t.Fatal("restart lost complete empty snapshot", persisted, err)
	}
}
func TestCatalogSyncDatabaseCommitFailureRollsBackWholeBatch(t *testing.T) {
	s := catalogTestStore(t)
	ctx := context.Background()
	before, err := s.CatalogSyncStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var attempts int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM catalog.model_sync_attempts`).Scan(&attempts)
	_, err = s.pool.Exec(ctx, `CREATE FUNCTION catalog.test_reject_sync_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic commit failure'; END $$; CREATE CONSTRAINT TRIGGER test_sync_commit_failure AFTER INSERT ON catalog.model_sync_attempts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION catalog.test_reject_sync_commit()`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := s.pool.Exec(context.Background(), `DROP TRIGGER test_sync_commit_failure ON catalog.model_sync_attempts; DROP FUNCTION catalog.test_reject_sync_commit()`); err != nil {
			t.Error(err)
		}
	})
	id := "rollback/" + announcementID(t)
	snapshot := catalogStoreSnapshot(t, id)
	if _, err = s.SyncCatalog(ctx, func(context.Context) (NativeCatalog, error) { return snapshot, nil }); err == nil {
		t.Fatal("deferred commit failure hidden")
	}
	after, err := s.CatalogSyncStatus(ctx)
	if err != nil || after.SnapshotID != before.SnapshotID || after.Version != before.Version {
		t.Fatal("failed commit replaced source state", err)
	}
	var leaked bool
	_ = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM catalog.model_catalog_metadata WHERE model_id=$1)`, id).Scan(&leaked)
	var afterAttempts int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM catalog.model_sync_attempts`).Scan(&afterAttempts)
	if leaked || attempts != afterAttempts {
		t.Fatal("partial identity or attempt committed")
	}
}
func TestCatalogSyncSerializesAcrossStores(t *testing.T) {
	s := catalogTestStore(t)
	second, err := Open(context.Background(), osCatalogDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	secondRead := make(chan struct{}, 1)
	done := make(chan error, 2)
	firstSource := catalogStoreSnapshot(t, "serial-a/"+announcementID(t))
	secondSource := catalogStoreSnapshot(t, "serial-b/"+announcementID(t))
	go func() {
		_, err := s.SyncCatalog(ctx, func(ctx context.Context) (NativeCatalog, error) {
			close(entered)
			select {
			case <-release:
				return firstSource, nil
			case <-ctx.Done():
				return NativeCatalog{}, ctx.Err()
			}
		})
		done <- err
	}()
	<-entered
	go func() {
		_, err := second.SyncCatalog(ctx, func(context.Context) (NativeCatalog, error) { secondRead <- struct{}{}; return secondSource, nil })
		done <- err
	}()
	waiting := false
	for !waiting && ctx.Err() == nil {
		if err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND wait_event='advisory')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if !waiting {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if !waiting {
		t.Fatal("second store did not wait on database serialization")
	}
	select {
	case <-secondRead:
		t.Fatal("concurrent native read bypassed database serialization")
	default:
	}
	close(release)
	for range 2 {
		if err = <-done; err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-secondRead:
	default:
		t.Fatal("second sync never resumed")
	}
	final, err := s.CatalogSyncStatus(ctx)
	if err != nil || final.SourceHash != secondSource.Hash {
		t.Fatal("older snapshot won after serialized reads", err)
	}
}
