package main

import (
	"context"
	"testing"
)

func TestBotGamesIdentity(t *testing.T) {
	for _, subject := range []string{"", "0", "01", "+1", " 1", "18446744073709551616", "1' OR true--"} {
		t.Run(subject, func(t *testing.T) {
			_, e := (botGamesResolver{}).Resolve(context.Background(), subject)
			if e == nil || e.Error() != "INVALID_REQUEST" {
				t.Fatalf("noncanonical subject was not rejected: %v", e)
			}
		})
	}
	t.Run("unconfigured", func(t *testing.T) {
		_, e := (botGamesResolver{}).Resolve(context.Background(), "1")
		if e == nil || e.Error() != "UPSTREAM_UNAVAILABLE" {
			t.Fatalf("missing stores not fail-closed: %v", e)
		}
	})
}
