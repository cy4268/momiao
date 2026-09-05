package platform

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

const UnitsPerCredit int64 = 500000

var (
	ErrAmountInvalid          = errors.New("AMOUNT_INVALID")
	ErrAmountNotRepresentable = errors.New("AMOUNT_NOT_REPRESENTABLE")
	ErrAmountOutOfRange       = errors.New("AMOUNT_OUT_OF_RANGE")
)

// ParseAmount accepts a positive decimal with at most six fractional digits.
// Two millionths equal one atomic unit. Never round or pass through a float.
func ParseAmount(text string) (int64, error) {
	whole, fraction, hasDot := strings.Cut(text, ".")
	if whole == "" || (hasDot && (fraction == "" || len(fraction) > 6)) {
		return 0, ErrAmountInvalid
	}
	for _, part := range []string{whole, fraction} {
		for i := range len(part) {
			if part[i] < '0' || part[i] > '9' {
				return 0, ErrAmountInvalid
			}
		}
	}
	w, err := strconv.ParseUint(whole, 10, 64)
	if err != nil {
		return 0, ErrAmountOutOfRange
	}
	var micros uint64
	for i := range len(fraction) {
		micros = micros*10 + uint64(fraction[i]-'0')
	}
	for i := len(fraction); i < 6; i++ {
		micros *= 10
	}
	if micros%2 != 0 {
		return 0, ErrAmountNotRepresentable
	}
	f := micros / 2
	if w > (math.MaxInt64-f)/uint64(UnitsPerCredit) {
		return 0, ErrAmountOutOfRange
	}
	units := w*uint64(UnitsPerCredit) + f
	if units == 0 {
		return 0, ErrAmountInvalid
	}
	return int64(units), nil
}

// FormatAmount returns a canonical decimal, including zero and signed ledger deltas.
func FormatAmount(units int64) string {
	n := uint64(units)
	sign := ""
	if units < 0 {
		sign = "-"
		n = uint64(-(units + 1)) + 1 // Also handles MinInt64 without signed overflow.
	}
	whole := sign + strconv.FormatUint(n/uint64(UnitsPerCredit), 10)
	fraction := n % uint64(UnitsPerCredit) * 2
	if fraction == 0 {
		return whole
	}
	digits := strconv.FormatUint(fraction, 10)
	return whole + "." + strings.TrimRight(strings.Repeat("0", 6-len(digits))+digits, "0")
}
