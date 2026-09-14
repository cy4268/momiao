package main

import (
	"testing"
	"time"
)

func TestPokerPasswordConfigurationBoundary(t *testing.T) {
	values := map[string]string{}
	lookup := func(k string) (string, bool) { v, ok := values[k]; return v, ok }
	t.Run("absent is disabled", func(t *testing.T) {
		if cfg, err := readPokerPasswordConfig(lookup); err != nil || cfg != nil {
			t.Fatal("absence must not choose production defaults")
		}
	})
	values["POKER_TABLE_PASSWORD_ARGON2_MEMORY_KIB"] = "64"
	t.Run("partial is rejected", func(t *testing.T) {
		if _, err := readPokerPasswordConfig(lookup); err == nil {
			t.Fatal("partial configuration accepted")
		}
	})
	// Tiny costs are a parsing fixture only, never deployment recommendations.
	for key, value := range map[string]string{
		"ARGON2_TIME": "1", "ARGON2_PARALLELISM": "1",
		"MAX_CONCURRENT": "1", "MAX_WORK_MEMORY_KIB": "128",
		"OWNER_ATTEMPTS": "3", "OWNER_WINDOW_MS": "1000",
		"OWNER_TABLE_ATTEMPTS": "2", "OWNER_TABLE_WINDOW_MS": "2000",
		"VERIFY_PROFILES_JSON": `[{"memory_kib":128,"time":1,"parallelism":1}]`,
	} {
		values["POKER_TABLE_PASSWORD_"+key] = value
	}
	t.Run("complete is explicit", func(t *testing.T) {
		cfg, err := readPokerPasswordConfig(lookup)
		if err != nil || cfg == nil {
			t.Fatal("complete fixture policy rejected")
		}
		p := cfg.Policy
		if p.Current.MemoryKiB != 64 || p.Current.Time != 1 || p.Current.Parallelism != 1 || p.MaxConcurrent != 1 || p.MaxWorkMemoryKiB != 128 || len(p.VerifyProfiles) != 1 || p.VerifyProfiles[0].MemoryKiB != 128 {
			t.Fatal("configured current/profile/capacity not preserved")
		}
		if cfg.OwnerAttempts != 3 || cfg.OwnerWindow != time.Second || cfg.OwnerTableAttempts != 2 || cfg.OwnerTableWindow != 2*time.Second {
			t.Fatal("configured attempt limits not preserved")
		}
	})
}
