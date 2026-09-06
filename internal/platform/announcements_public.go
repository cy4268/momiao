package platform

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Archive is an explicit historical projection; it never participates in delivery eligibility.
const announcementPublicWhere = ` a.withdrawn_at IS NULL AND a.first_published_at IS NOT NULL
 AND (c.visibility='PUBLIC' OR $1::bigint>0) AND a.publish_at<=now() AND a.visible_from<=now()
 AND ((NOT $2::boolean AND a.state='PUBLISHED' AND (a.visible_until IS NULL OR now()<a.visible_until))
 OR ($2::boolean AND (a.state IN ('EXPIRED','ARCHIVED') OR (a.state='PUBLISHED' AND a.visible_until<=now())))) `

func (s *Store) PublicAnnouncements(ctx context.Context, userID int64, f AnnouncementFilter) (AnnouncementPage, error) {
	page := AnnouncementPage{Items: []Announcement{}}
	if f.Limit == 0 {
		f.Limit = 20
	}
	if userID < 0 || f.Offset < 0 || f.Offset > 10000 || f.Limit < 1 || f.Limit > 50 || len(f.Search) > 200 || !utf8.ValidString(f.Search) || strings.ContainsRune(f.Search, 0) || f.Type != "" && !slices.Contains([]string{"SYSTEM", "NEW_MODELS", "GAME_EVENTS", "MAINTENANCE", "IMPORTANT", "ACKNOWLEDGEMENTS"}, f.Type) || f.Placement != "" && !slices.Contains([]string{"ENTRY_POPUP", "PUBLIC_HOME_BANNER", "POST_LOGIN_POPUP", "DASHBOARD_SUMMARY"}, f.Placement) || f.Archive && f.Placement != "" {
		return page, ErrAnnouncementInvalid
	}
	if f.Placement == "POST_LOGIN_POPUP" && userID <= 0 {
		return page, ErrAnnouncementForbidden
	}
	query := announcementSelect + " WHERE " + announcementPublicWhere + ` AND ($3='' OR c.type=$3) AND ($4='' OR strpos(lower(c.title||' '||c.body_markdown),lower($4))>0)
 AND ($5::timestamptz IS NULL OR a.publish_at >= $5) AND ($6::timestamptz IS NULL OR a.publish_at < $6)
 AND ($7='' OR EXISTS(SELECT 1 FROM content.announcement_placements p WHERE p.announcement_id=a.announcement_id AND p.placement=$7))
 AND ($7<>'POST_LOGIN_POPUP' OR NOT EXISTS(SELECT 1 FROM content.announcement_reads r WHERE r.newapi_user_id=$1 AND r.announcement_id=a.announcement_id AND r.notification_revision=a.notification_revision)) `
	if f.Placement == "POST_LOGIN_POPUP" {
		query += " ORDER BY a.visible_from ASC,a.announcement_id ASC"
	} else {
		query += ` ORDER BY CASE WHEN NOT $2 THEN (SELECT p.manual_order FROM content.announcement_placements p WHERE p.announcement_id=a.announcement_id AND p.placement='PINNED_LIST') END ASC NULLS LAST,a.publish_at DESC,a.announcement_id DESC`
	}
	query += " LIMIT $8 OFFSET $9"
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return page, err
	}
	defer rollback(tx)
	rows, err := tx.Query(ctx, query, userID, f.Archive, f.Type, f.Search, f.DateFrom, f.DateTo, f.Placement, f.Limit+1, f.Offset)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		a, err := scanOpsAnnouncement(rows)
		if err != nil {
			rows.Close()
			return page, err
		}
		if f.Archive && a.State == "PUBLISHED" {
			a.State = "EXPIRED"
		}
		page.Items = append(page.Items, a.Announcement)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > f.Limit {
		page.HasMore = true
		page.Items = page.Items[:f.Limit]
	}
	if userID > 0 {
		for i := range page.Items {
			a := &page.Items[i]
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM content.announcement_reads WHERE newapi_user_id=$1 AND announcement_id=$2 AND notification_revision=$3)`, userID, a.ID, a.NotificationRevision).Scan(&a.Read); err != nil {
				return page, err
			}
		}
	}
	return page, tx.Commit(ctx)
}
func (s *Store) PublicAnnouncement(ctx context.Context, userID int64, id string, archive bool) (Announcement, error) {
	if !announcementUUID(id) || userID < 0 {
		return Announcement{}, ErrAnnouncementNotFound
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Announcement{}, err
	}
	defer rollback(tx)
	a, err := scanOpsAnnouncement(tx.QueryRow(ctx, announcementSelect+" WHERE "+announcementPublicWhere+" AND a.announcement_id=$3", userID, archive, id))
	if err != nil {
		return Announcement{}, err
	}
	if archive && a.State == "PUBLISHED" {
		a.State = "EXPIRED"
	}
	if userID > 0 {
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM content.announcement_reads WHERE newapi_user_id=$1 AND announcement_id=$2 AND notification_revision=$3)`, userID, a.ID, a.NotificationRevision).Scan(&a.Read); err != nil {
			return Announcement{}, err
		}
	}
	return a.Announcement, tx.Commit(ctx)
}
func (s *Store) ReadAnnouncement(ctx context.Context, userID int64, id string, revision int64) (time.Time, error) {
	var at time.Time
	if userID <= 0 {
		return at, ErrAnnouncementForbidden
	}
	if !announcementUUID(id) || revision < 1 {
		return at, ErrAnnouncementInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return at, err
	}
	defer rollback(tx)
	var eligible string
	// SHARE serializes a visibility change with this explicit read without promoting the revision.
	err = tx.QueryRow(ctx, `SELECT a.announcement_id::text FROM content.announcements a JOIN content.announcement_revisions c ON (c.announcement_id,c.content_version)=(a.announcement_id,a.current_content_version)
 WHERE a.announcement_id=$2 AND a.withdrawn_at IS NULL AND a.first_published_at IS NOT NULL AND a.publish_at<=now() AND a.visible_from<=now()
 AND a.state IN ('PUBLISHED','EXPIRED','ARCHIVED') AND (c.visibility='PUBLIC' OR $1::bigint>0)
 AND EXISTS(SELECT 1 FROM content.notification_revisions n WHERE n.announcement_id=a.announcement_id AND n.notification_revision=$3)
 FOR SHARE OF a`, userID, id, revision).Scan(&eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		return at, ErrAnnouncementNotFound
	}
	if err != nil {
		return at, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO content.announcement_reads(newapi_user_id,announcement_id,notification_revision) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, userID, id, revision); err != nil {
		return at, err
	}
	if err = tx.QueryRow(ctx, `SELECT read_at FROM content.announcement_reads WHERE newapi_user_id=$1 AND announcement_id=$2 AND notification_revision=$3`, userID, id, revision).Scan(&at); err != nil {
		return at, err
	}
	return at, tx.Commit(ctx)
}

