// Package slot implements the frozen IS-06 sections 273-276 rules. It neither
// generates seeds nor performs wallet, storage, or presentation operations.
package slot

import (
	"errors"
	"fmt"
	"math"
)

const (
	ImplementationKey         = "direct.slot.v1"
	RulesetVersion            = "slot-rules-v1"
	AlgorithmVersion          = "slot-map-v1"
	ConfigSchemaVersion       = "slot-config-v1"
	ReelStripVersion          = "slot-strips-v1"
	PaylineVersion            = "slot-paylines-v1"
	PaytableVersion           = "slot-paytable-v1"
	UnitsPerChip        int64 = 500_000
	// Maximum sum of line multipliers for the frozen strips (516.4 x wager).
	MaxRoundLineMultiplier int64 = 5164
)

type Symbol string

const (
	L1 Symbol = "L1"
	L2 Symbol = "L2"
	L3 Symbol = "L3"
	M1 Symbol = "M1"
	M2 Symbol = "M2"
	H1 Symbol = "H1"
	H2 Symbol = "H2"
	W  Symbol = "W"
)

var symbols = [8]Symbol{L1, L2, L3, M1, M2, H1, H2, W}
var domains = [5]string{"slot:reel:1", "slot:reel:2", "slot:reel:3", "slot:reel:4", "slot:reel:5"}

type Config struct {
	ReelStripVersion string        `json:"reel_strip_version"`
	PaylineVersion   string        `json:"payline_version"`
	PaytableVersion  string        `json:"paytable_version"`
	Symbols          [8]Symbol     `json:"symbols"`
	ReelStrips       [5][32]Symbol `json:"reel_strips"`
	Paylines         [10][5]int    `json:"paylines"`
	// Rows follow Symbols; columns are lengths 3, 4, 5.
	Paytable [8][3]int64 `json:"paytable"`
}

// FrozenConfig returns a value copy, not writable aliases to the rule authority.
// Shared config code owns canonical JSON, envelope/version binding and hashing.
func FrozenConfig() Config {
	return Config{
		ReelStripVersion: ReelStripVersion, PaylineVersion: PaylineVersion, PaytableVersion: PaytableVersion, Symbols: symbols,
		ReelStrips: [5][32]Symbol{
			{L3, L2, L3, M1, L2, L1, L3, L2, M2, H2, L1, M2, L3, M1, H1, L2, L1, H1, L1, M2, L2, L3, L1, M1, W, L1, L2, L1, L2, H2, M1, L1},
			{L2, M2, L1, L2, L3, L2, W, L1, H1, L1, M1, L3, M1, H2, M1, L2, L1, L3, L1, L3, H1, L1, H2, L3, L1, L2, M2, L2, L1, L2, M1, M2},
			{L2, L3, L1, M2, L1, H1, L3, L1, M1, M2, L2, M1, L3, L1, L2, L3, H2, L2, L1, M1, L2, L1, H1, L2, L1, H2, M1, L3, L1, L2, M2, W},
			{M1, L3, W, L2, L1, H2, L2, M2, L1, M1, L1, L3, H1, L2, L3, M2, H1, L1, L2, L3, M1, L1, L3, L1, L2, H2, L1, M1, M2, L2, L1, L2},
			{L1, H2, L2, L1, L3, L2, L3, L1, H1, L2, L1, L2, M1, L1, W, H2, M1, L2, H1, L1, L2, L1, M1, M2, L3, M1, L2, L1, M2, L3, M2, L3},
		},
		Paylines: [10][5]int{{1, 1, 1, 1, 1}, {0, 0, 0, 0, 0}, {2, 2, 2, 2, 2}, {0, 1, 2, 1, 0}, {2, 1, 0, 1, 2}, {0, 0, 1, 2, 2}, {2, 2, 1, 0, 0}, {1, 0, 0, 0, 1}, {1, 2, 2, 2, 1}, {2, 1, 1, 1, 0}},
		Paytable: [8][3]int64{{4, 15, 50}, {8, 25, 80}, {10, 40, 150}, {15, 60, 250}, {25, 100, 500}, {50, 250, 1000}, {100, 500, 2500}, {125, 1000, 5000}},
	}
}

