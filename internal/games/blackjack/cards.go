package blackjack

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
)

const (
	ImplementationKey             = "direct.blackjack.v1"
	RulesetVersion                = "blackjack-rules-v1"
	AlgorithmVersion              = "blackjack-map-v1"
	ConfigSchemaVersion           = "blackjack-config-v1"
	ShuffleAlgorithmVersion       = "blackjack-fy-v1"
	ShuffleDomain                 = "blackjack:shuffle"
	FairRulesetVersion            = "blackjack-rules-v2"
	FairConfigSchemaVersion       = "blackjack-config-v2"
	FairReturnVersion             = "blackjack-fair-return-v1"
	FairReturnDomain              = "blackjack:fair-return:v1"
	FairReturnNumerator     int64 = 37_079
	FairReturnDenominator   int64 = 10_000_000
	UnitsPerChip            int64 = 500_000
	MaxHands                      = 4
)

type Card struct {
	ShoeIndex  int
	InstanceID uint16
}
type Value struct {
	HardTotal int  `json:"hard_total"`
	BestTotal int  `json:"best_total"`
	Soft      bool `json:"is_soft"`
}

func point(instance uint16) int {
	r := int(instance % 13)
	if r >= 9 {
		return 10
	}
	return r + 1
}

func Evaluate(cards []Card) (Value, error) {
	var v Value
	hasAce := false
	if len(cards) > 312 {
		return v, errors.New("BLACKJACK_INVALID_CARDS")
	}
	for _, c := range cards {
		if c.InstanceID >= 312 {
			return Value{}, errors.New("BLACKJACK_INVALID_CARD")
		}
		p := point(c.InstanceID)
		v.HardTotal += p
		hasAce = hasAce || p == 1
	}
	v.BestTotal = v.HardTotal
	if hasAce && v.HardTotal+10 <= 21 {
		v.BestTotal += 10
		v.Soft = true
	}
	return v, nil
}

// Shuffle consumes the bound blackjack:shuffle stream, with UniformInt bounds
// 312, 311, ..., 2. Seed creation and rejection sampling belong to shared fairness.
func Shuffle(sample func(n uint32) (uint32, error)) ([312]uint16, error) {
	var shoe [312]uint16
	if sample == nil {
		return shoe, errors.New("BLACKJACK_MISSING_SAMPLER")
	}
	for i := range shoe {
		shoe[i] = uint16(i)
	}
	for i := len(shoe) - 1; i > 0; i-- {
		j, e := sample(uint32(i + 1))
		if e != nil {
			return [312]uint16{}, e
		}
		if j > uint32(i) {
			return [312]uint16{}, errors.New("BLACKJACK_SAMPLE_OUT_OF_RANGE")
		}
		shoe[i], shoe[j] = shoe[j], shoe[i]
	}
	return shoe, nil
}

func ShoeHash(shoe [312]uint16) (string, error) {
	var seen [312]bool
	var serialized [624]byte
	for i, v := range shoe {
		if v >= 312 || seen[v] {
			return "", errors.New("BLACKJACK_INVALID_SHOE")
		}
		seen[v] = true
		binary.BigEndian.PutUint16(serialized[i*2:], v)
	}
	sum := sha256.Sum256(serialized[:])
	return hex.EncodeToString(sum[:]), nil
}
