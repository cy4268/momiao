package platform

import (
	"errors"
	"time"
)

var (
	ErrAnnouncementForbidden = errors.New("ANNOUNCEMENTS_FORBIDDEN")
	ErrAnnouncementConflict  = errors.New("ANNOUNCEMENT_STATE_CONFLICT")
	ErrAnnouncementWindow    = errors.New("ANNOUNCEMENT_PLACEMENT_CONFLICT")
	ErrAnnouncementNotFound  = errors.New("ANNOUNCEMENT_NOT_FOUND")
)

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
	Version          int64                   `json:"-"`
	Content          AnnouncementContent     `json:"content"`
	Placements       []AnnouncementPlacement `json:"placements"`
	WithdrawnAt      *time.Time              `json:"withdrawn_at"`
	FirstPublishedAt *time.Time              `json:"first_published_at"`
	ExpiredReason    string                  `json:"expired_reason"`
	CanonicalKey     string                  `json:"canonical_key"`
}
type AnnouncementCommand struct {
	ID           string                  `json:"announcement_id"`
	Action       string                  `json:"action"`
	Content      *AnnouncementContent    `json:"content,omitempty"`
	PublishAt    *time.Time              `json:"publish_at,omitempty"`
	VisibleFrom  *time.Time              `json:"visible_from,omitempty"`
	VisibleUntil *time.Time              `json:"visible_until,omitempty"`
	Placements   []AnnouncementPlacement `json:"placements,omitempty"`
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
