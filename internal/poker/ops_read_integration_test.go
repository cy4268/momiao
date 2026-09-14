package poker

import (
	"context"
	"testing"
	"time"
)

func TestReadOpsTableWithEmptyChat(t *testing.T) {
	_, pool := v06PokerDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := New(Options{
		Pool:    pool,
		Keyring: Keyring{Current: "ops-read", Keys: map[string][]byte{"ops-read": make([]byte, 32)}},
		Leases:  &fixtureLeases{entries: map[string]leaseEntry{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	table, err := s.CreateTable(ctx, CreateTableCommand{
		UserID: 910001, Key: "ops-read-empty-chat", Name: "Ops empty chat",
		BlindPreset: "5-10", MaxSeats: 2, ChatEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := s.ReadOpsTable(ctx, table.TableID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Table.TableID != table.TableID || len(detail.Messages) != 0 {
		t.Fatal("empty table Ops detail differs")
	}
}
