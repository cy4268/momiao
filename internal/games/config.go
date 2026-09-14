package games

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"github.com/cy4268/momiao/internal/games/fairness"
	"math/big"
	"slices"
)

const (
	DiceAlgorithm = "dice-map-v1"
	// ScratchAlgorithm implements the IS §270 canonical multiset and layout.
	ScratchAlgorithm = "scratch-map-v1"
	SummonAlgorithm  = "summon-map-v1"
	SummonPool       = "SUMMON_MAIN_V1"
	prizeWeightTotal = 100000
)

// Prize uses exact integers. The supported v1 rules fix tier/multiplier pairs;
// new versioned tables may change weights and configured order, not payouts.
type Prize struct {
	Multiplier int64  `json:"multiplier"`
	Tier       string `json:"tier"`
	Weight     int64  `json:"weight"`
}

// ConfigBinding identifies the immutable mapping snapshot using the original
// IS §248 identifiers and IS §249 canonical hash envelope.
type ConfigBinding struct {
	Game              string
	Version           string
	Hash              [32]byte
	AlgorithmVersion  string
	RulesetVersion    string
	SchemaVersion     string
	PrizeTableVersion string
	PoolID            string
}

// Config can only be constructed by the validated constructors below. Caller
// slices are copied; a zero Config is invalid. No config registry is maintained.
type Config struct {
	binding    ConfigBinding
	prizes     []Prize
	diceCounts [3]int64 // SMALL, BIG, TRIPLE
	canonical  []byte
}

func (c Config) Binding() ConfigBinding { return c.binding }

func (c Config) Prizes() []Prize { return slices.Clone(c.prizes) }

func (c Config) CanonicalJSON() []byte { return slices.Clone(c.canonical) }

// Statistics contains exact fractions (not percentages). Top is zero for Dice.
// Each call returns fresh rational values owned by the caller.
type Statistics struct {
	RTP       *big.Rat
	Loss      *big.Rat
	BreakEven *big.Rat
	Win       *big.Rat
	Top       *big.Rat
}

func (c Config) Statistics() (Statistics, error) {
	if c.binding.Hash == ([32]byte{}) {
		return Statistics{}, ErrInvalidConfig
	}
	if c.binding.Game == "slot" || c.binding.Game == "blackjack" {
		return Statistics{}, ErrInvalidConfig // Requires the separately bound measured artifact.
	}
	if c.binding.Game == "dice" {
		wins := c.diceCounts[0]
		return Statistics{big.NewRat(2*wins, 216), big.NewRat(216-wins, 216), new(big.Rat), big.NewRat(wins, 216), new(big.Rat)}, nil
	}
	var payout, loss, even, win, top int64
	for _, p := range c.prizes {
		// v1 multipliers <= 100 and sum(weights)=100000; no product can overflow.
		payout += p.Weight * p.Multiplier
		switch {
		case p.Multiplier == 0:
			loss += p.Weight
		case p.Multiplier == 1:
			even += p.Weight
		default:
			win += p.Weight
		}
		if p.Tier == "TOP" || (c.binding.Game == "summon" && p.Tier == "T5") {
			top += p.Weight
		}
	}
	return Statistics{big.NewRat(payout, prizeWeightTotal), big.NewRat(loss, prizeWeightTotal), big.NewRat(even, prizeWeightTotal), big.NewRat(win, prizeWeightTotal), big.NewRat(top, prizeWeightTotal)}, nil
}

// DiceV1 exposes no configurable product-rule fields: three d6, ranges 4..10
// and 11..17, both choices lose to triples, win total payout 2x. Construction
// exhaustively validates all 216 outcomes as required by TD §303 / IS §268.
func DiceV1(version string) (Config, error) {
	c := Config{binding: ConfigBinding{
		Game: "dice", Version: version, AlgorithmVersion: DiceAlgorithm,
		RulesetVersion: "dice-rules-v1", SchemaVersion: "dice-config-v1",
	}}
	for a := uint8(1); a <= 6; a++ {
		for b := uint8(1); b <= 6; b++ {
			for d := uint8(1); d <= 6; d++ {
				switch classifyDice([3]uint8{a, b, d}) {
				case Small:
					c.diceCounts[0]++
				case Big:
					c.diceCounts[1]++
				case Triple:
					c.diceCounts[2]++
				}
			}
		}
	}
	if c.diceCounts != [3]int64{105, 105, 6} {
		return Config{}, ErrInvalidConfig
	}
	payload := map[string]any{
		"allowed_choices": []string{"BIG", "SMALL"},
		"big_range":       [2]int{11, 17}, "small_range": [2]int{4, 10},
		"dice_count": 3, "faces_per_die": 6, "triple_rule": "BOTH_LOSE",
		"win_total_payout_multiplier": 2,
	}
	return sealConfig(c, payload)
}

func scratchPrizesV1() []Prize {
	return []Prize{
		{0, "LOSS", 54000}, {1, "BREAK_EVEN", 19500}, {2, "T2", 18500},
		{3, "T3", 5000}, {5, "T5", 2000}, {10, "T10", 800},
		{25, "T25", 180}, {100, "TOP", 20},
	}
}

func summonPrizesV1() []Prize {
	return []Prize{
		{0, "T0", 59850}, {1, "T1", 25000}, {2, "T2", 10000},
		{5, "T3", 4000}, {20, "T4", 1050}, {100, "T5", 100},
	}
}

