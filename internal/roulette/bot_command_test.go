package roulette

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Invalid Bot intents must fail before opening a transaction, rather than
// resolving an unrelated receipt or treating an ambiguous request as absent.
func TestBotIntentValidation(t *testing.T) {
	create := CreateRequest{Key: "bot-intent-validation", Game: "devil-roulette", Stake: "10", Players: 2}
	command := Command{Key: "bot-intent-validation", ExpectedVersion: 1, Action: Action{Kind: "JOIN"}}
	const id = "019a6000-0000-7000-8000-000000000001"
	cases := []struct {
		name   string
		user   int64
		intent BotIntent
	}{
		{"missing_purpose", 1, BotIntent{Game: create.Game, Create: &create}},
		{"lowercase_purpose", 1, BotIntent{Purpose: "create", Game: create.Game, Create: &create}},
		{"missing_create", 1, BotIntent{Purpose: "CREATE", Game: create.Game}},
		{"create_and_command", 1, BotIntent{Purpose: "CREATE", Game: create.Game, Create: &create, Command: &command}},
		{"create_with_round", 1, BotIntent{Purpose: "CREATE", Game: create.Game, RoundID: id, Create: &create}},
		{"create_game_mismatch", 1, BotIntent{Purpose: "CREATE", Game: "pressure-roulette", Create: &create}},
		{"unknown_game", 1, BotIntent{Purpose: "COMMAND", Game: "dice", RoundID: id, Command: &command}},
		{"missing_command", 1, BotIntent{Purpose: "COMMAND", Game: create.Game, RoundID: id}},
		{"command_and_create", 1, BotIntent{Purpose: "COMMAND", Game: create.Game, RoundID: id, Create: &create, Command: &command}},
		{"command_without_round", 1, BotIntent{Purpose: "COMMAND", Game: create.Game, Command: &command}},
		{"invalid_actor", 0, BotIntent{Purpose: "CREATE", Game: create.Game, Create: &create}},
	}
	var service *Service
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := service.FindReceiptMatching(context.Background(), tc.user, tc.intent, time.Now())
			if !errors.Is(err, ErrInvalidInput) || got.Status != "" || got.Receipt != nil {
				t.Fatalf("lookup=%+v error=%v; want invalid input and no status", got, err)
			}
		})
	}
}

func TestBotGuardedInputValidationMatchesBrowser(t *testing.T) {
	ctx := context.Background()
	var service *Service
	const id = "019a6000-0000-7000-8000-000000000001"
	for _, tc := range []struct {
		name string
		in   CreateRequest
	}{
		{"key", CreateRequest{Key: "short", Game: "devil-roulette", Stake: "10", Players: 2}},
		{"stake", CreateRequest{Key: "bot-intent-validation", Game: "devil-roulette", Stake: "0", Players: 2}},
		{"players", CreateRequest{Key: "bot-intent-validation", Game: "devil-roulette", Stake: "10", Players: 3}},
		{"overflow", CreateRequest{Key: "bot-intent-validation", Game: "pressure-roulette", Stake: "9223372036854", Players: 6}},
	} {
		t.Run("create/"+tc.name, func(t *testing.T) {
			_, browserErr := service.Create(ctx, 1, tc.in)
			_, botErr := service.CreateBefore(ctx, 1, tc.in, time.Now())
			_, lookupErr := service.FindReceiptMatching(ctx, 1, BotIntent{Purpose: "CREATE", Game: tc.in.Game, Create: &tc.in}, time.Now())
			for _, err := range []error{browserErr, botErr, lookupErr} {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("invalid create accepted: %v", err)
				}
			}
		})
	}
	for _, tc := range []struct {
		name string
		in   Command
	}{
		{"key", Command{Key: "short", ExpectedVersion: 1, Action: Action{Kind: "JOIN"}}},
		{"version", Command{Key: "bot-intent-validation", Action: Action{Kind: "JOIN"}}},
		{"internal_action", Command{Key: "bot-intent-validation", ExpectedVersion: 1, Action: Action{Kind: "TIMEOUT"}}},
		{"missing_ready", Command{Key: "bot-intent-validation", ExpectedVersion: 1, Action: Action{Kind: "READY"}}},
		{"extra_ready", Command{Key: "bot-intent-validation", ExpectedVersion: 1, Action: Action{Kind: "JOIN"}, Ready: &ReadyConfirmation{ClientSeed: "seed"}}},
		{"invalid_seed", Command{Key: "bot-intent-validation", ExpectedVersion: 1, Action: Action{Kind: "READY"}, Ready: &ReadyConfirmation{ClientSeed: "\n"}}},
	} {
		t.Run("command/"+tc.name, func(t *testing.T) {
			_, browserErr := service.Command(ctx, 1, id, tc.in)
			_, botErr := service.CommandBefore(ctx, 1, id, tc.in, time.Now())
			_, lookupErr := service.FindReceiptMatching(ctx, 1, BotIntent{Purpose: "COMMAND", Game: "devil-roulette", RoundID: id, Command: &tc.in}, time.Now())
			for _, err := range []error{browserErr, botErr, lookupErr} {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("invalid command accepted: %v", err)
				}
			}
		})
	}
}
