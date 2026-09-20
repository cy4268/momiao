// Package roulette implements fixed-stake native multiplayer rounds. Hidden rule
// snapshots never cross this package's projection boundary.
package roulette

import (
	"errors"
	"time"
)

type Keyring struct {
	Active string
	Keys   map[string][32]byte
}
type CreateRequest struct {
	Key     string `json:"-"`
	Game    string `json:"game"`
	Stake   string `json:"stake"`
	Players int    `json:"players"`
}
type ReadyConfirmation struct {
	ClientSeed     string `json:"client_seed"`
	ConfigHash     string `json:"config_hash"`
	PolicyHash     string `json:"policy_hash"`
	ServerSeedHash string `json:"server_seed_hash"`
	StakeUnits     string `json:"stake_units"`
}
type Action struct {
	Kind       string `json:"kind"`
	Target     string `json:"target,omitempty"`
	Item       string `json:"item,omitempty"`
	StolenItem string `json:"stolen_item,omitempty"`
	Agree      *bool  `json:"agree,omitempty"`
}
type Command struct {
	Key             string             `json:"-"`
	ExpectedVersion int64              `json:"expected_version,string"`
	Action          Action             `json:"action"`
	Ready           *ReadyConfirmation `json:"ready,omitempty"`
}
type Receipt struct {
	RoundID  string `json:"round_id"`
	Version  int64  `json:"version,string"`
	Sequence int64  `json:"sequence,string"`
	State    string `json:"state"`
}

var (
	ErrInvalidInput        = errors.New("ROULETTE_INPUT_INVALID")
	ErrVersionConflict     = errors.New("ROULETTE_VERSION_CONFLICT")
	ErrActionInvalid       = errors.New("ROULETTE_ACTION_INVALID")
	ErrAlreadySeated       = errors.New("ROULETTE_ALREADY_SEATED")
	ErrIdempotencyConflict = errors.New("ROULETTE_IDEMPOTENCY_CONFLICT")
	ErrNotFound            = errors.New("ROULETTE_NOT_FOUND")
	ErrUnavailable         = errors.New("ROULETTE_UNAVAILABLE")
)

type Binding struct {
	ConfigID   string `json:"config_version_id"`
	ConfigHash string `json:"config_hash"`
	PolicyID   string `json:"wager_policy_version_id"`
	PolicyHash string `json:"wager_policy_hash"`
	Ruleset    string `json:"ruleset_version"`
	Algorithm  string `json:"algorithm_version"`
	Stream     string `json:"fairness_stream_version"`
}
type RoomView struct {
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
	Self           *SelfView     `json:"self"`
	Actions        []Action      `json:"actions"`
	Log            []PublicEvent `json:"log"`
	Devil          *DevilView    `json:"devil"`
	Pressure       *PressureView `json:"pressure"`
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
type LobbyQuery struct{ Game, Cursor string }
type Lobby struct {
	Game           string     `json:"game"`
	State          string     `json:"state"`
	Binding        Binding    `json:"binding"`
	MinimumUnits   int64      `json:"minimum_units,string"`
	StepUnits      int64      `json:"step_units,string"`
	AvailableUnits int64      `json:"available_units,string"`
	OwnRoundID     *string    `json:"own_round_id"`
	Rooms          []RoomView `json:"rooms"`
	NextCursor     *string    `json:"next_cursor"`
}
type HistoryQuery struct {
	Limit  int
	Cursor string
}

// ValidAction rejects irrelevant fields before any rule RNG is constructed.
func ValidAction(a Action, internal bool) bool {
	switch a.Kind {
	case "DEVIL_SHOOT":
		return (a.Target == "SELF" || a.Target == "OPPONENT") && a.Item == "" && a.StolenItem == "" && a.Agree == nil
	case "DEVIL_ITEM":
		return a.Target == "" && a.Agree == nil && validItem(a.Item) && (a.Item == "adrenaline" && validItem(a.StolenItem) && a.StolenItem != "adrenaline" || a.Item != "adrenaline" && a.StolenItem == "")
	case "PRESSURE_VOTE":
		return a.Agree != nil && a.Target == "" && a.Item == "" && a.StolenItem == ""
	case "TIMEOUT", "GAME_LIMIT":
		if !internal {
			return false
		}
	case "JOIN", "LEAVE", "READY", "UNREADY", "CANCEL", "SURRENDER", "PRESSURE_FIRE", "PRESSURE_PASS", "PRESSURE_AGAIN", "PRESSURE_CHARGE", "PRESSURE_UNLOAD", "PRESSURE_RIPOSTE":
	default:
		return false
	}
	return a.Target == "" && a.Item == "" && a.StolenItem == "" && a.Agree == nil
}
