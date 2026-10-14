package platform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type announcementQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func announcementUUID(id string) bool {
	var u pgtype.UUID
	return len(id) == 36 && u.Scan(id) == nil && u.Valid
}
func announcementHash(v any) string {
	b, _ := json.Marshal(v)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func announcementDBError(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23505" {
		return ErrAnnouncementConflict
	}
	return err
}

const announcementSelect = `SELECT a.announcement_id::text,a.current_content_version,a.notification_revision,a.version,
 a.state,a.publish_at,a.visible_from,a.visible_until,a.updated_at,a.withdrawn_at,a.first_published_at,a.expired_reason,COALESCE(a.canonical_key,''),
 c.title,c.type,c.visibility,c.body_markdown,c.sanitized_html,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('placement',p.placement,'manual_order',p.manual_order) ORDER BY p.placement) FROM content.announcement_placements p WHERE p.announcement_id=a.announcement_id),'[]'::jsonb),
 COALESCE((SELECT jsonb_agg(jsonb_build_object('display_name',e.display_name,'external_link',e.external_link,'acknowledgement_note',e.acknowledgement_note,'group_name',e.group_name,'manual_order',e.manual_order,'anonymous',e.anonymous,'consent_attested',e.consent_attested) ORDER BY e.manual_order) FROM content.acknowledgement_entries e WHERE (e.announcement_id,e.content_version)=(a.announcement_id,a.current_content_version)),'[]'::jsonb)
 FROM content.announcements a JOIN content.announcement_revisions c ON (c.announcement_id,c.content_version)=(a.announcement_id,a.current_content_version) `

