package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/history"
	"github.com/cy4268/momiao/internal/historyaccess"
	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker"
	"github.com/cy4268/momiao/internal/roulette"
	"github.com/redis/go-redis/v9"
)

// Reuses the imported dev records and accounts; no gameplay or fixture writes.
func TestLocalOpsRecords(t *testing.T) {
	path := os.Getenv("MOMIAO_SESSION_DSN_FILE")
	if path == "" {
		t.Skip("requires existing isolated dev PostgreSQL and Redis")
	}
	dsn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read dev DSN")
	}
	store, err := platform.Open(t.Context(), strings.TrimSpace(string(dsn)))
	if err != nil {
		t.Fatal("open dev store")
	}
	defer store.Close()
	options, err := readSessionRedisConfig(os.Getenv("MOMIAO_SESSION_REDIS_CONFIG_FILE"))
	if err != nil {
		t.Fatal("read dev Redis configuration")
	}
	redisClient := redis.NewClient(options)
	defer redisClient.Close()
	auth := &identity.HTTP{Store: &identity.Store{Pool: store.Pool()}, Sessions: identity.Sessions{Redis: redisClient}, Origin: "https://dev.momiao.win"}
	var admin, regular int64
	if err = store.Pool().QueryRow(t.Context(), `SELECT min(id) FILTER(WHERE role>=10),min(id) FILTER(WHERE role<10) FROM identity.users WHERE status=1 AND deleted_at IS NULL`).Scan(&admin, &regular); err != nil {
		t.Fatal(err)
	}
	sessions := map[int64]identity.Session{}
	for _, id := range []int64{admin, regular} {
		s, e := auth.Sessions.Create(t.Context(), id)
		if e != nil {
			t.Fatal(e)
		}
		sessions[id] = s
		defer func() {
			if e := auth.Sessions.Revoke(context.Background(), s); e != nil {
				t.Error(e)
			}
		}()
	}
	reader, err := openPokerPool(t.Context(), os.Getenv("MOMIAO_HISTORY_READER_DSN_FILE"))
	if err != nil {
		t.Fatal("open existing history reader")
	}
	defer reader.Close()
	pokerPool, err := openPokerPool(t.Context(), os.Getenv("MOMIAO_HISTORY_POKER_DSN_FILE"))
	if err != nil {
		t.Fatal("open existing poker history reader")
	}
	defer pokerPool.Close()
	keys, err := readPokerStateKeys(os.Getenv("MOMIAO_HISTORY_POKER_KEYRING_FILE"))
	if err != nil {
		t.Fatal("read existing poker keys")
	}
	hands, err := poker.NewHistoryReader(pokerPool, keys)
	if err != nil {
		t.Fatal(err)
	}
	gameKeys, err := games.ReadKeyring(os.Getenv("MOMIAO_GAME_FAIRNESS_KEYRING_FILE"))
	if err != nil {
		t.Fatal("read existing game keys")
	}
	rounds, err := games.NewService(store, gameKeys)
	if err != nil {
		t.Fatal(err)
	}
	wheel, err := roulette.NewService(store, roulette.Keyring{Active: gameKeys.Active, Keys: gameKeys.Keys})
	if err != nil {
		t.Fatal(err)
	}
	historyHandler := &historyHTTP{list: history.NewReader(reader), wallet: store, rounds: rounds, poker: hands, roulette: wheel}
	var auditBefore, readAudits int
	if err = store.Pool().QueryRow(t.Context(), `SELECT count(*) FROM ops.audit_log WHERE actor_user_id=$1 AND action LIKE 'RECORD_READ_%'`, admin).Scan(&auditBefore); err != nil {
		t.Fatal(err)
	}
	for _, webDir := range []string{"", "/unused-records-web"} {
		cfg := config{PublicOrigin: auth.Origin, OpsEnvironment: "DEVELOPMENT", WebDir: webDir, Session: sessionConfig{Enabled: true}, localIdentity: auth, relay: store, history: historyHandler}
		h := newPortalHandler(cfg, catalogRoundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("records reached native"); return nil, nil }))
		call := func(user int64, method, path string, want int) *httptest.ResponseRecorder {
			t.Helper()
			r := httptest.NewRequest(method, path, nil)
			r.Header.Set("Origin", auth.Origin)
			r.Header.Set("New-Api-User", strconv.FormatInt(admin, 10))
			r.Header.Set("X-Auth-Session", "forged")
			if s, ok := sessions[user]; ok {
				r.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: s.Token})
				r.Header.Set("X-CSRF-Token", s.CSRFToken)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != want {
				t.Fatalf("local records %s %d %s; want %d", path, w.Code, w.Body.String(), want)
			}
			if user == admin && method == "GET" && strings.HasPrefix(path, "/api/v1/ops/records") && (want == 200 || want == 404 || want == 503) {
				readAudits++
			}
			return w
		}
		page := func(path string) history.Page {
			t.Helper()
			w := call(admin, "GET", path, 200)
			var envelope struct {
				Data history.Page `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			return envelope.Data
		}
		first := page("/api/v1/ops/records?limit=1")
		if len(first.Items) != 1 || !first.HasMore || first.NextCursor == nil {
			t.Fatal("global records not paginated")
		}
		call(regular, "GET", "/api/v1/ops/records", 403)
		call(0, "GET", "/api/v1/ops/records", 401)
		call(admin, "POST", "/api/v1/ops/records", 405)
		call(admin, "GET", "/api/v1/ops/records?user=-1", 400)
		call(admin, "GET", "/api/v1/ops/records?authz_epoch=1", 400)
		call(admin, "GET", "/api/v1/ops/records?user=1&user=2", 400)
		p1 := page("/api/v1/ops/records?record_type=POKER_HAND&limit=1")
		p2 := page("/api/v1/ops/records?record_type=POKER_HAND&limit=1&cursor=" + url.QueryEscape(*p1.NextCursor))
		if len(p2.Items) != 1 || p1.Items[0].SourceID != p2.Items[0].SourceID || p1.Items[0].UserID == p2.Items[0].UserID {
			t.Fatal("same hand participants lost across global cursor")
		}
		item := p1.Items[0]
		filtered := page("/api/v1/ops/records?record_type=POKER_HAND&id=" + item.SourceID + "&user=" + item.UserID)
		if len(filtered.Items) != 1 || filtered.Items[0].UserID != item.UserID {
			t.Fatal("optional user filter")
		}
		call(admin, "GET", "/api/v1/ops/records?record_type=POKER_HAND&limit=1&user="+item.UserID+"&cursor="+url.QueryEscape(*p1.NextCursor), 400)
		readAudits++ // Valid read request with a cursor bound to another subject: audited, then rejected.
		for _, kind := range []struct{ recordType, path string }{{"DIRECT_PLAY_ROUND", "rounds"}, {"POKER_SESSION", "sessions"}, {"POKER_HAND", "hands"}, {"ROULETTE_ROUND", "roulette"}} {
			suffix := ""
			if kind.path == "rounds" {
				suffix = "&game_slug=dice"
			}
			p := page("/api/v1/ops/records?record_type=" + kind.recordType + "&limit=1" + suffix)
			if len(p.Items) != 1 {
				t.Fatal("existing dev records missing", kind.recordType)
			}
			v := p.Items[0]
			base := "/api/v1/ops/records/" + kind.path + "/" + v.SourceID
			want := 200
			// Imported encrypted records can lack their original keys in the isolated
			// dev environment. Prove that Ops preserves the source error, not that such
			// a detail passed acceptance. Dice/session/transaction must still read 200.
			if kind.path == "hands" || kind.path == "roulette" {
				subject, _ := strconv.ParseInt(v.UserID, 10, 64)
				access := historyaccess.Records(admin, subject, func(context.Context, int64, int64) error { return nil })
				var sourceErr error
				if kind.path == "hands" {
					_, sourceErr = hands.HandDetail(t.Context(), access, v.SourceID, poker.HistoryHandQuery{})
				} else {
					_, sourceErr = wheel.HistoryDetail(t.Context(), access, v.SourceID, roulette.HistoryQuery{})
				}
				if sourceErr != nil {
					if !errors.Is(sourceErr, historyaccess.ErrUnavailable) {
						t.Fatal("unexpected source error", sourceErr)
					}
					want = 503
					t.Logf("DECRYPTED_ACCEPTANCE_PENDING %s %s: existing domain reader %v; assert unchanged HTTP503, not successful detail", kind.path, v.SourceID, sourceErr)
				}
			}
			w := call(admin, "GET", base+"?user="+v.UserID, want)
			call(regular, "GET", base+"?user="+v.UserID, 403)
			call(admin, "GET", base, 400)
			call(admin, "GET", base+"?user=9223372036854775807", 404)
			if kind.path == "rounds" || kind.path == "roulette" {
				call(admin, "GET", base+"/verify?user="+v.UserID, want)
			}
			if kind.path == "hands" {
				var participant bool
				if err := pokerPool.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM poker.hand_participants WHERE hand_id=$1 AND newapi_user_id=$2)`, v.SourceID, admin).Scan(&participant); err != nil {
					t.Fatal(err)
				}
				if !participant {
					var e struct {
						Data struct {
							Record struct {
								Participants []struct {
									Hole []int `json:"hole_cards"`
								} `json:"participants"`
							} `json:"record"`
						} `json:"data"`
					}
					if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
						t.Fatal(err)
					}
					if want == 200 && len(e.Data.Record.Participants) == 0 {
						t.Fatal("missing hand participants")
					}
					for _, p := range e.Data.Record.Participants {
						if len(p.Hole) > 0 {
							t.Fatal("Ops gained another player's private cards")
						}
					}
					call(admin, "GET", base+"/verify?user="+v.UserID, 404)
				}
			}
		}
		var transaction, subject string
		if err = store.Pool().QueryRow(t.Context(), `SELECT transaction_id::text,newapi_user_id::text FROM economy.asset_transactions WHERE status='CONFIRMED' ORDER BY created_at DESC LIMIT 1`).Scan(&transaction, &subject); err != nil {
			t.Fatal(err)
		}
		call(admin, "GET", "/api/v1/ops/records/transactions/"+transaction+"?user="+subject, 200)
		call(regular, "GET", "/api/v1/history?limit=1", 200)
		call(regular, "GET", "/api/v1/history?user="+item.UserID, 400)
		if _, err := historyHandler.list.List(t.Context(), 0, history.Query{}); err == nil {
			t.Fatal("personal reader accepted global subject")
		}
		own, err := historyHandler.list.List(t.Context(), regular, history.Query{Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range own.Items {
			if v.UserID != strconv.FormatInt(regular, 10) {
				t.Fatal("personal reader exposed other users")
			}
		}
		if own.NextCursor != nil {
			if _, err := historyHandler.list.List(t.Context(), regular, history.Query{Limit: 1, Cursor: *own.NextCursor}); err != nil {
				t.Fatal("personal cursor changed", err)
			}
		}
	}
	var granted bool
	if err = reader.QueryRow(t.Context(), `SELECT has_function_privilege(current_user,'games.ops_history_list(bigint,jsonb)','EXECUTE')`).Scan(&granted); err != nil || granted {
		t.Fatal("personal reader gained global capability", err)
	}
	if _, err = reader.Exec(t.Context(), `SELECT games.history_list(0,'{}')`); err == nil {
		t.Fatal("personal SQL accepted global subject")
	}
	var auditAfter int
	if err = store.Pool().QueryRow(t.Context(), `SELECT count(*) FROM ops.audit_log WHERE actor_user_id=$1 AND action LIKE 'RECORD_READ_%'`, admin).Scan(&auditAfter); err != nil || auditAfter-auditBefore != readAudits {
		t.Fatalf("one durable audit per read: before=%d after=%d want=%d err=%v", auditBefore, auditAfter, readAudits, err)
	}
	t.Logf("durable read audits added: %d", readAudits)
	var audit int
	if err = store.Pool().QueryRow(t.Context(), `SELECT count(*) FROM ops.audit_log WHERE actor_user_id=$1 AND action='RECORD_READ_LIST' AND target_type='HISTORY_LIST' AND after_state->>'subject_user_id'='0'`, admin).Scan(&audit); err != nil || audit < 1 {
		t.Fatal("global access audit missing", err)
	}
	t.Log("both routers: global/filter/tied-participant cursor; Dice/session/transaction200; encrypted detail source outcomes preserved; own-history retained; role401/403, input400, nonparticipant proof404; read audits retained; no native")
}

