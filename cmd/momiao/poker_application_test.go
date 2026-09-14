package main

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPokerApplicationConstructorFailsClosedBeforeListening(t *testing.T) {
	app, err := openPokerApplication(context.Background(), config{})
	if err != nil || app != nil {
		t.Fatal("default-off created resources", err)
	}
	cfg := config{Poker: pokerConfig{Enabled: true}, ListenAddr: "127.0.0.1:0"}
	app, err = openPokerApplication(context.Background(), cfg)
	if err == nil || app != nil || strings.Contains(err.Error(), "password") {
		t.Fatal("missing real authorities accepted")
	}
	if err = run(context.Background(), cfg, log.New(io.Discard, "", 0)); err == nil || err.Error() != "poker startup failed" {
		t.Fatal("startup admitted an incomplete Poker application", err)
	}
}

func TestPokerApplicationPoolEndpointsFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name, dsn string
		valid     bool
	}{
		{"local", "host=127.0.0.1 port=55432 dbname=fixture user=runtime sslmode=disable", true},
		{"verified", "host=db.example.invalid port=5432 dbname=fixture user=runtime sslmode=verify-full", true},
		{"plaintext-remote", "host=198.51.100.2 dbname=fixture user=runtime sslmode=disable", false},
		{"remote-plaintext-fallback", "host=127.0.0.1,198.51.100.2 dbname=fixture user=runtime sslmode=disable", false},
		{"prefer", "host=127.0.0.1 dbname=fixture user=runtime sslmode=prefer", false},
		{"allow", "host=127.0.0.1 dbname=fixture user=runtime sslmode=allow", false},
		{"even-verified-multihost", "host=db1.example.invalid,db2.example.invalid dbname=fixture user=runtime sslmode=verify-full", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := pgxpool.ParseConfig(tc.dsn)
			if err != nil {
				t.Fatal("test parse failed")
			}
			if got := validPokerEndpoint(c); got != tc.valid {
				t.Fatalf("endpoint validity=%t want=%t", got, tc.valid)
			}
		})
	}
	a, _ := pgxpool.ParseConfig("host=127.0.0.1 port=55432 dbname=fixture user=auth sslmode=disable")
	b := a.Copy()
	b.ConnConfig.User = "poker"
	if !samePokerEndpoint(a, b) {
		t.Fatal("distinct runtime users on one endpoint rejected")
	}
	for _, change := range []func(*pgxpool.Config){func(c *pgxpool.Config) { c.ConnConfig.Port++ }, func(c *pgxpool.Config) { c.ConnConfig.Host = "127.0.0.2" }, func(c *pgxpool.Config) { c.ConnConfig.Database = "other" }} {
		c := b.Copy()
		change(c)
		if samePokerEndpoint(a, c) {
			t.Fatal("different instance/database endpoint admitted")
		}
	}
}
