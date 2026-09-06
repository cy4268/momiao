package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cy4268/momiao/internal/platform"
)

type gateFixture struct {
	stage               string
	calls               []string
	ackUser, ackVersion int64
}

func (s *gateFixture) CatalogAuthority(context.Context, int64) (platform.AnnouncementPrincipal, error) {
	s.calls = append(s.calls, "models-role")
	if s.stage == "models-denied" {
		return platform.AnnouncementPrincipal{}, platform.ErrCatalogForbidden
	}
	return platform.AnnouncementPrincipal{Role: "OPERATOR"}, nil
}

func TestAccessGateCatalogRoutesAndScope(t *testing.T) {
	for _, route := range []string{"/api/access", "/api/access?model_id=%E7%BB%84%2Fmodel&intent=use", "/keys?model_id=a%3Fb%23c"} {
		if gateRouteDomain(route) != "API" {
			t.Errorf("valid model navigation rejected: %s", route)
		}
	}
	for _, route := range []string{"/api/access?intent=use", "/api/access?model_id=x&intent=delete", "/api/access?model_id=x&intent=use&intent=use", "/keys?model_id=x&intent=use", "/keys?model_id=x&model_id=y", "/api/access?model_id=x&token=secret", "/api/access?model_id=%FF", "/api/access?model_id=%GG", "/api/access?model_id=%20x", "/keys?model_id=x#fragment", "/api/access?"} {
		if gateRouteDomain(route) != "" {
			t.Errorf("unbounded model navigation accepted: %s", route)
		}
	}
	transport := walletTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":9007199254740993,"username":"synthetic-root","status":1,"role":100}}`))}, nil
	})
	for _, tc := range []struct{ stage, want string }{{"models-denied", "ROLE_DENIED"}, {"allowed", "READY"}} {
		store := &gateFixture{stage: tc.stage}
		w := httptest.NewRecorder()
		newAccessGateHandler("https://example.test", store, &accessDeclaration{Resources: map[string]string{"OPERATIONS": "AVAILABLE"}}, transport).ServeHTTP(w, announcementReq("GET", "/platform/v1/access-gate?route="+url.QueryEscape("/ops/models"), ""))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"stage":"`+tc.want+`"`) || strings.Join(store.calls, ",") != "master,migration,models-role" {
			t.Errorf("models scope/order lost: %d %s %v", w.Code, w.Body.String(), store.calls)
		}
	}
}

func (s *gateFixture) ReadProfile(context.Context, int64) (platform.Profile, error) {
	s.calls = append(s.calls, "master")
	if s.stage == "master" {
		return platform.Profile{Status: "INCOMPLETE"}, nil
	}
	return platform.Profile{Status: "COMPLETE"}, nil
}
func (s *gateFixture) ReadMigrationNotice(context.Context, int64, bool) (platform.MigrationNotice, error) {
	s.calls = append(s.calls, "migration")
	if s.stage == "migration" {
		return platform.MigrationNotice{State: "REQUIRED", RequiredVersion: 1}, nil
	}
	return platform.MigrationNotice{State: "NOT_REQUIRED"}, nil
}
func (s *gateFixture) AcknowledgeMigrationNotice(_ context.Context, user, version int64) (platform.MigrationNotice, error) {
	s.calls = append(s.calls, "ack")
	s.ackUser = user
	s.ackVersion = version
	return platform.MigrationNotice{UserID: user, State: "ACKNOWLEDGED", RequiredVersion: version, AcknowledgedVersion: version}, nil
}

func TestAccessDeclarationStrictSource(t *testing.T) {
	valid := `{"version":1,"environment":"STAGING","origin":"https://example.test","evidence_ref":"synthetic-reviewed-only","migration_applicability":"UNVERIFIED","resources":{"API":"AVAILABLE"}}`
	path := filepath.Join(t.TempDir(), "declaration.json")
	for _, raw := range []string{strings.Replace(valid, `"version":1`, `"version":1,"version":1`, 1), strings.Replace(valid, `"AVAILABLE"`, `"READY"`, 1), strings.Replace(valid, `"synthetic-reviewed-only"`, `" "`, 1), strings.Replace(valid, `"UNVERIFIED"`, `"FRESH_INSTALL"`, 1)} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadAccessDeclaration(path, "https://example.test"); err == nil {
			t.Fatalf("accepted invalid declaration: %s", raw)
		}
	}
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := loadAccessDeclaration(path, "https://example.test")
	if err != nil || d.MigrationApplicability != "UNVERIFIED" || d.Resources["ACCOUNT"] != "" {
		t.Fatal("explicit unverified source changed", err)
	}
	if _, err = loadAccessDeclaration(path, "https://wrong.test"); err == nil {
		t.Fatal("accepted different origin")
	}
}

func TestMigrationNoticeHTTPWriteBoundary(t *testing.T) {
	transport := walletTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":9007199254740993,"username":"synthetic-user","status":1,"role":1}}`))}, nil
	})
	for _, tc := range []struct {
		name, body, origin string
		want               int
	}{
		{"canonical version only", `{"version":"2"}`, "https://example.test", 200},
		{"caller supplies owner", `{"version":"2","user_id":"43"}`, "https://example.test", 400},
		{"duplicate version", `{"version":"2","version":"2"}`, "https://example.test", 400},
		{"numeric version", `{"version":2}`, "https://example.test", 400},
		{"foreign origin", `{"version":"2"}`, "https://wrong.test", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &gateFixture{}
			r := announcementReq("POST", "/platform/v1/migration-notice/acknowledge", tc.body)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			newAccessGateHandler("https://example.test", store, nil, transport).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
			if tc.want == 200 {
				if store.ackUser != 9007199254740993 || store.ackVersion != 2 || strings.Join(store.calls, ",") != "master,ack" {
					t.Fatalf("wrong write boundary: %+v", store)
				}
			} else if store.ackUser != 0 {
				t.Fatal("invalid write reached store")
			}
		})
	}
}
func (s *gateFixture) AnnouncementAuthority(context.Context, int64) (platform.AnnouncementPrincipal, error) {
	s.calls = append(s.calls, "role")
	if s.stage == "role" {
		return platform.AnnouncementPrincipal{}, platform.ErrAnnouncementForbidden
	}
	return platform.AnnouncementPrincipal{Role: "SUPER_ADMIN"}, nil
}