func TestHistoryHTTPBoundary(t *testing.T) {
	var h *historyHTTP
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/other", 404},
		{"POST", historyPath, 405},
		{"GET", historyPath + "?user_id=123", 503},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Authorization", "Bearer fixture-not-an-identity")
		h.ServeHTTP(w, r)
		if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s %s: status %d", tc.method, tc.path, w.Code)
		}
		if tc.status == 405 && w.Header().Get("Allow") != http.MethodGet {
			t.Fatal("missing read-only method declaration")
		}
	}
}

func TestHistoryHTTPQuery(t *testing.T) {
	u, _ := url.Parse(historyPath + "?record_type=POKER_SESSION&mode=POKER&game_slug=poker&time_from=2026-09-01T00%3A00%3A00Z&time_to=2026-09-02T00%3A00%3A00Z&result=WIN&status=SETTLED&id=11111111-1111-4111-8111-111111111111&limit=25&cursor=opaque")
	kind, _, proof, q, err := parseHistoryRequest(u)
	if err != nil || kind != "list" || proof || historyLimit(q, "limit") != 25 || q.Get("cursor") != "opaque" {
		t.Fatalf("valid filters lost: %v", err)
	}
	for _, raw := range []string{"user_id=1", "mode+game_slug=POKER", "limit=1&limit=2", "limit=101", "limit=01", "cursor=%zz", "status=SETTLED;user_id=1"} {
		u := &url.URL{Path: historyPath, RawQuery: raw}
		if _, _, _, _, err := parseHistoryRequest(u); err == nil {
			t.Fatalf("accepted ambiguous or untrusted query %q", raw)
		}
	}
}

func TestHistoryHTTPDetailRoutes(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	for _, part := range []string{"rounds/" + id, "rounds/" + id + "/verify", "hands/" + id + "/verify", "sessions/" + id + "?funding_limit=20&hand_cursor=opaque", "hands/" + id + "?limit=20"} {
		u, _ := url.Parse(historyPath + "/" + part)
		if _, got, _, _, err := parseHistoryRequest(u); err != nil || got != id {
			t.Fatalf("valid detail route %q: %v", part, err)
		}
	}
	for _, part := range []string{"rounds/not-an-id", "rounds/" + id + "?subject=2", "hands/" + id + "/verify?cursor=opaque", "sessions/" + id + "/", "records/" + id} {
		u, _ := url.Parse(historyPath + "/" + part)
		if _, _, _, _, err := parseHistoryRequest(u); err == nil {
			t.Fatalf("accepted unknown detail route %q", part)
		}
	}
}
