// Package botroulette adapts private Bot requests to the existing roulette
// engine. Rules, randomness, settlement and transaction ordering stay there.
package botroulette

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cy4268/momiao/internal/roulette"
)

type Engine interface {
	List(context.Context, int64, roulette.LobbyQuery) (roulette.Lobby, error)
	View(context.Context, int64, string) (roulette.RoomView, error)
	PublicView(context.Context, string) (roulette.PublicRoomView, error)
	CreateBefore(context.Context, int64, roulette.CreateRequest, time.Time) (roulette.Receipt, error)
	CommandBefore(context.Context, int64, string, roulette.Command, time.Time) (roulette.Receipt, error)
	FindReceiptMatching(context.Context, int64, roulette.BotIntent, time.Time) (roulette.BotLookup, error)
}

type LobbyInput struct {
	Game   string  `json:"game"`
	Cursor *string `json:"cursor"`
}
type RoomInput struct {
	Game   string `json:"game"`
	RoomID string `json:"room_id"`
}
type StateInput struct {
	RoomID string `json:"room_id"`
}
type QuoteInput struct {
	Quote string `json:"quote"`
}
type PrepareInput struct {
	RequestID string     `json:"request_id"`
	Purpose   string     `json:"purpose"`
	Game      string     `json:"game"`
	RoomID    *string    `json:"room_id,omitempty"`
	Input     TypedInput `json:"input"`
}
type CreateInput struct {
	Stake   string `json:"stake"`
	Players int    `json:"players"`
}
type CommandInput struct {
	ExpectedVersion int64              `json:"expected_version,string"`
	Action          Action             `json:"action"`
	Ready           *ReadyConfirmation `json:"ready"`
}
type ReadyConfirmation struct {
	ClientSeed     string `json:"client_seed"`
	ConfigHash     string `json:"config_hash"`
	PolicyHash     string `json:"policy_hash"`
	ServerSeedHash string `json:"server_seed_hash"`
	StakeUnits     int64  `json:"stake_units,string"`
}
type Action struct {
	Kind       string `json:"kind"`
	Target     string `json:"target,omitempty"`
	Item       string `json:"item,omitempty"`
	StolenItem string `json:"stolen_item,omitempty"`
	Agree      *bool  `json:"agree,omitempty"`
}

// TypedInput has exactly one branch, serialized without a union wrapper. The
// enclosing purpose must match it; no arbitrary JSON is signed or forwarded.
type TypedInput struct {
	Create  *CreateInput
	Command *CommandInput
}

func (in TypedInput) MarshalJSON() ([]byte, error) {
	if in.Create != nil && in.Command == nil {
		return json.Marshal(in.Create)
	}
	if in.Command != nil && in.Create == nil {
		return json.Marshal(in.Command)
	}
	return nil, Fault{Code: "INVALID_REQUEST"}
}

