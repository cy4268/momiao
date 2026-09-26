package games

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/games/blackjack"
	"github.com/cy4268/momiao/internal/games/slot"
	"github.com/cy4268/momiao/internal/platform"
)

var (
	ErrNotFound          = errors.New("games: not found")
	ErrUnavailable       = errors.New("games: temporarily unavailable")
	ErrMaintenance       = errors.New("games: maintenance")
	ErrCommitmentInvalid = errors.New("games: commitment invalid; reload bootstrap")
	ErrScratchIncomplete = errors.New("games: previous scratch reveal incomplete")
	ErrNonceExhausted    = errors.New("games: nonce exhausted")
	ErrActiveRound       = errors.New("games: active blackjack round must be resumed")
)

const UnitsPerChip int64 = 500000
const validationVersion = "direct-validate-v1"

type CreateInput struct {
	Type         string `json:"type"`
	Wager        string `json:"wager,omitempty"`
	Choice       string `json:"choice,omitempty"`
	BaseWager    string `json:"base_wager,omitempty"`
	Mode         string `json:"mode,omitempty"`
	TotalWager   string `json:"total_wager,omitempty"`
	InitialWager string `json:"initial_wager,omitempty"`
}
type normalizedCreate struct {
	Input                       CreateInput
	Base, TotalStake, MaxPayout int64
}

func normalizeCreate(slug string, input CreateInput) (normalizedCreate, error) {
	return normalizeCreateForRuleset(slug, input, "")
}

