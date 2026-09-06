package platform

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrMigrationNoticeStale = errors.New("MIGRATION_NOTICE_VERSION_STALE")

type MigrationNotice struct {
	UserID              int64      `json:"user_id,string"`
	State               string     `json:"state"`
	RequiredVersion     int64      `json:"required_migration_version,string"`
	AcknowledgedVersion int64      `json:"acknowledged_migration_version,string"`
	AcknowledgedAt      *time.Time `json:"acknowledged_at"`
	Title               string     `json:"title"`
	Body                string     `json:"body"`
	CompletedAt         *time.Time `json:"completed_at"`
}

func readMigrationNotice(ctx context.Context, q announcementQuerier, user int64, noneDeclared bool) (MigrationNotice, error) {
	return readMigrationNoticeVersion(ctx, q, user, 0, noneDeclared)
}

func readMigrationNoticeVersion(ctx context.Context, q announcementQuerier, user, version int64, noneDeclared bool) (MigrationNotice, error) {
	n := MigrationNotice{UserID: user, State: "UNVERIFIED"}
	if user <= 0 {
		return n, ErrInvalidProfile
	}
	err := q.QueryRow(ctx, `SELECT v.version,v.title,v.body,v.completed_at,a.acknowledged_at FROM identity.migration_notice_requirements r
 JOIN identity.migration_notice_versions v ON v.version=r.version
 LEFT JOIN identity.migration_notice_acknowledgements a ON a.newapi_user_id=r.newapi_user_id AND a.version=r.version
 WHERE r.newapi_user_id=$1 AND ($2::bigint=0 OR r.version=$2) ORDER BY r.version DESC LIMIT 1`, user, version).Scan(&n.RequiredVersion, &n.Title, &n.Body, &n.CompletedAt, &n.AcknowledgedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		if noneDeclared {
			n.State = "NOT_REQUIRED"
		}
		return n, nil
	}
	if err != nil {
		return n, err
	}
	n.State = "REQUIRED"
	if n.AcknowledgedAt != nil {
		n.State = "ACKNOWLEDGED"
		n.AcknowledgedVersion = n.RequiredVersion
	}
	return n, nil
}
func (s *Store) ReadMigrationNotice(ctx context.Context, user int64, noneDeclared bool) (MigrationNotice, error) {
	return readMigrationNotice(ctx, s.pool, user, noneDeclared)
}
func (s *Store) AcknowledgeMigrationNotice(ctx context.Context, user, version int64) (MigrationNotice, error) {
	if user <= 0 || version <= 0 {
		return MigrationNotice{}, ErrMigrationNoticeStale
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return MigrationNotice{}, err
	}
	defer rollback(tx)
	n, err := readMigrationNoticeVersion(ctx, tx, user, version, false)
	if err != nil {
		return n, err
	}
	if n.RequiredVersion != version {
		return n, ErrMigrationNoticeStale
	}
	_, err = tx.Exec(ctx, `INSERT INTO identity.migration_notice_acknowledgements(newapi_user_id,version) VALUES($1,$2) ON CONFLICT(newapi_user_id,version) DO NOTHING`, user, version)
	if err != nil {
		return n, err
	}
	// An old-version retry returns its original ACK even if a later version
	// has become required. The next Gate read still selects the newest version.
	n, err = readMigrationNoticeVersion(ctx, tx, user, version, false)
	if err != nil {
		return n, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MigrationNotice{}, err
	}
	return n, nil
}