func TestAccessGateOrderAndMissingDeclaration(t *testing.T) {
	for _, tc := range []struct {
		stage       string
		status      int
		want, calls string
	}{{"account", 2, "ACCOUNT_RESTRICTED", ""}, {"master", 1, "MASTER_REQUIRED", "master"}, {"migration", 1, "MIGRATION_REQUIRED", "master,migration"}, {"role", 1, "ROLE_DENIED", "master,migration,role"}, {"resource", 1, "RESOURCE_UNVERIFIED", "master,migration,role"}, {"AVAILABLE", 1, "READY", "master,migration,role"}, {"MAINTENANCE", 1, "MAINTENANCE", "master,migration,role"}} {
		t.Run(tc.stage, func(t *testing.T) {
			store := &gateFixture{stage: tc.stage}
			transport := walletTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"success":true,"data":{"id":9007199254740993,"username":"synthetic-user","status":%d,"role":1}}`, tc.status)))}, nil
			})
			r := announcementReq("GET", "/platform/v1/access-gate?route=%2Fops%2Fannouncements", "")
			w := httptest.NewRecorder()
			var declaration *accessDeclaration
			if tc.stage == "AVAILABLE" || tc.stage == "MAINTENANCE" {
				declaration = &accessDeclaration{Resources: map[string]string{"OPERATIONS": tc.stage}}
			}
			newAccessGateHandler("https://example.test", store, declaration, transport).ServeHTTP(w, r)
			var result struct {
				Data struct {
					Stage string `json:"stage"`
				} `json:"data"`
			}
			if json.Unmarshal(w.Body.Bytes(), &result) != nil || w.Code != 200 || result.Data.Stage != tc.want {
				t.Fatalf("gate=%d %s", w.Code, w.Body.String())
			}
			if strings.Join(store.calls, ",") != tc.calls {
				t.Fatalf("wrong order: %v", store.calls)
			}
		})
	}
}
