package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// One bounded deployment acceptance, not an alternative grant implementation.
// Only the frozen psql templates grant schema 5-9 runtime/deployer privileges.
func jointRuntimeStores(t *testing.T) (owner, runtime, deployer *Store) {
	t.Helper()
	path := os.Getenv("MOMIAO_RUNTIME_JOINT_CONNECTION_FILE")
	if path == "" {
		t.Skip("explicit private local joint-acceptance connection file required")
	}
	var connection struct {
		Host, User, Password string
		Port                 int
	}
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &connection) != nil || connection.Host != "127.0.0.1" || connection.Port != 55432 || connection.User == "" || connection.Password == "" {
		t.Fatal("invalid private local test connection")
	}
	ctx := context.Background()
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(connection.Host, strconv.Itoa(connection.Port)), Path: "/postgres", User: url.UserPassword(connection.User, connection.Password), RawQuery: "sslmode=disable"}
	admin, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal("local joint-acceptance administrator connection failed", strings.ReplaceAll(err.Error(), connection.Password, "[redacted]"))
	}
	t.Cleanup(admin.Close)
	suffix := fmt.Sprint(time.Now().UnixNano())
	database := "m4_joint_" + suffix
	ownerRole, runtimeRole, deployerRole := "m4jo_"+suffix, "m4jr_"+suffix, "m4jd_"+suffix
	roles := []string{}
	createdDatabase := false
	t.Cleanup(func() {
		if createdDatabase {
			if _, err := admin.pool.Exec(ctx, "DROP DATABASE "+pgx.Identifier{database}.Sanitize()); err != nil {
				t.Error("isolated test database cleanup failed", err)
				return
			}
		}
		for _, role := range roles {
			if _, err := admin.pool.Exec(ctx, "DROP ROLE "+pgx.Identifier{role}.Sanitize()); err != nil {
				t.Error("isolated test role cleanup failed", err)
			}
		}
	})
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	password := hex.EncodeToString(secret)
	for _, role := range []string{ownerRole, runtimeRole, deployerRole} {
		login := "NOLOGIN"
		if role != ownerRole {
			login = "LOGIN PASSWORD '" + password + "'"
		}
		if _, err = admin.pool.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT "+login); err != nil {
			t.Fatal("synthetic role setup failed")
		}
		roles = append(roles, role)
	}
	if _, err = admin.pool.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{database}.Sanitize()+" OWNER "+pgx.Identifier{ownerRole}.Sanitize()); err != nil {
		t.Fatal("isolated test database creation failed")
	}
	createdDatabase = true
	u.Path = "/" + database
	cfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal("owner configuration failed")
	}
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE "+pgx.Identifier{ownerRole}.Sanitize())
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("owner pool failed")
	}
	owner = &Store{pool: pool}
	t.Cleanup(owner.Close)
	if err = owner.Migrate(ctx); err != nil {
		t.Fatal("frozen migrations under non-login owner failed", err)
	}
	// Exact read-only observed 1-4 effective privileges, not a permissive fixture.
	baseline, err := os.ReadFile("testdata/runtime-baseline-0001-0004.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.pool.Exec(ctx, strings.ReplaceAll(string(baseline), `:"runtime_role"`, pgx.Identifier{runtimeRole}.Sanitize())); err != nil {
		t.Fatal("observed 1-4 baseline failed", err)
	}
	fixture := fmt.Sprintf(`REVOKE ALL ON DATABASE %[1]s FROM PUBLIC;
 GRANT CONNECT ON DATABASE %[1]s TO %[2]s,%[3]s;
 CREATE TABLE public.users(id bigint PRIMARY KEY,quota bigint NOT NULL);
 INSERT INTO public.users VALUES(1,77);`, pgx.Identifier{database}.Sanitize(), pgx.Identifier{runtimeRole}.Sanitize(), pgx.Identifier{deployerRole}.Sanitize())
	if _, err = owner.pool.Exec(ctx, fixture); err != nil {
		t.Fatal("schema 1-4 baseline setup failed", err)
	}
	apply := func(file string, enabled bool) {
		t.Helper()
		psql := os.Getenv("MOMIAO_TEST_PSQL")
		if !filepath.IsAbs(psql) {
			t.Fatal("explicit absolute psql path required")
		}
		args := []string{"-X", "--no-password", "-h", connection.Host, "-p", strconv.Itoa(connection.Port), "-U", connection.User, "-d", database, "--set=schema_owner=" + ownerRole, "--set=runtime_role=" + runtimeRole, "--set=bootstrap_deployer=" + deployerRole, "--file=" + filepath.Join("..", "..", "deploy", "sql", file)}
		if enabled {
			args = append(args, "--set=apply_grants=true")
		}
		command := exec.CommandContext(ctx, psql, args...)
		command.Env = append(os.Environ(), "PGPASSWORD="+connection.Password)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("actual psql template %s failed: %s", file, output)
		}
	}
	apply("runtime-grants-0005-0009.psql", false)
	var granted bool
	if err = owner.pool.QueryRow(ctx, "SELECT has_table_privilege($1,'content.announcements','SELECT')", runtimeRole).Scan(&granted); err != nil || granted {
		t.Fatal("default template changed privileges", err)
	}
	apply("runtime-grants-0005-0009.psql", true)
	apply("bootstrap-deployer-grant.psql", true)
	openRole := func(role string) *Store {
		t.Helper()
		v := u
		v.User = url.UserPassword(role, password)
		s, err := Open(ctx, v.String())
		if err != nil {
			t.Fatal("independent role connection failed")
		}
		t.Cleanup(s.Close)
		var actual, session string
		if err = s.pool.QueryRow(ctx, "SELECT current_user,session_user").Scan(&actual, &session); err != nil || actual != role || session != role {
			t.Fatal("acceptance still used owner/session elevation", err)
		}
		return s
	}
	runtime, deployer = openRole(runtimeRole), openRole(deployerRole)
	t.Log("actual psql templates applied; independent runtime/deployer LOGIN identities verified; fixture database", database)
	return
}

