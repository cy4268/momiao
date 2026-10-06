package roulette

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"
)

// A private field added to the public DTO, a lost nullable field, or a numeric
// int64 on the wire breaks the service-only public contract.
func TestPublicRoomJSONWhitelist(t *testing.T) {
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	v := PublicRoomView{ID: "019a6000-0000-7000-8000-000000000031", Game: "devil-roulette",
		Title: "Public room", Version: 9007199254740993, Sequence: 23, State: "PLAYING",
		TargetPlayers: 2, StakeUnits: 5000000, PoolUnits: 10000000, ServerNow: now,
		Players: []PlayerView{{Seat: 0, Name: "Player", Ready: true, Alive: true, HP: 4, ItemCount: 1}},
		Log:     []PublicEvent{{Sequence: 23, At: now, Kind: "DEVIL_ITEM", Text: "Public event"}},
		Devil:   &DevilView{Remaining: 6}}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var publicJSON map[string]json.RawMessage
	if err = json.Unmarshal(raw, &publicJSON); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"self", "actions", "economic_policy", "economy_settlement", "quote"} {
		if _, ok := publicJSON[k]; ok {
			t.Fatalf("private field: %s", k)
		}
	}
	want := []string{"id", "game", "title", "version", "sequence", "state", "target_players", "stake_units", "pool_units", "binding", "server_seed_hash", "turn_seat", "server_now", "deadline", "game_deadline", "players", "log", "devil", "pressure"}
	got := make([]string, 0, len(publicJSON))
	for k := range publicJSON {
		got = append(got, k)
	}
	slices.Sort(want)
	slices.Sort(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("public fields: %v", got)
	}
	for k, value := range map[string]string{"version": `"9007199254740993"`, "sequence": `"23"`, "stake_units": `"5000000"`, "pool_units": `"10000000"`, "target_players": "2", "turn_seat": "null", "deadline": "null", "game_deadline": "null", "pressure": "null"} {
		if string(publicJSON[k]) != value {
			t.Fatalf("%s=%s want %s", k, publicJSON[k], value)
		}
	}
	var devil map[string]json.RawMessage
	if err = json.Unmarshal(publicJSON["devil"], &devil); err != nil {
		t.Fatal(err)
	}
	if string(devil["live"]) != "null" || string(devil["blank"]) != "null" {
		t.Fatal("hidden shell counts must remain null")
	}
	t.Run("invalid_id_before_storage", func(t *testing.T) {
		s := &Service{}
		for _, id := range []string{"", "not-a-uuid", "019a6000-0000-4000-8000-000000000031"} {
			if _, err := s.PublicView(context.Background(), id); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("id %q: %v", id, err)
			}
		}
	})
}
