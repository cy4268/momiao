package platform

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type CatalogSource func(context.Context) (NativeCatalog, error)
type CatalogSyncResult struct {
	AttemptID     string `json:"attempt_id"`
	SnapshotID    string `json:"snapshot_id,omitempty"`
	Status        string `json:"status"`
	Changed       bool   `json:"changed"`
	ObservedCount int    `json:"observed_count"`
	FailureCode   string `json:"failure_code,omitempty"`
}
type CatalogSyncStatus struct {
	SnapshotID        string     `json:"snapshot_id"`
	SourceHash        string     `json:"source_hash"`
	Version           int64      `json:"version,string"`
	ObservedCount     int        `json:"observed_count"`
	LastObservedAt    *time.Time `json:"last_observed_at"`
	LastVerifiedAt    *time.Time `json:"last_verified_at"`
	LastAttemptAt     *time.Time `json:"last_attempt_at"`
	LastAttemptStatus string     `json:"last_attempt_status"`
	FailureCode       string     `json:"failure_code,omitempty"`
}

func catalogSyncStatus(ctx context.Context, q announcementQuerier) (CatalogSyncStatus, error) {
	var status CatalogSyncStatus
	err := q.QueryRow(ctx, `SELECT COALESCE(s.source_snapshot_id::text,''),COALESCE('sha256:'||encode(n.source_hash,'hex'),''),s.version,COALESCE(n.observed_model_count,0),s.last_observed_at,s.last_verified_at,a.created_at,COALESCE(a.status,'NEVER_SYNCED'),COALESCE(a.failure_code,'')
 FROM catalog.model_sync_state s LEFT JOIN catalog.model_sync_snapshots n ON n.sync_snapshot_id=s.source_snapshot_id LEFT JOIN catalog.model_sync_attempts a ON a.attempt_id=s.last_attempt_id WHERE s.singleton`).Scan(&status.SnapshotID, &status.SourceHash, &status.Version, &status.ObservedCount, &status.LastObservedAt, &status.LastVerifiedAt, &status.LastAttemptAt, &status.LastAttemptStatus, &status.FailureCode)
	return status, err
}
func (s *Store) CatalogSyncStatus(ctx context.Context) (CatalogSyncStatus, error) {
	return catalogSyncStatus(ctx, s.pool)
}
func lockCatalog(ctx context.Context, tx pgx.Tx) error {
	// ponytail: one DB lock serializes <=1000-model batches and editorial writes;
	// partition only if measured catalog write throughput requires it.
	return lockIdentity(ctx, tx, "catalog", "sync-publication-v1")
}
func readCatalogSource(ctx context.Context, read CatalogSource) (NativeCatalog, error) {
	if read == nil {
		return NativeCatalog{}, ErrCatalogSource
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	source, err := read(bounded)
	if err != nil {
		return NativeCatalog{}, err
	}
	hash, err := hex.DecodeString(strings.TrimPrefix(source.Hash, "sha256:"))
	if bounded.Err() != nil {
		return NativeCatalog{}, bounded.Err()
	}
	if !source.validated || len(source.Hash) != 71 || !strings.HasPrefix(source.Hash, "sha256:") || err != nil || len(hash) != 32 || source.ObservedAt.IsZero() || source.VerifiedAt.IsZero() || source.Models == nil || len(source.Models) > 1000 {
		return NativeCatalog{}, ErrCatalogSource
	}
	return source, nil
}
func (s *Store) SyncCatalog(parent context.Context, read CatalogSource) (CatalogSyncResult, error) {
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CatalogSyncResult{}, err
	}
	defer rollback(tx)
	if err = lockCatalog(ctx, tx); err != nil {
		return CatalogSyncResult{}, err
	}
	source, readErr := readCatalogSource(ctx, read)
	result, err := syncCatalogInTx(ctx, tx, source, readErr, "SCHEDULED")
	if err != nil {
		return CatalogSyncResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CatalogSyncResult{}, err
	}
	return result, nil
}
func syncCatalogInTx(ctx context.Context, tx pgx.Tx, source NativeCatalog, readErr error, trigger string) (CatalogSyncResult, error) {
	result := CatalogSyncResult{}
	var err error
	result.AttemptID, err = uuidV7()
	if err != nil {
		return result, err
	}
	if readErr != nil {
		result.Status = "FAILED"
		result.FailureCode = "CATALOG_READ_FAILED"
		if errors.Is(readErr, ErrCatalogSource) {
			result.FailureCode = "CATALOG_SOURCE_INVALID"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO catalog.model_sync_attempts(attempt_id,trigger_kind,status,failure_code) VALUES($1,$2,'FAILED',$3)`, result.AttemptID, trigger, result.FailureCode); err != nil {
			return result, err
		}
		_, err = tx.Exec(ctx, `UPDATE catalog.model_sync_state SET last_attempt_id=$1 WHERE singleton`, result.AttemptID)
		return result, err
	}
	current, err := catalogSyncStatus(ctx, tx)
	if err != nil {
		return result, err
	}
	raw, err := json.Marshal(source.Models)
	if err != nil {
		return result, err
	}
	hash, _ := hex.DecodeString(strings.TrimPrefix(source.Hash, "sha256:"))
	err = tx.QueryRow(ctx, `SELECT sync_snapshot_id::text FROM catalog.model_sync_snapshots WHERE source_hash=$1`, hash).Scan(&result.SnapshotID)
	if errors.Is(err, pgx.ErrNoRows) {
		result.SnapshotID, err = uuidV7()
		if err != nil {
			return result, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO catalog.model_sync_snapshots(sync_snapshot_id,source_identity,source_hash,observed_model_count,status,observed_at,source_models) VALUES($1,$2,$3,$4,'VERIFIED',$5,$6)`, result.SnapshotID, NativeCatalogSchema, hash, len(source.Models), source.ObservedAt, raw)
	}
	if err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, `INSERT INTO catalog.model_catalog_metadata(model_id)
 SELECT item->>'model_id' FROM jsonb_array_elements($1::jsonb) item ON CONFLICT DO NOTHING RETURNING model_id`, raw)
	if err != nil {
		return result, err
	}
	type newIdentity struct {
		ID      string `json:"id"`
		ModelID string `json:"model_id"`
	}
	newIdentities := []newIdentity{}
	newIDs := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		identityID, e := uuidV7()
		if e != nil {
			rows.Close()
			return result, e
		}
		newIdentities = append(newIdentities, newIdentity{identityID, id})
		newIDs = append(newIDs, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}
	if len(newIDs) > 0 {
		if _, err = tx.Exec(ctx, `INSERT INTO catalog.model_catalog_publication(model_id) SELECT unnest($1::text[])`, newIDs); err != nil {
			return result, err
		}
		identities, _ := json.Marshal(newIdentities)
		if _, err = tx.Exec(ctx, `INSERT INTO catalog.historical_model_identity(historical_identity_id,model_id,display_name_snapshot,family_snapshot,effective_from)
  SELECT id::uuid,model_id,model_id,'',$2 FROM jsonb_to_recordset($1::jsonb) AS x(id text,model_id text)`, identities, source.VerifiedAt); err != nil {
			return result, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO catalog.model_metadata_revisions(model_id,metadata_version,content)
  SELECT model_id,metadata_version,jsonb_build_object('display_name',display_name,'family',family,'summary',summary,'context_length',context_length,'metadata',metadata) FROM catalog.model_catalog_metadata WHERE model_id=ANY($1::text[])`, newIDs); err != nil {
			return result, err
		}
	}
	ids := make([]string, 0, len(source.Models))
	for _, model := range source.Models {
		ids = append(ids, model.ModelID)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO catalog.model_availability_mappings(model_id,availability_state,source_snapshot_id,observed_at,last_seen_at,source_facts)
 SELECT item->>'model_id',CASE WHEN (item->>'native_catalog_visible')::boolean THEN 'CONFIGURED' ELSE 'NATIVE_HIDDEN' END,$2,$3,$3,item FROM jsonb_array_elements($1::jsonb) item
 ON CONFLICT(model_id) DO UPDATE SET availability_state=excluded.availability_state,source_snapshot_id=excluded.source_snapshot_id,observed_at=excluded.observed_at,last_seen_at=excluded.last_seen_at,source_facts=excluded.source_facts`, raw, result.SnapshotID, source.ObservedAt); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `UPDATE catalog.model_availability_mappings SET availability_state='NOT_OBSERVED',source_snapshot_id=$2,observed_at=$3 WHERE NOT(model_id=ANY($1::text[]))`, ids, result.SnapshotID, source.ObservedAt); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO catalog.model_sync_attempts(attempt_id,trigger_kind,status,source_snapshot_id,observed_at,verified_at) VALUES($1,$2,'VERIFIED',$3,$4,$5)`, result.AttemptID, trigger, result.SnapshotID, source.ObservedAt, source.VerifiedAt); err != nil {
		return result, err
	}
	result.Changed = current.SourceHash != source.Hash
	if _, err = tx.Exec(ctx, `UPDATE catalog.model_sync_state SET source_snapshot_id=$1,last_attempt_id=$2,last_verified_at=$3,last_observed_at=$4,version=version+CASE WHEN $5 THEN 1 ELSE 0 END WHERE singleton`, result.SnapshotID, result.AttemptID, source.VerifiedAt, source.ObservedAt, result.Changed); err != nil {
		return result, err
	}
	result.Status = "VERIFIED"
	result.ObservedCount = len(source.Models)
	return result, nil
}
