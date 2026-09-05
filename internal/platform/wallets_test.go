package platform

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReadWalletsInvalidUser(t *testing.T) {
	s := &Store{}
	if _, err := s.ReadWallets(context.Background(), 0); !errors.Is(err, ErrInvalidMutation) {
		t.Fatal(err)
	}
}
func TestReadWalletsSnapshot(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	u := time.Now().UnixNano()
	ws, err := s.ReadWallets(ctx, u)
	if err != nil || len(ws) != 0 {
		t.Fatal(ws, err)
	}
	if _, err = s.pool.Exec(ctx, "INSERT INTO identity.account_refs(newapi_user_id) VALUES($1)", u); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, "INSERT INTO economy.wallet_balances(newapi_user_id,asset_type) VALUES($1,'RESERVE_API_CREDIT')", u); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadWallets(ctx, u); !errors.Is(err, ErrIncompleteWallets) {
		t.Fatal(err)
	}
	mustEnsure(t, s, u)
	ws, err = s.ReadWallets(ctx, u)
	if err != nil || len(ws) != 2 || ws[0].Asset != ReserveAPICredit || ws[1].Asset != AvailableChips {
		t.Fatal(ws, err)
	}
}

func TestOpenLazyDoesNotConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, err := OpenLazy(ctx, "host=127.0.0.1 port=1 user=wallet password=private dbname=wallet sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	for _, dsn := range []string{"", " ", "postgres://%invalid"} {
		if s, err := OpenLazy(ctx, dsn); err == nil {
			s.Close()
			t.Fatal("invalid DSN accepted")
		}
	}
}