func normalizeCreateForRuleset(slug string, input CreateInput, ruleset string) (normalizedCreate, error) {
	n := normalizedCreate{Input: input}
	if (slug != "slot" && input.TotalWager != "") || (slug != "blackjack" && input.InitialWager != "") {
		return n, ErrInvalidInput
	}
	value := input.Wager
	count, max, divisor := int64(1), int64(0), int64(1)
	switch slug {
	case "dice":
		if input.Type != "DICE" || (input.Choice != "BIG" && input.Choice != "SMALL") || input.Mode != "" || input.BaseWager != "" {
			return n, ErrInvalidInput
		}
		max = 2
	case "scratch":
		if input.Type != "SCRATCH" || input.Choice != "" || input.Mode != "" || input.BaseWager != "" {
			return n, ErrInvalidInput
		}
		max = 100
	case "summon":
		if input.Type != "SUMMON" || input.Choice != "" || input.Wager != "" || (input.Mode != "SINGLE" && input.Mode != "TENFOLD") {
			return n, ErrInvalidInput
		}
		value = input.BaseWager
		max = 100
		if input.Mode == "TENFOLD" {
			count = 10
			max = 1000
		}
	case "slot", "blackjack":
		if input.Type != strings.ToUpper(slug) || input.Wager != "" || input.Choice != "" || input.Mode != "" || input.BaseWager != "" {
			return n, ErrInvalidInput
		}
		if slug == "slot" {
			value, max, divisor = input.TotalWager, slot.MaxRoundLineMultiplier, 10
			if ruleset == slot.FrequentRulesetVersion {
				max = slot.FrequentMaxRoundLineMultiplier
			}
		} else {
			// The v2 reference-strategy fair return is paid in addition to the
			// largest ordinary blackjack settlement, so reserve one extra wager.
			value, max = input.InitialWager, 17
		}
	default:
		return n, ErrNotFound
	}
	if value == "" || len(value) > 19 || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return n, ErrInvalidInput
	}
	chips, err := strconv.ParseInt(value, 10, 64)
	if err != nil || chips < 10 || chips > math.MaxInt64/UnitsPerChip || chips*UnitsPerChip/divisor > math.MaxInt64/max {
		return n, ErrInvalidInput
	}
	n.Base = chips * UnitsPerChip
	n.TotalStake = n.Base * count
	n.MaxPayout = n.Base / divisor * max
	if slug == "summon" {
		n.Input.BaseWager = strconv.FormatInt(chips, 10)
	} else if slug == "slot" {
		n.Input.TotalWager = strconv.FormatInt(chips, 10)
	} else if slug == "blackjack" {
		n.Input.InitialWager = strconv.FormatInt(chips, 10)
	} else {
		n.Input.Wager = strconv.FormatInt(chips, 10)
	}
	return n, nil
}
func validClientSeed(seed string) bool {
	if len(seed) < 1 || len(seed) > 128 || !utf8.ValidString(seed) {
		return false
	}
	for _, r := range seed {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func validRequestKey(key string) bool {
	if len(key) < 16 || len(key) > 128 {
		return false
	}
	for _, c := range key {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func semanticHash(user int64, slug string, input CreateInput) [32]byte {
	raw, _ := json.Marshal(input)
	return sha256.Sum256(append([]byte("game.round.create.v1\x00"+strconv.FormatInt(user, 10)+"\x00"+slug+"\x00"), raw...))
}

type Policy struct {
	ID          string   `json:"version_id"`
	Version     int64    `json:"version,string"`
	Minimum     int64    `json:"minimum_wager_units,string"`
	MaximumMode string   `json:"maximum_mode"`
	Step        int64    `json:"input_step_units,string"`
	Quick       []string `json:"quick_amount_units"`
	Hash        string   `json:"hash"`
}
type ConfigSummary struct {
	Version    string          `json:"version_id"`
	Hash       string          `json:"hash"`
	Schema     string          `json:"schema"`
	Ruleset    string          `json:"ruleset_version"`
	Algorithm  string          `json:"algorithm_version"`
	Prizes     []Prize         `json:"prizes,omitempty"`
	Statistics MathSummary     `json:"statistics"`
	Validation json.RawMessage `json:"validation,omitempty"`
}
type MathSummary struct {
	BreakEven string `json:"break_even"`
	Loss      string `json:"loss"`
	RTP       string `json:"rtp"`
	Top       string `json:"top"`
	Win       string `json:"win"`
}

func mathSummary(c Config) MathSummary {
	m, err := c.Statistics()
	if err != nil {
		return MathSummary{}
	}
	return MathSummary{m.BreakEven.RatString(), m.Loss.RatString(), m.RTP.RatString(), m.Top.RatString(), m.Win.RatString()}
}

type CatalogEntry struct {
	Slug           string         `json:"slug"`
	Title          string         `json:"title"`
	State          string         `json:"effective_runtime"`
	Implementation string         `json:"implementation_key"`
	Config         *ConfigSummary `json:"config,omitempty"`
}
type ClientSeedPreference struct {
	Seed    string `json:"client_seed"`
	Version int64  `json:"version,string"`
}
type Commitment struct {
	EconomicVersion string          `json:"economic_policy_version,omitempty"`
	ID              string          `json:"id"`
	ReservedRoundID string          `json:"reserved_round_id"`
	ServerSeedHash  string          `json:"server_seed_hash"`
	Nonce           int64           `json:"nonce,string"`
	ClientSeed      string          `json:"client_seed"`
	ClientVersion   int64           `json:"client_seed_version,string"`
	ConfigVersion   string          `json:"config_version_id"`
	ConfigHash      string          `json:"config_hash"`
	Ruleset         string          `json:"ruleset_version"`
	Algorithm       string          `json:"algorithm_version"`
	Stream          string          `json:"fairness_stream_version"`
	PolicyVersion   string          `json:"wager_policy_version_id"`
	PolicyHash      string          `json:"wager_policy_hash"`
	Resources       json.RawMessage `json:"resource_versions"`
}
type Bootstrap struct {
	EconomicPolicy *platform.EconomicPolicy `json:"economic_policy,omitempty"`
	Game           CatalogEntry             `json:"game"`
	WagerPolicy    Policy                   `json:"wager_policy"`
	AvailableUnits int64                    `json:"available_units,string"`
	Latest         *GameRound               `json:"latest_round"`
	Active         *GameRound               `json:"active_round"`
	ScratchBlocker *GameRound               `json:"scratch_presentation_blocker"`
	EntryAction    string                   `json:"effective_entry_action"`
	Next           *Commitment              `json:"next_commitment"`
	ClientSeed     ClientSeedPreference     `json:"client_seed_preference"`
	CSRFToken      string                   `json:"csrf_token,omitempty"`
}
type GameRound struct {
	EconomicVersion         string                  `json:"economic_policy_version,omitempty"`
	EconomySettlement       *platform.PayoutCapView `json:"economy_settlement,omitempty"`
	capWithheld             int64
	ID                      string                `json:"id"`
	Game                    string                `json:"game"`
	State                   string                `json:"state"`
	RecoveryState           string                `json:"recovery_state"`
	Input                   CreateInput           `json:"input"`
	StakeUnits              int64                 `json:"total_stake_units,string"`
	PayoutUnits             int64                 `json:"total_payout_units,string"`
	NetUnits                int64                 `json:"net_change_units,string"`
	Outcome                 Outcome               `json:"common_result"`
	BalanceBeforeUnits      int64                 `json:"balance_before_units,string"`
	BalanceAfterUnits       int64                 `json:"balance_after_units,string"`
	WagerTransactionID      string                `json:"wager_transaction_id"`
	SettlementTransactionID string                `json:"settlement_transaction_id"`
	ConfigVersion           string                `json:"config_version_id"`
	ConfigHash              string                `json:"config_hash"`
	PolicyVersion           string                `json:"wager_policy_version_id"`
	PolicyHash              string                `json:"wager_policy_hash"`
	Algorithm               string                `json:"algorithm_version"`
	Ruleset                 string                `json:"ruleset_version"`
	Stream                  string                `json:"fairness_stream_version"`
	Nonce                   int64                 `json:"nonce,string"`
	CommitmentID            string                `json:"commitment_id"`
	CreatedAt               time.Time             `json:"created_at"`
	SettledAt               *time.Time            `json:"settled_at"`
	PresentationCompletedAt *time.Time            `json:"presentation_completed_at,omitempty"`
	Dice                    *DiceResult           `json:"dice,omitempty"`
	Scratch                 *ScratchResult        `json:"scratch,omitempty"`
	Summon                  *SummonResult         `json:"summon,omitempty"`
	Slot                    *slot.Result          `json:"slot,omitempty"`
	Blackjack               *blackjack.Projection `json:"blackjack,omitempty"`
}
type HistoryQuery struct {
	Game, Before string
	Limit        int
}
type HistoryPage struct {
	Items      []GameRound `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
}
type Verification struct {
	RoundID         string          `json:"round_id"`
	Commitment      Commitment      `json:"commitment"`
	ServerSeedHash  string          `json:"server_seed_hash"`
	ServerSeed      string          `json:"server_seed"`
	RevealState     string          `json:"reveal_state"`
	Verified        bool            `json:"verified"`
	CanonicalConfig string          `json:"canonical_config"`
	Result          GameRound       `json:"result"`
	BlackjackAudit  *BlackjackAudit `json:"blackjack_audit,omitempty"`
}