// Each job and lifecycle transition commit together. A crashed process releases its locks;
// the still-PENDING durable job is recoverable. No popup/read side effect is performed here.
func (s *Store) RunAnnouncementJobs(ctx context.Context) (int, error) {
	completed := 0
	for range 50 {
		did, err := s.runAnnouncementJob(ctx)
		if err != nil {
			return completed, err
		}
		if !did {
			return completed, nil
		}
		completed++
	}
	return completed, nil
}
func (s *Store) runAnnouncementJob(ctx context.Context) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollback(tx)
	var key, id string
	err = tx.QueryRow(ctx, `SELECT job_key,announcement_id::text FROM content.announcement_jobs WHERE status='PENDING' AND due_at<=now() ORDER BY due_at,job_key LIMIT 1`).Scan(&key, &id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// All writers lock target before job rows, avoiding a worker/execute lock-order inversion.
	var locked string
	err = tx.QueryRow(ctx, "SELECT announcement_id::text FROM content.announcements WHERE announcement_id=$1 FOR UPDATE SKIP LOCKED", id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	a, err := loadOpsAnnouncement(ctx, tx, id, false)
	if err != nil {
		return false, err
	}
	var kind string
	var cv, nr int64
	var due, now time.Time
	err = tx.QueryRow(ctx, `SELECT kind,content_version,notification_revision,due_at,clock_timestamp() FROM content.announcement_jobs WHERE job_key=$1 AND status='PENDING' AND due_at<=now() FOR UPDATE SKIP LOCKED`, key).Scan(&kind, &cv, &nr, &due, &now)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	status := "OBSOLETE"
	state := a.State
	reason := a.ExpiredReason
	if kind == "PUBLISH" && a.State == "SCHEDULED" && a.WithdrawnAt == nil && cv == a.ContentVersion && a.PublishAt != nil && !a.PublishAt.After(now) {
		status = "DONE"
		state = "PUBLISHED"
		if a.VisibleUntil != nil && !a.VisibleUntil.After(now) {
			state = "EXPIRED"
			reason = "MISSED_PUBLISH_WINDOW"
		}
	} else if kind == "EXPIRE" && a.State == "PUBLISHED" && nr == a.NotificationRevision && a.VisibleUntil != nil && !a.VisibleUntil.After(now) {
		status = "DONE"
		state = "EXPIRED"
		reason = "VISIBLE_WINDOW_ENDED"
	}
	if status == "DONE" {
		if _, err = tx.Exec(ctx, `UPDATE content.announcements SET state=$2,version=version+1,expired_reason=$3,first_published_at=CASE WHEN $2='PUBLISHED' THEN COALESCE(first_published_at,now()) ELSE first_published_at END,updated_at=now() WHERE announcement_id=$1`, id, state, reason); err != nil {
			return false, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE content.announcement_jobs SET status=$2,finished_at=now() WHERE job_key=$1`, key, status); err != nil {
		return false, err
	}
	op, err := uuidV7()
	if err != nil {
		return false, err
	}
	details := map[string]any{"job_key": key, "job_status": status, "state": state, "reason": reason}
	if _, err = tx.Exec(ctx, `INSERT INTO ops.admin_operations(operation_id,actor_kind,action,announcement_id,request_hash,details,result) VALUES($1,'SYSTEM',$2,$3,$4,$5,$5)`, op, "ANNOUNCEMENT_JOB_"+kind, id, announcementHash(fmt.Sprint(key, due)), details); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