func ScratchV1(version string) (Config, error) {
	return NewScratchConfig(version, "scratch-prize-v1", scratchPrizesV1())
}

func SummonV1(version string) (Config, error) {
	return NewSummonConfig(version, "summon-prize-v1", summonPrizesV1())
}

// NewScratchConfig validates the complete table, including zero-weight tiers.
// The reserved scratch-prize-v1 identity accepts only the exact frozen table.
func NewScratchConfig(version, prizeTableVersion string, prizes []Prize) (Config, error) {
	c := Config{binding: ConfigBinding{
		Game: "scratch", Version: version, AlgorithmVersion: ScratchAlgorithm,
		RulesetVersion: "scratch-rules-v1", SchemaVersion: "scratch-config-v1",
		PrizeTableVersion: prizeTableVersion,
	}}
	return newPrizeConfig(c, prizes, scratchPrizesV1(), "scratch-prize-v1")
}

// NewSummonConfig keeps the v1 single pool, fixed multipliers and independent
// draws. Only weights/order may vary under a new prize-table version.
func NewSummonConfig(version, prizeTableVersion string, prizes []Prize) (Config, error) {
	c := Config{binding: ConfigBinding{
		Game: "summon", Version: version, AlgorithmVersion: SummonAlgorithm,
		RulesetVersion: "summon-rules-v1", SchemaVersion: "summon-config-v1",
		PrizeTableVersion: prizeTableVersion, PoolID: SummonPool,
	}}
	return newPrizeConfig(c, prizes, summonPrizesV1(), "summon-prize-v1")
}

func newPrizeConfig(c Config, prizes, frozen []Prize, frozenVersion string) (Config, error) {
	if !validVersion(c.binding.PrizeTableVersion) || len(prizes) != len(frozen) {
		return Config{}, ErrInvalidConfig
	}
	if c.binding.PrizeTableVersion == frozenVersion && !slices.Equal(prizes, frozen) {
		return Config{}, ErrInvalidConfig
	}
	seen := make(map[string]bool, len(prizes))
	var total int64
	for _, p := range prizes {
		if p.Weight < 0 || p.Weight > prizeWeightTotal || seen[p.Tier] {
			return Config{}, ErrInvalidConfig
		}
		matched := false
		for _, expected := range frozen {
			if p.Tier == expected.Tier && p.Multiplier == expected.Multiplier {
				matched = true
				break
			}
		}
		if !matched {
			return Config{}, ErrInvalidConfig
		}
		seen[p.Tier] = true
		total += p.Weight // At most 8 * 100000; bounded before adding.
	}
	if total != prizeWeightTotal {
		return Config{}, ErrInvalidConfig
	}
	c.prizes = slices.Clone(prizes)
	payload := map[string]any{"prizes": c.prizes, "prize_table_version": c.binding.PrizeTableVersion}
	if c.binding.Game == "summon" {
		payload["pool_id"] = c.binding.PoolID
		payload["draw_counts"] = []int{1, 10}
	}
	return sealConfig(c, payload)
}

func validVersion(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for i := range len(s) {
		b := s[i]
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.') {
			return false
		}
	}
	return true
}

func sealConfig(c Config, payload map[string]any) (Config, error) {
	if !validVersion(c.binding.Version) {
		return Config{}, ErrInvalidConfig
	}
	payload["config_version"] = c.binding.Version
	payload["ruleset_version"] = c.binding.RulesetVersion
	// All payload values are typed integers/ASCII strings/arrays. Map keys are
	// sorted by encoding/json; Prize fields are declared in ascending key order.
	canonical, err := json.Marshal(payload)
	if err != nil {
		return Config{}, ErrInvalidConfig
	}
	b := []byte("CHALDEA-GAME-CONFIG-V1\x00")
	b = appendLP16(b, c.binding.Game)
	b = appendLP16(b, c.binding.SchemaVersion)
	b = appendLP16(b, c.binding.AlgorithmVersion)
	b = append(b, canonical...)
	c.binding.Hash = sha256.Sum256(b)
	c.canonical = canonical
	return c, nil
}

func appendLP16(dst []byte, s string) []byte {
	dst = binary.BigEndian.AppendUint16(dst, uint16(len(s)))
	return append(dst, s...)
}

func (c Config) checkRound(r FairRound, game string) error {
	b := c.binding
	if b.Hash == ([32]byte{}) || b.Game != game {
		return ErrInvalidConfig
	}
	if r.Game != b.Game || r.AlgorithmVersion != b.AlgorithmVersion || r.ConfigVersion != b.Version || r.ConfigHash != b.Hash {
		return ErrConfigMismatch
	}
	return nil
}

func (c Config) prizeAt(sample uint64) (Prize, error) {
	if sample >= prizeWeightTotal || len(c.prizes) == 0 {
		return Prize{}, ErrInvalidInput
	}
	var cumulative uint64
	for _, p := range c.prizes {
		cumulative += uint64(p.Weight)
		if sample < cumulative {
			return p, nil
		}
	}
	return Prize{}, ErrInvalidConfig
}

func (c Config) samplePrize(seed []byte, r FairRound, domain string) (Prize, error) {
	s, err := fairness.NewStream(seed, r, domain)
	if err != nil {
		return Prize{}, err
	}
	n, err := fairness.UniformInt(s, prizeWeightTotal)
	if err != nil {
		return Prize{}, err
	}
	return c.prizeAt(n)
}
