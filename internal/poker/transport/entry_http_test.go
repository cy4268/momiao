package transport

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEntryReadHTTPBoundary(t *testing.T) {
	b := newSynthetic()
	h, err := New(b.opts()) // Both new read ports deliberately absent.
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	check := func(path, body, method, auth, origin string, want int) {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", auth)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("route=%s status=%d want=%d cache=%q body=%s", path, w.Code, want, w.Header().Get("Cache-Control"), body)
		}
	}
	entry := "/api/v1/poker/entry-receipt-query"
	reservation := "/api/v1/poker/tables/" + testTable + "/reservation-query"
	create := `{"kind":"create","mutation_id":"entry-original-key"}`
	seat := `{"reservation_id":"` + testOther + `"}`
	for _, route := range []struct{ path, body string }{{entry, create}, {reservation, seat}} {
		check(route.path, route.body, "POST", "Bearer synthetic-http", testOrigin, 503)
		check(route.path, route.body, "POST", "", testOrigin, 401)
		check(route.path, route.body, "POST", "Bearer synthetic-http", testOrigin+"/", 403)
		check(route.path, route.body, "POST", "Bearer synthetic-http", "", 403)
		check(route.path+"?", route.body, "POST", "Bearer synthetic-http", testOrigin, 400)
		check(route.path+"?mutation_id=secret", route.body, "POST", "Bearer synthetic-http", testOrigin, 400)
		check(route.path, route.body, "GET", "Bearer synthetic-http", testOrigin, 405)
		b.revoked = true
		check(route.path, route.body, "POST", "Bearer synthetic-http", testOrigin, 401)
		b.revoked = false
		for _, body := range []string{`{}`, `null`, route.body + `{}`, strings.TrimSuffix(route.body, "}") + `,"user_id":1}`, strings.TrimSuffix(route.body, "}") + `,"request_id":"entry-injected-key"}`} {
			check(route.path, body, "POST", "Bearer synthetic-http", testOrigin, 400)
		}
	}
	for _, body := range []string{
		`{"kind":"create"}`, `{"kind":"create","mutation_id":null}`, `{"kind":"create","mutation_id":"short"}`,
		`{"kind":"create","kind":"create","mutation_id":"entry-original-key"}`,
		`{"kind":"create","mutation_id":"entry-original-key","mutation_id":"entry-another-key"}`,
		`{"kind":"action","mutation_id":"entry-original-key"}`, `{"Kind":"create","mutation_id":"entry-original-key"}`,
		`{"kind":"create","mutation_id":"entry-original-key","table_id":null}`,
		`{"kind":"create","mutation_id":"entry-original-key","table_id":""}`,
		`{"kind":"create","mutation_id":"entry-original-key","table_id":"` + testTable + `"}`,
		`{"kind":"reserve","mutation_id":"entry-original-key"}`,
		`{"kind":"buyin","mutation_id":"entry-original-key","table_id":null}`,
		`{"kind":"buyin","mutation_id":"entry-original-key","table_id":"AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA"}`,
		`{"kind":"reserve","mutation_id":"entry-original-key","table_id":12}`,
		`{"kind":null,"mutation_id":"entry-original-key"}`,
		`{"kind":"create","mutation_id":"entry-original-key","table_id":"00000000-0000-0000-0000-000000000000"}`,
		`{"kind":"buyin","mutation_id":"entry-original-key","table_id":"` + testTable + `","table_id":"` + testTable + `"}`,
		`{"kind":"create","mutation_id":"entry-original-key","scope":"poker.create.v1"}`,
		`{"kind":"create","mutation_id":"` + strings.Repeat("a", 129) + `"}`,
	} {
		check(entry, body, "POST", "Bearer synthetic-http", testOrigin, 400)
	}
	for _, kind := range []string{"reserve", "buyin"} {
		check(entry, `{"kind":"`+kind+`","mutation_id":"entry-original-key","table_id":"`+testTable+`"}`, "POST", "Bearer synthetic-http", testOrigin, 503)
	}
	for _, body := range []string{`{"reservation_id":null}`, `{"reservation_id":"bad"}`, `{"reservation_id":"AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA"}`, `{"reservation_id":"` + testOther + `","reservation_id":"` + testOther + `"}`, `{"reservation_id":"` + testOther + `","lease_token":"secret"}`} {
		check(reservation, body, "POST", "Bearer synthetic-http", testOrigin, 400)
	}
	check("/api/v1/poker/tables/AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA/reservation-query", seat, "POST", "Bearer synthetic-http", testOrigin, 400)
	if b.authCalls != 0 || b.snapshotCalls != 0 || b.actionCalls != 0 || b.effects != 0 || len(b.controllers) != 0 {
		t.Fatal("read boundary entered ticket/snapshot/mutation/control")
	}
}