func TestRuntimeJointAcceptance(t *testing.T) {
	owner, runtime, deployer := jointRuntimeStores(t)
	ctx := context.Background()
	const user int64 = 4201
	// Synthetic local setup only: this is not a native/production bootstrap.
	receipt, err := deployer.Bootstrap(ctx, BootstrapInput{Environment: "STAGING", UserID: user, Username: "JointSynthetic", ReleaseBuild: "synthetic-joint", ExpectedEmpty: true})
	if err != nil || receipt.PrincipalID == "" {
		t.Fatal("deployer-only function failed", err)
	}
	principal, err := runtime.AnnouncementAuthority(ctx, user)
	if err != nil {
		t.Fatal("runtime authority read failed", err)
	}
	t.Run("profile", func(t *testing.T) {
		p, err := runtime.EnsureProvisionalProfile(ctx, user)
		if err != nil || p.Status != "INCOMPLETE" {
			t.Fatal("provisional profile", err)
		}
		p = initializeProfile(t, runtime, user, "JointSynthetic")
		p, err = runtime.UpdateProfile(ctx, user, ProfilePatch{ExpectedVersion: p.ProfileVersion, DisplayName: textPtr("JointRenamed")})
		if err != nil || p.ProfileVersion != 2 {
			t.Fatal("profile rename", err)
		}
	})
	t.Run("announcements", func(t *testing.T) {
		draft := announcementDraft(t, runtime, ctx, principal)
		publish := announcementPublishCommand(t, principal, draft)
		until := time.Now().UTC().Add(time.Hour)
		publish.VisibleUntil = &until
		publish.Placements = []AnnouncementPlacement{{Placement: "ENTRY_POPUP"}, {Placement: "PINNED_LIST"}}
		result := announcementConfirm(t, runtime, ctx, principal, publish)
		view, err := runtime.PublicAnnouncement(ctx, user, result.ID, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = runtime.ReadAnnouncement(ctx, user, result.ID, view.NotificationRevision); err != nil {
			t.Fatal("announcement read ACK", err)
		}
		placement := AnnouncementCommand{OperationID: announcementID(t), Epoch: principal.Epoch, ID: result.ID, ExpectedVersion: result.Version, Action: "UPDATE_PLACEMENTS", Reason: "Synthetic replacement", Placements: []AnnouncementPlacement{{Placement: "PINNED_LIST", ManualOrder: 1}}}
		announcementConfirm(t, runtime, ctx, principal, placement)
		// Advance only these local synthetic expiry facts; the worker remains runtime.
		if _, err = owner.pool.Exec(ctx, "UPDATE content.announcements SET visible_until=clock_timestamp()-interval '1 second' WHERE announcement_id=$1", result.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = owner.pool.Exec(ctx, "UPDATE content.announcement_jobs SET due_at=clock_timestamp()-interval '1 second' WHERE announcement_id=$1", result.ID); err != nil {
			t.Fatal(err)
		}
		if count, err := runtime.RunAnnouncementJobs(ctx); err != nil || count != 1 {
			t.Fatal("announcement job", count, err)
		}
	})
	t.Run("catalog", func(t *testing.T) {
		source := catalogStoreSnapshot(t, "synthetic/joint")
		read := func(context.Context) (NativeCatalog, error) { return source, nil }
		if _, err := runtime.SyncCatalog(ctx, read); err != nil {
			t.Fatal("catalog sync", err)
		}
		p, model, err := runtime.OpsCatalogModel(ctx, user, "synthetic/joint", catalogTestPolicy)
		if err != nil {
			t.Fatal(err)
		}
		metadata := catalogEditableMetadata()
		save := catalogCommand(t, runtime, p, model, "SAVE")
		save.Metadata = &metadata
		catalogConfirm(t, runtime, p, save)
		_, model, err = runtime.OpsCatalogModel(ctx, user, model.ModelID, catalogTestPolicy)
		if err != nil {
			t.Fatal(err)
		}
		catalogConfirm(t, runtime, p, catalogCommand(t, runtime, p, model, "PUBLISH"))
		if model, err = runtime.PublicCatalogModel(ctx, model.ModelID, catalogTestPolicy); err != nil || model.PublicationState != "PUBLISHED" {
			t.Fatal("catalog published read", err)
		}
		if _, err = runtime.SyncCatalog(ctx, read); err != nil {
			t.Fatal("catalog sync upsert", err)
		}
	})
	t.Run("admission", func(t *testing.T) {
		page := RegistrationPage{Receipts: []RegistrationReceipt{{Ordinal: 1, OperationID: "550e8400-e29b-41d4-a716-446655440000", NativeUserID: 4202, DiscordSubject: "12345678901234567", Source: "NEW_DISCORD_REGISTRATION", PolicyVersion: "joint-synthetic-v1", CreatedAt: time.Now().UTC().Add(-time.Minute)}}, NextCursor: 1}
		if err := runtime.IngestRegistrationPage(ctx, 0, page); err != nil {
			t.Fatal("synthetic receipt ingest", err)
		}
		if found, err := runtime.RecoverRegistrationGrant(ctx); err != nil || !found {
			t.Fatal("synthetic grant recovery", err)
		}
		status, err := runtime.ReadAdmission(ctx, 4202)
		if err != nil || status.GrantStatus != "CONFIRMED" {
			t.Fatal("synthetic receipt confirmation", err)
		}
		if found, err := runtime.RecoverRegistrationGrant(ctx); err != nil || found {
			t.Fatal("completed grant replay", err)
		}
	})
	t.Run("migration_notice", func(t *testing.T) {
		_, err := owner.pool.Exec(ctx, `INSERT INTO identity.migration_notice_versions(version,title,body,completed_at,evidence_ref) VALUES(1,'Synthetic notice','Local completed fixture',now(),'joint-fixture'); INSERT INTO identity.migration_notice_requirements(newapi_user_id,version) VALUES(4201,1)`)
		if err != nil {
			t.Fatal(err)
		}
		notice, err := runtime.ReadMigrationNotice(ctx, user, false)
		if err != nil || notice.State != "REQUIRED" {
			t.Fatal(err)
		}
		first, err := runtime.AcknowledgeMigrationNotice(ctx, user, 1)
		if err != nil || first.AcknowledgedAt == nil {
			t.Fatal("notice first ACK", err)
		}
		_, err = owner.pool.Exec(ctx, `INSERT INTO identity.migration_notice_versions(version,title,body,completed_at,evidence_ref) VALUES(2,'Synthetic later notice','Local later fixture',now(),'joint-fixture-v2'); INSERT INTO identity.migration_notice_requirements(newapi_user_id,version) VALUES(4201,2)`)
		if err != nil {
			t.Fatal(err)
		}
		again, err := runtime.AcknowledgeMigrationNotice(ctx, user, 1)
		if err != nil || again.AcknowledgedAt == nil || !again.AcknowledgedAt.Equal(*first.AcknowledgedAt) {
			t.Fatal("notice original ACK replay", err)
		}
		notice, err = runtime.ReadMigrationNotice(ctx, user, false)
		if err != nil || notice.RequiredVersion != 2 || notice.State != "REQUIRED" {
			t.Fatal("latest notice remained gated", err)
		}
		var n int
		if err = owner.pool.QueryRow(ctx, "SELECT count(*) FROM economy.asset_transactions WHERE newapi_user_id=$1", user).Scan(&n); err != nil || n != 0 {
			t.Fatal("notice caused asset effects", err)
		}
	})
	t.Run("denials", func(t *testing.T) {
		// Existing tables/functions are queried as owner before asserting 42501;
		// missing objects (42P01/42883) are never accepted as permission evidence.
		for _, table := range []string{"ops.admin_principals", "ops.admin_role_history", "ops.bootstrap_closure", "ops.access_control_guards", "ops.admin_operations", "catalog.model_metadata_revisions", "identity.migration_notice_versions", "identity.migration_notice_requirements", "identity.migration_notice_acknowledgements", "public.users"} {
			var n int
			if err := owner.pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
				t.Fatal("owner cannot access denial target", table, err)
			}
		}
		var function string
		if err := owner.pool.QueryRow(ctx, "SELECT 'ops.bootstrap_super_admin(text,bigint,text,text,boolean,uuid,uuid,uuid)'::regprocedure::text").Scan(&function); err != nil {
			t.Fatal(err)
		}
		queries := []string{
			"CREATE TABLE catalog.forbidden_runtime_ddl(id int)",
			"UPDATE ops.admin_principals SET base_role='SUPER_ADMIN'",
			"DELETE FROM ops.admin_role_history",
			"UPDATE ops.bootstrap_closure SET reason='PRINCIPAL_OBSERVED'",
			"SELECT * FROM ops.access_control_guards FOR UPDATE",
			"UPDATE ops.admin_operations SET details='{}'",
			"UPDATE catalog.model_metadata_revisions SET content='{}'",
			"UPDATE identity.migration_notice_versions SET body='changed'",
			"DELETE FROM identity.migration_notice_requirements",
			"UPDATE identity.migration_notice_acknowledgements SET acknowledged_at=now()",
			"UPDATE public.users SET quota=999 WHERE id=1",
			"SELECT * FROM ops.bootstrap_super_admin('STAGING',4203,'Synthetic','synthetic',true,'550e8400-e29b-41d4-a716-446655440001','550e8400-e29b-41d4-a716-446655440002','550e8400-e29b-41d4-a716-446655440003')",
		}
		for _, query := range queries {
			_, err := runtime.pool.Exec(ctx, query)
			var p *pgconn.PgError
			if !errors.As(err, &p) || p.Code != "42501" {
				t.Fatalf("runtime denial was not permission error: %s: %v", query, err)
			}
		}
		if _, err := deployer.pool.Exec(ctx, "SELECT * FROM ops.admin_principals"); err == nil {
			t.Fatal("deployer acquired table access")
		}
		var quota int
		if err := owner.pool.QueryRow(ctx, "SELECT quota FROM public.users WHERE id=1").Scan(&quota); err != nil || quota != 77 {
			t.Fatal("synthetic native-users sentinel changed", err)
		}
		t.Logf("%d runtime operations denied with 42501 against verified objects; native-users target was a local synthetic sentinel only", len(queries))
	})
}
