package platform

import (
	"errors"
	"math"
	"testing"
)

func TestParseAmount(t *testing.T) {
	for _, tt := range []struct {
		text string
		want int64
	}{
		{"1", 500000}, {"0.0372", 18600}, {"0.000002", 1},
		{"1.000000", 500000}, {"0001.20", 600000}, {"0.999998", 499999},
		{"18446744073709.551614", math.MaxInt64},
	} {
		t.Run(tt.text, func(t *testing.T) {
			got, err := ParseAmount(tt.text)
			if err != nil || got != tt.want {
				t.Fatalf("ParseAmount(%q) = %d, %v; want %d", tt.text, got, err, tt.want)
			}
		})
	}
}

func TestParseAmountRejectsInvalidOrLossy(t *testing.T) {
	for _, text := range []string{"", "0", "0.000000", "-1", "+1", "1e2", "NaN", "Infinity", " 1", "1 ", "1\n", ".5", "1.", "1.2.3", "１", "1_000", "1.0000000"} {
		if _, err := ParseAmount(text); !errors.Is(err, ErrAmountInvalid) {
			t.Errorf("%q: got %v, want invalid", text, err)
		}
	}
	for _, text := range []string{"0.000001", "1.000003", "0.999999"} {
		if _, err := ParseAmount(text); !errors.Is(err, ErrAmountNotRepresentable) {
			t.Errorf("%q: got %v, want not representable", text, err)
		}
	}
	for _, text := range []string{"18446744073709.551616", "18446744073710", "9223372036854775808", "999999999999999999999999999999999"} {
		if _, err := ParseAmount(text); !errors.Is(err, ErrAmountOutOfRange) {
			t.Errorf("%q: got %v, want out of range", text, err)
		}
	}
}

func TestFormatAmount(t *testing.T) {
	for _, tt := range []struct {
		units int64
		text  string
	}{
		{0, "0"}, {1, "0.000002"}, {18600, "0.0372"}, {500000, "1"}, {600000, "1.2"},
		{-1, "-0.000002"}, {math.MaxInt64, "18446744073709.551614"}, {math.MinInt64, "-18446744073709.551616"},
	} {
		if got := FormatAmount(tt.units); got != tt.text {
			t.Errorf("FormatAmount(%d) = %q; want %q", tt.units, got, tt.text)
		}
	}
}

func FuzzAmountRoundTrip(f *testing.F) {
	for _, n := range []int64{1, 2, 18600, 499999, 500000, math.MaxInt64} {
		f.Add(n)
	}
	f.Fuzz(func(t *testing.T, n int64) {
		if n <= 0 {
			return
		}
		text := FormatAmount(n)
		got, err := ParseAmount(text)
		if err != nil || got != n {
			t.Fatalf("round trip %d via %q = %d, %v", n, text, got, err)
		}
	})
}
