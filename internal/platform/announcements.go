package platform

import (
	"errors"
	"time"
)

var (
	ErrAnnouncementForbidden    = errors.New("ANNOUNCEMENTS_FORBIDDEN")
	ErrAnnouncementStale        = errors.New("AUTHORIZATION_STALE")
	ErrAnnouncementConflict     = errors.New("ANNOUNCEMENT_VERSION_CONFLICT")
	ErrAnnouncementWindow       = errors.New("ANNOUNCEMENT_PLACEMENT_CONFLICT")
	ErrAnnouncementNotFound     = errors.New("ANNOUNCEMENT_NOT_FOUND")
	ErrAnnouncementConfirmation = errors.New("ANNOUNCEMENT_CONFIRMATION_REQUIRED")
	ErrAnnouncementOperation    = errors.New("ANNOUNCEMENT_OPERATION_CONFLICT")
)

type AnnouncementPrincipal struct {
	UserID      int64    `json:"user_id,string"`
	Role        string   `json:"base_role"`
	Epoch       int64    `json:"authz_epoch"`
	Permissions []string `json:"permissions"`
}
type Acknowledgement struct {
	DisplayName     string `json:"display_name"`
	ExternalLink    string `json:"external_link"`
	Note            string `json:"acknowledgement_note"`
	GroupName       string `json:"group_name"`
	ManualOrder     int    `json:"manual_order"`
	Anonymous       bool   `json:"anonymous"`
	ConsentAttested bool   `json:"consent_attested,omitempty"`
	MediaID         string `json:"avatar_or_logo_media_id,omitempty"`
}
type AnnouncementContent struct {
	Title            string            `json:"title"`
	Type             string            `json:"type"`
	Visibility       string            `json:"visibility"`
	Markdown         string            `json:"body_markdown"`
	Acknowledgements []Acknowledgement `json:"acknowledgements"`
}
type AnnouncementPlacement struct {
	Placement   string `json:"placement"`
	ManualOrder int    `json:"manual_order"`
}
type Announcement struct {
	ID                   string            `json:"announcement_id"`
	ContentVersion       int64             `json:"content_version"`
	NotificationRevision int64             `json:"notification_revision"`
	Title                string            `json:"title"`
	Type                 string            `json:"type"`
	HTML                 string            `json:"sanitized_html"`
	State                string            `json:"state"`
	PublishAt            *time.Time        `json:"publish_at"`
	VisibleFrom          *time.Time        `json:"visible_from"`
	VisibleUntil         *time.Time        `json:"visible_until"`
	UpdatedAt            time.Time         `json:"updated_at"`
	Pinned               bool              `json:"pinned"`
	Read                 bool              `json:"read"`
	Acknowledgements     []Acknowledgement `json:"acknowledgements"`
}
type OpsAnnouncement struct {
	Announcement
	Version          int64                   `json:"version"`
	Content          AnnouncementContent     `json:"content"`
	Placements       []AnnouncementPlacement `json:"placements"`
	WithdrawnAt      *time.Time              `json:"withdrawn_at"`
	FirstPublishedAt *time.Time              `json:"first_published_at"`
	ExpiredReason    string                  `json:"expired_reason"`
	CanonicalKey     string                  `json:"canonical_key"`
}
type AnnouncementCommand struct {
	OperationID     string                  `json:"operation_id"`
	Epoch           int64                   `json:"authz_epoch"`
	ID              string                  `json:"announcement_id"`
	ExpectedVersion int64                   `json:"expected_version"`
	Action          string                  `json:"action"`
	Content         *AnnouncementContent    `json:"content,omitempty"`
	PublishAt       *time.Time              `json:"publish_at,omitempty"`
	VisibleFrom     *time.Time              `json:"visible_from,omitempty"`
	VisibleUntil    *time.Time              `json:"visible_until,omitempty"`
	Placements      []AnnouncementPlacement `json:"placements,omitempty"`
	Reason          string                  `json:"reason"`
}
type AnnouncementImpact struct {
	Action               string                  `json:"action"`
	ID                   string                  `json:"announcement_id"`
	TargetVersion        int64                   `json:"target_version"`
	Title                string                  `json:"title"`
	Visibility           string                  `json:"visibility"`
	NotificationRevision int64                   `json:"notification_revision"`
	ReadAccounts         int64                   `json:"read_accounts"`
	PublishAt            *time.Time              `json:"publish_at"`
	VisibleFrom          *time.Time              `json:"visible_from"`
	VisibleUntil         *time.Time              `json:"visible_until"`
	Placements           []AnnouncementPlacement `json:"placements"`
	Effect               string                  `json:"effect"`
}
type AnnouncementPreview struct {
	ID        string             `json:"preview_id"`
	ExpiresAt time.Time          `json:"expires_at"`
	Impact    AnnouncementImpact `json:"impact"`
}
type AnnouncementResult struct {
	OperationID          string `json:"operation_id"`
	ID                   string `json:"announcement_id"`
	Version              int64  `json:"version"`
	ContentVersion       int64  `json:"content_version"`
	NotificationRevision int64  `json:"notification_revision"`
	State                string `json:"state"`
}
type AnnouncementFilter struct {
	Type, Search, Placement string
	DateFrom, DateTo        *time.Time
	Archive                 bool
	Offset, Limit           int
}
type AnnouncementPage struct {
	Items   []Announcement `json:"items"`
	HasMore bool           `json:"has_more"`
}