type LobbyReply struct {
	SchemaVersion  string      `json:"schema_version"`
	Game           string      `json:"game"`
	State          string      `json:"state"`
	Binding        Binding     `json:"binding"`
	MinimumUnits   int64       `json:"minimum_units,string"`
	StepUnits      int64       `json:"step_units,string"`
	AvailableUnits int64       `json:"available_units,string"`
	OwnRoundID     *string     `json:"own_round_id"`
	Rooms          []LobbyRoom `json:"rooms"`
	NextCursor     *string     `json:"next_cursor"`
}
type LobbyRoom struct {
	ID            string       `json:"id"`
	Game          string       `json:"game"`
	Title         string       `json:"title"`
	Version       int64        `json:"version,string"`
	State         string       `json:"state"`
	TargetPlayers int          `json:"target_players"`
	StakeUnits    int64        `json:"stake_units,string"`
	PoolUnits     int64        `json:"pool_units,string"`
	Players       []PlayerView `json:"players"`
}
type PublicReply struct {
	SchemaVersion string     `json:"schema_version"`
	Room          PublicRoom `json:"room"`
}
type PublicRoom struct {
	ID             string        `json:"id"`
	Game           string        `json:"game"`
	Title          string        `json:"title"`
	Version        int64         `json:"version,string"`
	Sequence       int64         `json:"sequence,string"`
	State          string        `json:"state"`
	TargetPlayers  int           `json:"target_players"`
	StakeUnits     int64         `json:"stake_units,string"`
	PoolUnits      int64         `json:"pool_units,string"`
	Binding        Binding       `json:"binding"`
	ServerSeedHash string        `json:"server_seed_hash"`
	TurnSeat       *int          `json:"turn_seat"`
	ServerNow      time.Time     `json:"server_now"`
	Deadline       *time.Time    `json:"deadline"`
	GameDeadline   *time.Time    `json:"game_deadline"`
	Players        []PlayerView  `json:"players"`
	Log            []PublicEvent `json:"log"`
	Devil          *DevilView    `json:"devil"`
	Pressure       *PressureView `json:"pressure"`
}
type Binding struct {
	ConfigID   string `json:"config_version_id"`
	ConfigHash string `json:"config_hash"`
	PolicyID   string `json:"wager_policy_version_id"`
	PolicyHash string `json:"wager_policy_hash"`
	Ruleset    string `json:"ruleset_version"`
	Algorithm  string `json:"algorithm_version"`
	Stream     string `json:"fairness_stream_version"`
}
type PlayerView struct {
	Seat      int    `json:"seat"`
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	Alive     bool   `json:"alive"`
	Forfeited bool   `json:"forfeited"`
	HP        int    `json:"hp"`
	ItemCount int    `json:"item_count"`
}
type DevilView struct {
	Remaining int     `json:"remaining"`
	Live      *int    `json:"live"`
	Blank     *int    `json:"blank"`
	Saw       bool    `json:"saw"`
	Cuffed    [2]bool `json:"cuffed"`
}
type PressureView struct {
	ActualLoad      int          `json:"actual_load"`
	ForcedShots     int          `json:"forced_shots"`
	Order           []int        `json:"order"`
	Pointer         int          `json:"pointer"`
	UnloadSkipsShot bool         `json:"unload_skips_shot"`
	TimeoutTier     int          `json:"timeout_tier"`
	Phase           string       `json:"phase"`
	Chambers        [6]string    `json:"chambers"`
	PoolRemaining   int          `json:"pool_remaining"`
	Duds            int          `json:"duds"`
	Charge          int          `json:"charge"`
	Loaded          int          `json:"loaded"`
	Forced          int          `json:"forced"`
	Aggressor       *int         `json:"aggressor"`
	RiposteTarget   *int         `json:"riposte_target"`
	Votes           map[int]bool `json:"votes"`
}
type PublicEvent struct {
	Sequence int64     `json:"sequence,string"`
	At       time.Time `json:"at"`
	Seat     *int      `json:"seat"`
	Kind     string    `json:"kind"`
	Text     string    `json:"text"`
}
type StateReply struct {
	SchemaVersion     string          `json:"schema_version"`
	Room              PublicRoom      `json:"room"`
	Self              *SelfView       `json:"self"`
	Actions           []Action        `json:"actions"`
	EconomicPolicy    *EconomicPolicy `json:"economic_policy"`
	EconomySettlement *PayoutCapView  `json:"economy_settlement"`
	OwnRoundID        *string         `json:"own_round_id"`
}
type SelfView struct {
	Seat           int          `json:"seat"`
	AvailableUnits int64        `json:"available_units,string"`
	Items          []string     `json:"items"`
	Intel          []IntelEntry `json:"intel"`
}
type IntelEntry struct {
	Index int  `json:"index"`
	Live  bool `json:"live"`
}
type EconomicPolicy struct {
	Version              string `json:"version"`
	Hash                 string `json:"hash"`
	SinglePlayerMaxUnits int64  `json:"single_player_max_units,string"`
	AssetCapUnits        int64  `json:"asset_cap_units,string"`
	CapMode              string `json:"cap_mode"`
}
type PayoutCapView struct {
	PolicyVersion       string `json:"policy_version"`
	PolicyHash          string `json:"policy_hash"`
	GrossPayoutUnits    int64  `json:"gross_payout_units,string"`
	CreditedPayoutUnits int64  `json:"credited_payout_units,string"`
	WithheldUnits       int64  `json:"withheld_units,string"`
	ActualNetUnits      int64  `json:"actual_net_units,string"`
	NeutralReturnUnits  int64  `json:"neutral_return_units,string,omitempty"`
}
type PreparedReply struct {
	SchemaVersion      string     `json:"schema_version"`
	RequestID          string     `json:"request_id"`
	Purpose            string     `json:"purpose"`
	Game               string     `json:"game"`
	RoomID             *string    `json:"room_id"`
	Input              TypedInput `json:"input"`
	Quote              string     `json:"quote"`
	BindingFingerprint string     `json:"binding_fingerprint"`
	IssuedAt           string     `json:"issued_at"`
	ExpiresAt          string     `json:"expires_at"`
}
type ReceiptReply struct {
	SchemaVersion string   `json:"schema_version"`
	Status        string   `json:"status"`
	RequestID     string   `json:"request_id"`
	Receipt       *Receipt `json:"receipt"`
}
type Receipt struct {
	RoundID  string `json:"round_id"`
	Version  int64  `json:"version,string"`
	Sequence int64  `json:"sequence,string"`
	State    string `json:"state"`
}

type Fault struct{ Code string }

func (f Fault) Error() string {
	switch f.Code {
	case "INVALID_REQUEST", "UNAUTHORIZED", "BINDING_CHANGED", "ACCOUNT_RESTRICTED",
		"NOT_LINKED", "NOT_FOUND", "ACCOUNT_NOT_READY", "QUOTE_EXPIRED", "COMMITMENT_INVALID",
		"IDEMPOTENCY_CONFLICT", "INSUFFICIENT_CHIPS", "MAINTENANCE", "UPSTREAM_UNAVAILABLE",
		"ROULETTE_VERSION_CONFLICT", "ROULETTE_ACTION_INVALID", "ROULETTE_ALREADY_SEATED", "ROULETTE_NEEDS_REVIEW":
		return f.Code
	default:
		return "UPSTREAM_UNAVAILABLE"
	}
}
