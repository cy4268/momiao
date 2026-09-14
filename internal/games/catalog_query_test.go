package games

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCatalogQueryParsing(t *testing.T) {
	for _, tc := range []struct{ raw, q, availability, sort string }{
		{"", "", "ALL", "RECOMMENDED"},
		{"q=+Dice+&availability=PLAY&sort=NAME", "Dice", "PLAY", "NAME"},
		{"q=%E3%80%80%E9%AA%B0%E5%AD%90%C2%A0", "骰子", "ALL", "RECOMMENDED"},
		{"q=" + url.QueryEscape(strings.Repeat("🎲", 128)), strings.Repeat("🎲", 128), "ALL", "RECOMMENDED"},
		{"q=%EF%BB%BFDice&availability=ALL&sort=RECOMMENDED", "\ufeffDice", "ALL", "RECOMMENDED"},
		{"q=a%3Bb%26c%2Bd", "a;b&c+d", "ALL", "RECOMMENDED"},
	} {
		got, err := ParseCatalogQuery(tc.raw)
		if err != nil || got != (CatalogQuery{Q: tc.q, Availability: tc.availability, Sort: tc.sort}) {
			t.Errorf("%q: got %#v, %v", tc.raw, got, err)
		}
	}
	for _, raw := range []string{"q=%", "q=%G1", "q=%FF", "q=%C0%AF", "q=%ED%A0%80", "q=%F4%90%80%80", "q=x&q=y", "q=x&%71=y", "q=x;sort=NAME", "q=%00", "q=%09Dice", "q=%C2%85", "q=" + url.QueryEscape(strings.Repeat("🎲", 129)), "availability=", "availability=RESUME", "sort=", "sort=POPULAR", "sort=NAME&sort=NAME", "user_id=101", "Q=dice", "q=\xff"} {
		if _, err := ParseCatalogQuery(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestCatalogQueryDatabaseReadOnly(t *testing.T) {
	owner, runtime := gameTestStores(t)
	ctx := context.Background()
	var readOnly string
	if err := runtime.WithTx(ctx, func(tx pgx.Tx) error { return tx.QueryRow(ctx, `SHOW transaction_read_only`).Scan(&readOnly) }); err != nil || readOnly != "on" {
		t.Fatal("catalog acceptance requires a read-only runtime role", readOnly, err)
	}
	// The fixture installs the existing migrations in its own new database.
	// All behavior under test below is a catalog read; no account or wager is created.
	readCounts := func() [3]int64 {
		var counts [3]int64
		err := owner.WithTx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM games.game_rounds),(SELECT count(*) FROM economy.wallet_ledger),(SELECT count(*) FROM games.fairness_commitments)`).Scan(&counts[0], &counts[1], &counts[2])
		})
		if err != nil {
			t.Fatal(err)
		}
		return counts
	}
	before := readCounts()
	items, err := newTestService(t, runtime).Catalog(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 6 {
		t.Fatal("catalog omitted seeded registry entries", len(items))
	}
	query, err := ParseCatalogQuery("q=DICE&availability=PLAY&sort=NAME")
	if err != nil {
		t.Fatal(err)
	}
	match := FilterCatalog(items, query)
	if len(match) != 1 || match[0].Slug != "dice" || match[0].Config == nil {
		t.Fatalf("resolved config/filter mismatch: %#v", match)
	}
	query, _ = ParseCatalogQuery("q=missing-salon&sort=NAME")
	if got := FilterCatalog(items, query); got == nil || len(got) != 0 {
		t.Fatal("empty catalog not an array", got)
	}
	if after := readCounts(); after != before || before != [3]int64{} {
		t.Fatal("catalog read created private game state", before, after)
	}
	t.Logf("transaction_read_only=%s, public entries=%d, resolved Dice PLAY=1, empty match=[], private rows/ledger/commitments=%v", readOnly, len(items), before)
}

func TestCatalogPokerRuntime(t *testing.T) {
	owner, runtime := gameTestStores(t)
	ctx := context.Background()
	s := newTestService(t, runtime)
	baseline, err := s.Catalog(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	update := func(publication, configured, implementation string, maintenance bool) {
		t.Helper()
		if err := owner.WithTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "UPDATE games.game_registry SET publication_state=$1,configured_runtime_state=$2,implementation_key=$3 WHERE game_slug='texas-holdem'", publication, configured, implementation); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "UPDATE games.runtime_gate SET maintenance=$1", maintenance)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	var original [3]string
	var maintenance bool
	if err = owner.WithTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT publication_state,configured_runtime_state,implementation_key FROM games.game_registry WHERE game_slug='texas-holdem'").Scan(&original[0], &original[1], &original[2]); err != nil {
			return err
		}
		return tx.QueryRow(ctx, "SELECT maintenance FROM games.runtime_gate").Scan(&maintenance)
	}); err != nil {
		t.Fatal(err)
	}
	if original != [3]string{"COMING_SOON", "UNAVAILABLE", "poker.texas-holdem.v1"} || maintenance {
		t.Fatal("catalog tests require a fresh unmodified seed")
	}
	t.Cleanup(func() { update(original[0], original[1], original[2], maintenance) })
	for _, tc := range []struct {
		name, publication, configured, implementation, reply, want string
		calls                                                      int
		failure, nilReader, directMaintenance                      bool
	}{
		{"draft", "DRAFT", "AVAILABLE", "poker.texas-holdem.v1", "PLAY", "", 0, false, false, false},
		{"preview", "COMING_SOON", "AVAILABLE", "poker.texas-holdem.v1", "PLAY", "COMING_SOON", 0, false, false, false},
		{"retired", "RETIRED", "AVAILABLE", "poker.texas-holdem.v1", "PLAY", "RETIRED", 0, false, false, false},
		{"wrong implementation", "PUBLISHED", "AVAILABLE", "direct.texas-holdem.v1", "PLAY", "TEMPORARILY_UNAVAILABLE", 0, false, false, false},
		{"unavailable metadata", "PUBLISHED", "UNAVAILABLE", "poker.texas-holdem.v1", "PLAY", "TEMPORARILY_UNAVAILABLE", 0, false, false, false},
		{"registry is not poker maintenance", "PUBLISHED", "MAINTENANCE", "poker.texas-holdem.v1", "PLAY", "TEMPORARILY_UNAVAILABLE", 0, false, false, false},
		{"play", "PUBLISHED", "AVAILABLE", "poker.texas-holdem.v1", "PLAY", "PLAY", 1, false, false, false},
		{"real maintenance", "PUBLISHED", "AVAILABLE", "poker.texas-holdem.v1", "MAINTENANCE", "MAINTENANCE", 1, false, false, false},
		{"incomplete", "PUBLISHED", "AVAILABLE", "poker.texas-holdem.v1", "TEMPORARILY_UNAVAILABLE", "TEMPORARILY_UNAVAILABLE", 1, false, false, false},
		{"nil reader", "PUBLISHED", "AVAILABLE", "poker.texas-holdem.v1", "PLAY", "TEMPORARILY_UNAVAILABLE", 0, false, true, false},
		{"failed reader", "PUBLISHED", "AVAILABLE", "poker.texas-holdem.v1", "PLAY", "TEMPORARILY_UNAVAILABLE", 1, true, false, false},
		{"unknown response", "PUBLISHED", "AVAILABLE", "poker.texas-holdem.v1", "READY", "TEMPORARILY_UNAVAILABLE", 1, false, false, false},
		{"direct maintenance independent", "PUBLISHED", "AVAILABLE", "poker.texas-holdem.v1", "PLAY", "PLAY", 1, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			update(tc.publication, tc.configured, tc.implementation, tc.directMaintenance)
			calls := 0
			var read PokerCatalogRuntime = func(context.Context) (string, error) {
				calls++
				if tc.failure {
					return tc.reply, errors.New("private database detail")
				}
				return tc.reply, nil
			}
			if tc.nilReader {
				read = nil
			}
			items, err := s.Catalog(ctx, read)
			if err != nil {
				t.Fatal("Poker outage must not become whole catalog failure", err)
			}
			var found CatalogEntry
			direct := 0
			for _, item := range items {
				if item.Slug == "texas-holdem" {
					found = item
					continue
				}
				prior := baseline[direct]
				if tc.directMaintenance {
					prior.State = "MAINTENANCE"
				}
				if !reflect.DeepEqual(item, prior) {
					t.Fatal("existing direct state/config/order changed", item.Slug)
				}
				direct++
			}
			if direct != 5 || found.State != tc.want || found.Config != nil || calls != tc.calls {
				t.Fatalf("runtime=%s calls=%d direct=%d want=%s/%d", found.State, calls, direct, tc.want, tc.calls)
			}
			for _, availability := range []string{"PLAY", "MAINTENANCE"} {
				filtered := FilterCatalog(items, CatalogQuery{Q: "texas-holdem", Availability: availability, Sort: "RECOMMENDED"})
				if (len(filtered) == 1) != (tc.want == availability) {
					t.Fatal("filter did not use final public runtime", availability, filtered)
				}
			}
		})
	}
}

func TestCatalogQueryFilteringAndStableOrder(t *testing.T) {
	items := []CatalogEntry{
		{Slug: "zeta", Title: "alpha", State: "PLAY"},
		{Slug: "alpha", Title: "ALPHA", State: "MAINTENANCE"},
		{Slug: "dice", Title: "骰子 DICE", State: "PLAY"},
		{Slug: "future-card", Title: "新沙龙", State: "COMING_SOON"},
		{Slug: "retired", Title: "旧沙龙", State: "RETIRED"},
		{Slug: "unavailable", Title: "暂不可用", State: "TEMPORARILY_UNAVAILABLE"},
	}
	before := append([]CatalogEntry(nil), items...)
	slugs := func(entries []CatalogEntry) []string {
		out := []string{}
		for _, e := range entries {
			out = append(out, e.Slug)
		}
		return out
	}
	for _, tc := range []struct {
		raw  string
		want []string
	}{
		{"", []string{"zeta", "alpha", "dice", "future-card", "retired", "unavailable"}},
		{"q=DiCe", []string{"dice"}}, {"q=FUTURE", []string{"future-card"}},
		{"availability=PLAY&sort=NAME", []string{"zeta", "dice"}},
		{"q=alpha&sort=NAME", []string{"alpha", "zeta"}},
		{"q=alpha&availability=COMING_SOON", []string{}},
		{"availability=RETIRED", []string{"retired"}},
		{"availability=TEMPORARILY_UNAVAILABLE", []string{"unavailable"}},
	} {
		query, err := ParseCatalogQuery(tc.raw)
		if err != nil {
			t.Fatal(err)
		}
		got := FilterCatalog(items, query)
		if got == nil || !reflect.DeepEqual(slugs(got), tc.want) {
			t.Errorf("%q: got %v want %v", tc.raw, slugs(got), tc.want)
		}
	}
	if !reflect.DeepEqual(items, before) {
		t.Fatal("catalog sorting mutated resolver output")
	}
}