type LineResult struct {
	LineNumber     int    `json:"line_number"`
	Symbol         Symbol `json:"interpreted_symbol"`
	MatchLength    int    `json:"match_length"`
	Multiplier     int64  `json:"multiplier"`
	LineStakeUnits int64  `json:"line_stake_units,string"`
	PayoutUnits    int64  `json:"line_payout_units,string"`
}
type Result struct {
	Stops            [5]int         `json:"stops"`
	Grid             [5][3]Symbol   `json:"full_grid"`
	Lines            [10]LineResult `json:"lines"`
	TotalWagerUnits  int64          `json:"total_wager_units,string"`
	LineStakeUnits   int64          `json:"line_stake_units,string"`
	TotalPayoutUnits int64          `json:"total_payout_units,string"`
	NetChangeUnits   int64          `json:"net_change_units,string"`
	Class            string         `json:"result_class"`
	Detail           string         `json:"result_detail"`
}

var ErrWager = errors.New("SLOT_INVALID_WAGER")
var ErrOverflow = errors.New("SLOT_INTEGER_OVERFLOW")

func validateWager(wager int64) error {
	if wager < 10*UnitsPerChip || wager%UnitsPerChip != 0 {
		return ErrWager
	}
	if wager/10 > math.MaxInt64/MaxRoundLineMultiplier {
		return ErrOverflow
	}
	return nil
}

// Spin draws once per independently bound domain; sample must implement shared
// IS-06 UniformInt. It never falls back to a local or client random generator.
func Spin(wagerUnits int64, sample func(domain string, n uint32) (uint32, error)) (Result, error) {
	if e := validateWager(wagerUnits); e != nil {
		return Result{}, e
	}
	if sample == nil {
		return Result{}, errors.New("SLOT_MISSING_SAMPLER")
	}
	var stops [5]int
	for i, domain := range domains {
		n, e := sample(domain, 32)
		if e != nil {
			return Result{}, e
		}
		if n >= 32 {
			return Result{}, errors.New("SLOT_SAMPLE_OUT_OF_RANGE")
		}
		stops[i] = int(n)
	}
	return Resolve(wagerUnits, stops)
}

// Resolve is deterministic and uses exact atomic-unit arithmetic. It emits all
// ten line records, including zeros, in the frozen order.
func Resolve(wagerUnits int64, stops [5]int) (Result, error) {
	if e := validateWager(wagerUnits); e != nil {
		return Result{}, e
	}
	c := FrozenConfig()
	r := Result{Stops: stops, TotalWagerUnits: wagerUnits, LineStakeUnits: wagerUnits / 10}
	for reel, stop := range stops {
		if stop < 0 || stop >= 32 {
			return Result{}, fmt.Errorf("SLOT_INVALID_STOP: reel %d", reel+1)
		}
		for row := 0; row < 3; row++ {
			r.Grid[reel][row] = c.ReelStrips[reel][(stop+row+31)%32]
		}
	}
	for i, line := range c.Paylines {
		var values [5]Symbol
		for reel, row := range line {
			values[reel] = r.Grid[reel][row]
		}
		result := evaluateLine(values)
		result.LineNumber = i + 1
		result.LineStakeUnits = r.LineStakeUnits
		if result.Multiplier > 0 && r.LineStakeUnits > math.MaxInt64/result.Multiplier {
			return Result{}, ErrOverflow
		}
		result.PayoutUnits = r.LineStakeUnits * result.Multiplier
		if r.TotalPayoutUnits > math.MaxInt64-result.PayoutUnits {
			return Result{}, ErrOverflow
		}
		r.TotalPayoutUnits += result.PayoutUnits
		r.Lines[i] = result
	}
	r.NetChangeUnits = r.TotalPayoutUnits - wagerUnits
	switch {
	case r.TotalPayoutUnits == 0:
		r.Class = "LOSS"
		r.Detail = "NO_WIN"
	case r.NetChangeUnits < 0:
		r.Class = "LOSS"
		r.Detail = "PARTIAL_RETURN"
	case r.NetChangeUnits == 0:
		r.Class = "BREAK_EVEN"
		r.Detail = "BREAK_EVEN"
	default:
		r.Class = "WIN"
		r.Detail = "WIN"
	}
	return r, nil
}

func evaluateLine(values [5]Symbol) LineResult {
	c := FrozenConfig()
	var best LineResult
	for i, symbol := range c.Symbols {
		count := 0
		for _, value := range values {
			if value != symbol && (value != W || symbol == W) {
				break
			}
			count++
		}
		if count < 3 {
			continue
		}
		multiplier := c.Paytable[i][count-3]
		// Equal-paying interpretations use frozen symbol order only for metadata;
		// the amount is paid exactly once.
		if multiplier > best.Multiplier {
			best = LineResult{Symbol: symbol, MatchLength: count, Multiplier: multiplier}
		}
	}
	return best
}
