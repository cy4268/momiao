package poker

import (
	"time"

	"github.com/cy4268/momiao/internal/platform"
)

type HistorySessionQuery struct {
	FundingLimit, HandLimit   int
	FundingCursor, HandCursor string
}
type HistoryHandQuery struct {
	Limit  int
	Cursor string
}
type HistoryMetadata struct {
	SnapshotID string    `json:"snapshot_id"`
	GameTitle  string    `json:"game_title"`
	TableName  *string   `json:"table_name"`
	ActorName  *string   `json:"actor_display_name"`
	Origin     string    `json:"metadata_origin"`
	CapturedAt time.Time `json:"captured_at"`
}
type HistoryConfiguration struct {
	OriginalReferenceStatus string  `json:"original_reference_status"`
	VersionID               *string `json:"game_config_version_id"`
	Hash                    *string `json:"config_hash"`
	AccessMode              *string `json:"access_mode"`
	AccessModeStatus        string  `json:"access_mode_status"`
	BlindOrigin             string  `json:"blind_origin"`
	SmallBlind              *int64  `json:"small_blind_units,string"`
	BigBlind                *int64  `json:"big_blind_units,string"`
	EvaluatorVersion        string  `json:"evaluator_version,omitempty"`
	DeckVersion             string  `json:"deck_version,omitempty"`
	DealVersion             string  `json:"deal_sequence_version,omitempty"`
	Ante                    string  `json:"ante"`
	AnteOrigin              string  `json:"ante_origin"`
}
type HistorySessionDetail struct {
	ID                string               `json:"session_id"`
	TableID           string               `json:"table_id"`
	Seat              int                  `json:"seat_no"`
	State             string               `json:"state"`
	StartedAt         time.Time            `json:"started_at"`
	EndedAt           *time.Time           `json:"ended_at"`
	EndReason         *string              `json:"end_reason"`
	InitialBuyIn      int64                `json:"initial_buyin_units,string"`
	TopUp             int64                `json:"confirmed_topup_units,string"`
	Rebuy             int64                `json:"confirmed_rebuy_units,string"`
	FinalCashOut      *int64               `json:"final_cashout_units,string"`
	RealizedPL        *int64               `json:"realized_pl_units,string"`
	Metadata          HistoryMetadata      `json:"metadata"`
	Configuration     HistoryConfiguration `json:"configuration"`
	FundingCount      int64                `json:"funding_count,string"`
	HandCount         int64                `json:"hand_count,string"`
	Funding           []HistoryFunding     `json:"funding"`
	Hands             []HistoryHandSummary `json:"hands"`
	NextFundingCursor string               `json:"next_funding_cursor,omitempty"`
	NextHandCursor    string               `json:"next_hand_cursor,omitempty"`
	ReadAt            time.Time            `json:"read_at"`
}
type HistoryFunding struct {
	ID          string                       `json:"funding_operation_id"`
	Kind        string                       `json:"kind"`
	State       string                       `json:"state"`
	Amount      int64                        `json:"amount_units,string"`
	CreatedAt   time.Time                    `json:"created_at"`
	ConfirmedAt *time.Time                   `json:"confirmed_at"`
	FailureCode *string                      `json:"failure_code"`
	Transaction *platform.HistoryTransaction `json:"transaction,omitempty"`
}
type HistoryHandSummary struct {
	ID        string     `json:"hand_id"`
	Number    int64      `json:"hand_no,string"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	SettledAt *time.Time `json:"settled_at"`
}
type HistoryHandDetail struct {
	HistoryHandSummary
	TableID       string               `json:"table_id"`
	SessionID     string               `json:"session_id"`
	Seat          int                  `json:"seat_no"`
	Button        int                  `json:"button_seat"`
	Version       uint64               `json:"hand_version,string"`
	Metadata      HistoryMetadata      `json:"metadata"`
	Configuration HistoryConfiguration `json:"configuration"`
	Participants  []HistoryParticipant `json:"participants"`
	Board         []int                `json:"board_cards"`
	Actions       []HistoryAction      `json:"actions"`
	NextCursor    string               `json:"next_cursor,omitempty"`
	Pots          []HistoryPot         `json:"pots"`
	Returns       []HistoryReturn      `json:"uncalled_returns"`
	Settlement    *HistorySettlement   `json:"settlement"`
	Fairness      FairnessView         `json:"fairness"`
	ReadAt        time.Time            `json:"read_at"`
}
type HistoryParticipant struct {
	Seat            int    `json:"seat_no"`
	DisplayName     string `json:"display_name"`
	NameOrigin      string `json:"name_origin"`
	Initial         int64  `json:"initial_stack_units,string"`
	Ending          *int64 `json:"ending_stack_units,string"`
	Net             *int64 `json:"net_change_units,string"`
	Folded          bool   `json:"folded"`
	HoleCards       []int  `json:"hole_cards,omitempty"`
	PublicHoleCards []int  `json:"public_hole_cards,omitempty"`
}
type HistoryAction struct {
	Sequence uint64    `json:"sequence,string"`
	Version  uint64    `json:"hand_version,string"`
	Type     string    `json:"type"`
	Street   string    `json:"street"`
	Seat     int       `json:"seat_no,omitempty"`
	Delta    int64     `json:"delta_units,string"`
	To       int64     `json:"to_units,string"`
	Card     *int      `json:"card,omitempty"`
	At       time.Time `json:"at"`
}
type HistoryPot struct {
	Index    int            `json:"index"`
	Amount   int64          `json:"amount_units,string"`
	Floor    int64          `json:"contribution_floor,string"`
	Ceiling  int64          `json:"contribution_ceiling,string"`
	Eligible []int          `json:"eligible_seats"`
	Awards   []HistoryAward `json:"awards"`
}
type HistoryAward struct {
	Seat   int   `json:"seat_no"`
	Base   int64 `json:"base_share_units,string"`
	Odd    int64 `json:"odd_chip_units,string"`
	Amount int64 `json:"award_units,string"`
}
type HistoryReturn struct {
	Seat   int   `json:"seat_no"`
	Amount int64 `json:"amount_units,string"`
}
type HistorySettlement struct {
	Paid     int64     `json:"total_commitment_units,string"`
	Awarded  int64     `json:"total_award_units,string"`
	Returned int64     `json:"uncalled_return_units,string"`
	Rake     int64     `json:"rake_units,string"`
	At       time.Time `json:"settled_at"`
}
