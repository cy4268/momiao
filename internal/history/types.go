package history

import (
	"errors"
	"time"
)

type Source string

const (
	Round   Source = "DIRECT_PLAY_ROUND"
	Session Source = "POKER_SESSION"
	Hand    Source = "POKER_HAND"
 RouletteRound Source = "ROULETTE_ROUND"
)

var ErrQuery = errors.New("HISTORY_QUERY_INVALID")

type Query struct {
	RecordType     Source `json:"record_type,omitempty"`
	Mode           string `json:"mode,omitempty"`
	GameSlug       string `json:"game_slug,omitempty"`
	TimeFrom       string `json:"time_from,omitempty"`
	TimeTo         string `json:"time_to,omitempty"`
	Result         string `json:"result,omitempty"`
	Status         string `json:"status,omitempty"`
	ID             string `json:"id,omitempty"`
	ParentSourceID string `json:"parent_source_id,omitempty"`
	Limit          int    `json:"limit,omitempty"`
	Cursor         string `json:"-"`
}

type Snapshot struct {
	ID               string  `json:"snapshot_id"`
	GameTitle        string  `json:"game_title"`
	TableID          *string `json:"table_id"`
	TableName        *string `json:"table_name"`
	ActorDisplayName *string `json:"actor_display_name"`
	MetadataOrigin   string  `json:"metadata_origin"`
}

type Summary struct {
	RecordType        Source     `json:"record_type"`
	SourceID          string     `json:"source_id"`
	UserID            string     `json:"newapi_user_id"`
	ParentSourceID    *string    `json:"parent_source_id"`
	GameSlug          string     `json:"game_slug"`
	Mode              string     `json:"mode"`
	OccurredAt        time.Time  `json:"occurred_at"`
	EndedAt           *time.Time `json:"ended_at"`
	Result            *string    `json:"result"`
	Status            string     `json:"status"`
	SourceVersion     string     `json:"source_version"`
	StakeUnits        *string    `json:"stake_units"`
	PayoutUnits       *string    `json:"payout_units"`
	NetChangeUnits    *string    `json:"net_change_units"`
	InitialBuyInUnits *string    `json:"initial_buyin_units"`
	TotalTopUpUnits   *string    `json:"total_topup_units"`
	FinalCashOutUnits *string    `json:"final_cashout_units"`
	Snapshot          Snapshot   `json:"snapshot"`
}

type GameOption struct {
	GameSlug  string `json:"game_slug"`
	GameTitle string `json:"game_title"`
	Retired   bool   `json:"retired"`
}

type Page struct {
	Items       []Summary    `json:"items"`
	NextCursor  *string      `json:"next_cursor"`
	HasMore     bool         `json:"has_more"`
	GameOptions []GameOption `json:"game_options"`
}

type cursor struct {
	Version int
	UserID  int64
	Filter  string
	Time    time.Time
	Type    Source
	ID      string
}