func scanOpsAnnouncement(row pgx.Row) (OpsAnnouncement, error) {
	var a OpsAnnouncement
	var placements, acks []byte
	err := row.Scan(&a.ID, &a.ContentVersion, &a.NotificationRevision, &a.Version, &a.State, &a.PublishAt, &a.VisibleFrom, &a.VisibleUntil, &a.UpdatedAt, &a.WithdrawnAt, &a.FirstPublishedAt, &a.ExpiredReason, &a.CanonicalKey, &a.Content.Title, &a.Content.Type, &a.Content.Visibility, &a.Content.Markdown, &a.HTML, &placements, &acks)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrAnnouncementNotFound
	}
	if err != nil {
		return a, err
	}
	if err = json.Unmarshal(placements, &a.Placements); err != nil {
		return a, err
	}
	if err = json.Unmarshal(acks, &a.Content.Acknowledgements); err != nil {
		return a, err
	}
	a.Title = a.Content.Title
	a.Type = a.Content.Type
	a.Acknowledgements = append([]Acknowledgement{}, a.Content.Acknowledgements...)
	for i := range a.Acknowledgements {
		a.Acknowledgements[i].ConsentAttested = false
	}
	for _, p := range a.Placements {
		if p.Placement == "PINNED_LIST" {
			a.Pinned = true
		}
	}
	return a, nil
}
func loadOpsAnnouncement(ctx context.Context, q announcementQuerier, id string, lock bool) (OpsAnnouncement, error) {
	if lock {
		var locked string
		if err := q.QueryRow(ctx, "SELECT announcement_id::text FROM content.announcements WHERE announcement_id=$1 FOR UPDATE", id).Scan(&locked); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				err = ErrAnnouncementNotFound
			}
			return OpsAnnouncement{}, err
		}
	}
	// The content pointer may change while waiting. Join revisions in a fresh statement
	// after locking the identity, so its immutable content is visible in this snapshot.
	return scanOpsAnnouncement(q.QueryRow(ctx, announcementSelect+" WHERE a.announcement_id=$1", id))
}
func (s *Store) OpsAnnouncements(ctx context.Context, userID int64) ([]OpsAnnouncement, error) {
	if err := catalogAdmin(ctx, s.pool, userID, false); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, announcementSelect+" ORDER BY a.updated_at DESC,a.announcement_id DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []OpsAnnouncement{}
	for rows.Next() {
		a, err := scanOpsAnnouncement(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
func (s *Store) OpsAnnouncement(ctx context.Context, userID int64, id string) (OpsAnnouncement, error) {
	if err := catalogAdmin(ctx, s.pool, userID, false); err != nil {
		return OpsAnnouncement{}, err
	}
	return loadOpsAnnouncement(ctx, s.pool, id, false)
}

func validAnnouncementContent(c *AnnouncementContent) (RenderedAnnouncement, error) {
	if c == nil || !utf8.ValidString(c.Title) || strings.TrimSpace(c.Title) != c.Title || len([]rune(c.Title)) < 1 || len([]rune(c.Title)) > 160 || strings.ContainsAny(c.Title, "\x00\r\n") || !slices.Contains([]string{"SYSTEM", "NEW_MODELS", "GAME_EVENTS", "MAINTENANCE", "IMPORTANT", "ACKNOWLEDGEMENTS"}, c.Type) || !slices.Contains([]string{"PUBLIC", "AUTHENTICATED"}, c.Visibility) {
		return RenderedAnnouncement{}, ErrAnnouncementInvalid
	}
	if len(c.Acknowledgements) > 100 || c.Type != "ACKNOWLEDGEMENTS" && len(c.Acknowledgements) > 0 {
		return RenderedAnnouncement{}, ErrAnnouncementInvalid
	}
	seen := map[int]bool{}
	for _, a := range c.Acknowledgements {
		if !a.ConsentAttested || a.MediaID != "" || len([]rune(a.DisplayName)) < 1 || len([]rune(a.DisplayName)) > 120 || len(a.Note) > 2000 || len(a.GroupName) > 120 || a.ManualOrder < 0 || a.ManualOrder > 1000000 || seen[a.ManualOrder] || a.ExternalLink != "" && !safeAnnouncementURL(a.ExternalLink) || a.Anonymous && (a.ExternalLink != "" || a.DisplayName != "匿名贡献者") {
			return RenderedAnnouncement{}, ErrAnnouncementInvalid
		}
		for _, v := range []string{a.DisplayName, a.Note, a.GroupName} {
			if !utf8.ValidString(v) || strings.ContainsRune(v, 0) {
				return RenderedAnnouncement{}, ErrAnnouncementInvalid
			}
		}
		seen[a.ManualOrder] = true
	}
	return RenderAnnouncement(c.Markdown)
}
func validAnnouncementPlacements(p []AnnouncementPlacement, visibility string) bool {
	seen := map[string]bool{}
	for _, v := range p {
		if !slices.Contains([]string{"PINNED_LIST", "ENTRY_POPUP", "POST_LOGIN_POPUP", "PUBLIC_HOME_BANNER", "DASHBOARD_SUMMARY"}, v.Placement) || seen[v.Placement] || v.ManualOrder < 0 || v.ManualOrder > 1000000 {
			return false
		}
		if visibility != "PUBLIC" && (v.Placement == "ENTRY_POPUP" || v.Placement == "PUBLIC_HOME_BANNER") {
			return false
		}
		seen[v.Placement] = true
	}
	return true
}
func validateAnnouncementCommand(ctx context.Context, tx pgx.Tx, c AnnouncementCommand) (OpsAnnouncement, RenderedAnnouncement, error) {
	var a OpsAnnouncement
	var rendered RenderedAnnouncement
	if !slices.Contains([]string{"SAVE", "UPDATE_CONTENT_ONLY", "PUBLISH", "SCHEDULE", "WITHDRAW", "ARCHIVE", "RE_NOTIFY", "UPDATE_PLACEMENTS"}, c.Action) {
		return a, rendered, ErrAnnouncementInvalid
	}
	var err error
	if c.ID == "" {
		if c.Action != "SAVE" {
			return a, rendered, ErrAnnouncementInvalid
		}
		a.State = "DRAFT"
	} else {
		if !announcementUUID(c.ID) {
			return a, rendered, ErrAnnouncementInvalid
		}
		a, err = loadOpsAnnouncement(ctx, tx, c.ID, true)
		if err != nil {
			return a, rendered, err
		}
	}
	if c.Action == "SAVE" || c.Action == "UPDATE_CONTENT_ONLY" {
		if c.Action == "SAVE" && a.State != "DRAFT" || c.Action == "UPDATE_CONTENT_ONLY" && (a.State != "SCHEDULED" && a.State != "PUBLISHED" || a.WithdrawnAt != nil) {
			return a, rendered, ErrAnnouncementConflict
		}
		rendered, err = validAnnouncementContent(c.Content)
		if err != nil {
			return a, rendered, err
		}
		if c.ID == "" && c.Content.Type == "ACKNOWLEDGEMENTS" && c.Content.Visibility != "PUBLIC" {
			return a, rendered, ErrAnnouncementInvalid
		}
		if a.CanonicalKey != "" && c.Content.Type != "ACKNOWLEDGEMENTS" {
			return a, rendered, ErrAnnouncementInvalid
		}
		if !validAnnouncementPlacements(a.Placements, c.Content.Visibility) {
			return a, rendered, ErrAnnouncementInvalid
		}
	} else if c.Content != nil {
		return a, rendered, ErrAnnouncementInvalid
	}
	if c.Action != "PUBLISH" && c.Action != "SCHEDULE" && (c.PublishAt != nil || c.VisibleFrom != nil || c.VisibleUntil != nil || c.Action != "UPDATE_PLACEMENTS" && len(c.Placements) > 0) {
		return a, rendered, ErrAnnouncementInvalid
	}
	switch c.Action {
	case "PUBLISH", "SCHEDULE":
		if a.State != "DRAFT" && a.State != "SCHEDULED" {
			return a, rendered, ErrAnnouncementConflict
		}
		var now time.Time
		if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
			return a, rendered, err
		}
		if c.PublishAt == nil || c.VisibleFrom == nil || c.VisibleFrom.Before(*c.PublishAt) || c.VisibleUntil != nil && (!c.VisibleUntil.After(*c.VisibleFrom) || !c.VisibleUntil.After(now)) || c.Action == "SCHEDULE" && !c.PublishAt.After(now) || c.Action == "PUBLISH" && c.PublishAt.After(now) || !validAnnouncementPlacements(c.Placements, a.Content.Visibility) {
			return a, rendered, ErrAnnouncementInvalid
		}
		if err = announcementPlacementGuard(ctx, tx, c.ID, c.Placements, *c.VisibleFrom, c.VisibleUntil); err != nil {
			return a, rendered, err
		}
	case "RE_NOTIFY", "UPDATE_PLACEMENTS":
		if a.State != "PUBLISHED" || a.WithdrawnAt != nil {
			return a, rendered, ErrAnnouncementInvalid
		}
		var active bool
		if err = tx.QueryRow(ctx, `SELECT publish_at<=clock_timestamp() AND visible_from<=clock_timestamp() AND (visible_until IS NULL OR clock_timestamp()<visible_until) FROM content.announcements WHERE announcement_id=$1`, a.ID).Scan(&active); err != nil {
			return a, rendered, err
		}
		if !active {
			return a, rendered, ErrAnnouncementInvalid
		}
		if c.Action == "UPDATE_PLACEMENTS" {
			if !validAnnouncementPlacements(c.Placements, a.Content.Visibility) {
				return a, rendered, ErrAnnouncementInvalid
			}
			if err = announcementPlacementGuard(ctx, tx, a.ID, c.Placements, *a.VisibleFrom, a.VisibleUntil); err != nil {
				return a, rendered, err
			}
		}
	case "WITHDRAW":
		if a.State == "DRAFT" || a.WithdrawnAt != nil {
			return a, rendered, ErrAnnouncementInvalid
		}
	case "ARCHIVE":
		if a.State == "DRAFT" || a.State == "ARCHIVED" {
			return a, rendered, ErrAnnouncementInvalid
		}
	}
	return a, rendered, nil
}
func announcementPlacementGuard(ctx context.Context, tx pgx.Tx, id string, placements []AnnouncementPlacement, from time.Time, until *time.Time) error {
	for _, pair := range [][2]string{{"ENTRY_POPUP", "ENTRY_POPUP"}, {"PUBLIC_HOME_BANNER", "PRIMARY_HOME_BANNER"}} {
		if !slices.ContainsFunc(placements, func(p AnnouncementPlacement) bool { return p.Placement == pair[0] }) {
			continue
		}
		var key string
		if err := tx.QueryRow(ctx, "SELECT guard_key FROM content.placement_guards WHERE guard_key=$1 FOR UPDATE", pair[1]).Scan(&key); err != nil {
			return err
		}
		var conflict bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM content.announcements a JOIN content.announcement_placements p USING(announcement_id) WHERE a.announcement_id<>$1 AND p.placement=$2 AND a.state IN ('SCHEDULED','PUBLISHED') AND a.withdrawn_at IS NULL AND tstzrange(a.visible_from,a.visible_until,'[)') && tstzrange($3::timestamptz,$4::timestamptz,'[)'))`, id, pair[0], from, until).Scan(&conflict)
		if err != nil {
			return err
		}
		if conflict {
			return ErrAnnouncementWindow
		}
	}
	return nil
}

func (s *Store) SaveAnnouncement(ctx context.Context, userID int64, c AnnouncementCommand) (OpsAnnouncement, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OpsAnnouncement{}, err
	}
	defer rollback(tx)
	result, err := saveAnnouncement(ctx, tx, userID, c)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func saveAnnouncement(ctx context.Context, tx pgx.Tx, userID int64, c AnnouncementCommand) (OpsAnnouncement, error) {
	var result OpsAnnouncement
	if err := catalogAdmin(ctx, tx, userID, true); err != nil {
		return result, err
	}
	a, rendered, err := validateAnnouncementCommand(ctx, tx, c)
	if err != nil {
		return result, err
	}
	var before any
	if c.ID != "" {
		before = a
	}
	if c.ID == "" {
		a.ID, err = uuidV7()
		if err != nil {
			return result, err
		}
		a.ContentVersion = 1
		a.NotificationRevision = 1
		a.Version = 1
		var canonical *string
		if c.Content.Type == "ACKNOWLEDGEMENTS" {
			v := "ACKNOWLEDGEMENTS"
			canonical = &v
			a.CanonicalKey = v
		}
		if _, err = tx.Exec(ctx, `INSERT INTO content.announcements(announcement_id,canonical_key) VALUES($1,$2)`, a.ID, canonical); err != nil {
			return result, announcementDBError(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO content.notification_revisions(announcement_id,notification_revision,created_by,reason) VALUES($1,1,$2,'INITIAL')`, a.ID, userID); err != nil {
			return result, err
		}
		if canonical != nil {
			a.Placements = []AnnouncementPlacement{{Placement: "PINNED_LIST"}, {Placement: "ENTRY_POPUP"}}
			for _, p := range a.Placements {
				if _, err = tx.Exec(ctx, `INSERT INTO content.announcement_placements VALUES($1,$2,$3)`, a.ID, p.Placement, p.ManualOrder); err != nil {
					return result, err
				}
			}
		}
	}
	if c.Content != nil {
		if c.ID != "" {
			a.ContentVersion++
			a.Version++
		}
		if err = insertAnnouncementContent(ctx, tx, a.ID, a.ContentVersion, userID, *c.Content, rendered); err != nil {
			return result, err
		}
		a.Content = *c.Content
		if a.CanonicalKey == "" && c.Content.Type == "ACKNOWLEDGEMENTS" {
			a.CanonicalKey = "ACKNOWLEDGEMENTS"
		}
		if _, err = tx.Exec(ctx, `UPDATE content.announcements SET current_content_version=$2,version=$3,canonical_key=NULLIF($4,''),updated_at=now() WHERE announcement_id=$1`, a.ID, a.ContentVersion, a.Version, a.CanonicalKey); err != nil {
			return result, announcementDBError(err)
		}
	}
	switch c.Action {
	case "PUBLISH", "SCHEDULE":
		a.State = "PUBLISHED"
		if c.Action == "SCHEDULE" {
			a.State = "SCHEDULED"
		}
		a.Version++
		a.PublishAt = c.PublishAt
		a.VisibleFrom = c.VisibleFrom
		a.VisibleUntil = c.VisibleUntil
		if _, err = tx.Exec(ctx, `UPDATE content.announcements SET state=$2,publish_at=$3,visible_from=$4,visible_until=$5,version=$6,first_published_at=CASE WHEN $2='PUBLISHED' THEN COALESCE(first_published_at,now()) ELSE first_published_at END,updated_at=now() WHERE announcement_id=$1`, a.ID, a.State, c.PublishAt, c.VisibleFrom, c.VisibleUntil, a.Version); err != nil {
			return result, err
		}
	case "UPDATE_PLACEMENTS":
		a.Version++
		if _, err = tx.Exec(ctx, `UPDATE content.announcements SET version=$2,updated_at=now() WHERE announcement_id=$1`, a.ID, a.Version); err != nil {
			return result, err
		}
	case "WITHDRAW", "ARCHIVE":
		a.Version++
		if c.Action == "ARCHIVE" {
			a.State = "ARCHIVED"
		}
		if _, err = tx.Exec(ctx, `UPDATE content.announcements SET state=$2,version=$3,withdrawn_at=CASE WHEN $4='WITHDRAW' THEN now() ELSE withdrawn_at END,updated_at=now() WHERE announcement_id=$1`, a.ID, a.State, a.Version, c.Action); err != nil {
			return result, err
		}
	case "RE_NOTIFY":
		a.NotificationRevision++
		a.Version++
		if _, err = tx.Exec(ctx, `INSERT INTO content.notification_revisions(announcement_id,notification_revision,created_by,reason) VALUES($1,$2,$3,$4)`, a.ID, a.NotificationRevision, userID, "RE_NOTIFY"); err != nil {
			return result, err
		}
		if _, err = tx.Exec(ctx, `UPDATE content.announcements SET notification_revision=$2,version=$3,updated_at=now() WHERE announcement_id=$1`, a.ID, a.NotificationRevision, a.Version); err != nil {
			return result, err
		}
	}
	if c.Action == "PUBLISH" || c.Action == "SCHEDULE" || c.Action == "UPDATE_PLACEMENTS" {
		if _, err = tx.Exec(ctx, `DELETE FROM content.announcement_placements WHERE announcement_id=$1`, a.ID); err != nil {
			return result, err
		}
		for _, placement := range c.Placements {
			if _, err = tx.Exec(ctx, `INSERT INTO content.announcement_placements VALUES($1,$2,$3)`, a.ID, placement.Placement, placement.ManualOrder); err != nil {
				return result, err
			}
		}
	}
	if a.State == "SCHEDULED" || a.State == "PUBLISHED" {
		if err = ensureAnnouncementJobs(ctx, tx, a); err != nil {
			return result, err
		}
	}
	result, err = loadOpsAnnouncement(ctx, tx, a.ID, false)
	if err != nil {
		return result, err
	}
	return result, relayOpsAudit(ctx, tx, userID, "ANNOUNCEMENT_"+c.Action, "ANNOUNCEMENT", a.ID, before, result)
}
func insertAnnouncementContent(ctx context.Context, tx pgx.Tx, id string, version, user int64, c AnnouncementContent, r RenderedAnnouncement) error {
	_, err := tx.Exec(ctx, `INSERT INTO content.announcement_revisions(announcement_id,content_version,title,type,visibility,body_markdown,sanitized_html,body_markdown_hash,sanitized_html_hash,sanitizer_policy_version,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, version, c.Title, c.Type, c.Visibility, c.Markdown, r.HTML, r.MarkdownHash, r.HTMLHash, r.PolicyVersion, user)
	if err != nil {
		return err
	}
	for _, a := range c.Acknowledgements {
		if _, err = tx.Exec(ctx, `INSERT INTO content.acknowledgement_entries(announcement_id,content_version,manual_order,display_name,external_link,acknowledgement_note,group_name,anonymous,consent_attested) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, version, a.ManualOrder, a.DisplayName, a.ExternalLink, a.Note, a.GroupName, a.Anonymous, a.ConsentAttested); err != nil {
			return err
		}
	}
	return nil
}
func ensureAnnouncementJobs(ctx context.Context, tx pgx.Tx, a OpsAnnouncement) error {
	if a.State == "SCHEDULED" {
		key := fmt.Sprintf("announcement:publish:%s:%d", a.ID, a.ContentVersion)
		if _, err := tx.Exec(ctx, `INSERT INTO content.announcement_jobs(job_key,announcement_id,kind,content_version,notification_revision,due_at) VALUES($1,$2,'PUBLISH',$3,$4,$5) ON CONFLICT(job_key) DO UPDATE SET due_at=EXCLUDED.due_at,notification_revision=EXCLUDED.notification_revision,status='PENDING',finished_at=NULL`, key, a.ID, a.ContentVersion, a.NotificationRevision, a.PublishAt); err != nil {
			return err
		}
	}
	if a.VisibleUntil != nil {
		key := fmt.Sprintf("announcement:expire:%s:%d", a.ID, a.NotificationRevision)
		if _, err := tx.Exec(ctx, `INSERT INTO content.announcement_jobs(job_key,announcement_id,kind,content_version,notification_revision,due_at) VALUES($1,$2,'EXPIRE',$3,$4,$5) ON CONFLICT(job_key) DO UPDATE SET due_at=EXCLUDED.due_at,content_version=EXCLUDED.content_version,status='PENDING',finished_at=NULL`, key, a.ID, a.ContentVersion, a.NotificationRevision, a.VisibleUntil); err != nil {
			return err
		}
	}
	return nil
}
