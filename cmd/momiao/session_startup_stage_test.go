package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSessionStartupFixture(t *testing.T, dir, name, value string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func sessionStartupFixture(t *testing.T) config {
	t.Helper()
	dir := resolvedAppPrivateDir(t)
	return config{
		PublicOrigin: "https://recovery.example",
		NewAPISocket: filepath.Join(dir, "native.sock"),
		Session: sessionConfig{
			Enabled:               true,
			ReaderKeyFile:         writeSessionStartupFixture(t, dir, "reader.key", strings.Repeat("1", 64)),
			OpsKeyFile:            writeSessionStartupFixture(t, dir, "ops.key", strings.Repeat("2", 64)),
			SessionSealKeyFile:    writeSessionStartupFixture(t, dir, "session.key", strings.Repeat("3", 64)),
			CredentialSealKeyFile: writeSessionStartupFixture(t, dir, "credential.key", strings.Repeat("4", 64)),
			DSNFile:               writeSessionStartupFixture(t, dir, "session.dsn", "postgres://%zz"),
			RedisConfigFile:       writeSessionStartupFixture(t, dir, "session-redis.json", "{}"),
			OpsSocket:             filepath.Join(dir, "ops.sock"),
			Environment:           "RECOVERY_TEST",
		},
	}
}

func TestSessionStartupFailureStagesAreConstantAndSanitized(t *testing.T) {
	for _, stage := range []sessionStartupStage{
		sessionStageContext, sessionStageReaderKey, sessionStageOpsKey,
		sessionStageSessionSealKey, sessionStageCredentialSealKey,
		sessionStageKeyDistinctness, sessionStagePGDSNFile, sessionStagePGDSNParse,
		sessionStagePGPoolCreate, sessionStagePGPing, sessionStagePGPermissions,
		sessionStageRedisConfig, sessionStageRedisPing,
		sessionStageSessionConstructor, sessionStageBFFAuthConstructor,
	} {
		err := newSessionStartupFailure(stage)
		if err.Error() != "session startup failed" || sessionStartupFailureStage(err) != stage {
			t.Fatalf("stage %q was not preserved behind the generic error", stage)
		}
		if strings.Contains(err.Error(), string(stage)) || errors.Is(err, errSessionConfig) {
			t.Fatalf("stage %q leaked through the public error", stage)
		}
	}
	if sessionStartupFailureStage(errors.New("unclassified")) != sessionStageUnknown {
		t.Fatal("foreign error did not fail closed to UNKNOWN")
	}
}

func TestSessionStartupReportsEarlyRealBranches(t *testing.T) {
	cfg := sessionStartupFixture(t)
	if _, err := openSessionApplication(nil, cfg); err == nil || err.Error() != "session startup failed" || sessionStartupFailureStage(err) != sessionStageContext {
		t.Fatalf("nil context stage = %v/%q", err, sessionStartupFailureStage(err))
	}

	cfg = sessionStartupFixture(t)
	cfg.Session.ReaderKeyFile = filepath.Join(t.TempDir(), "missing-reader.key")
	if _, err := openSessionApplication(context.Background(), cfg); err == nil || sessionStartupFailureStage(err) != sessionStageReaderKey {
		t.Fatalf("reader stage = %v/%q", err, sessionStartupFailureStage(err))
	}

	cfg = sessionStartupFixture(t)
	cfg.Session.OpsKeyFile = filepath.Join(t.TempDir(), "missing-ops.key")
	if _, err := openSessionApplication(context.Background(), cfg); err == nil || sessionStartupFailureStage(err) != sessionStageOpsKey {
		t.Fatalf("ops stage = %v/%q", err, sessionStartupFailureStage(err))
	}

	cfg = sessionStartupFixture(t)
	cfg.Session.SessionSealKeyFile = filepath.Join(t.TempDir(), "missing-session.key")
	if _, err := openSessionApplication(context.Background(), cfg); err == nil || sessionStartupFailureStage(err) != sessionStageSessionSealKey {
		t.Fatalf("session seal stage = %v/%q", err, sessionStartupFailureStage(err))
	}

	cfg = sessionStartupFixture(t)
	cfg.Session.CredentialSealKeyFile = filepath.Join(t.TempDir(), "missing-credential.key")
	if _, err := openSessionApplication(context.Background(), cfg); err == nil || sessionStartupFailureStage(err) != sessionStageCredentialSealKey {
		t.Fatalf("credential seal stage = %v/%q", err, sessionStartupFailureStage(err))
	}

	cfg = sessionStartupFixture(t)
	cfg.Session.OpsKeyFile = cfg.Session.ReaderKeyFile
	if _, err := openSessionApplication(context.Background(), cfg); err == nil || sessionStartupFailureStage(err) != sessionStageKeyDistinctness {
		t.Fatalf("distinctness stage = %v/%q", err, sessionStartupFailureStage(err))
	}

	cfg = sessionStartupFixture(t)
	if _, err := openSessionApplication(context.Background(), cfg); err == nil || sessionStartupFailureStage(err) != sessionStagePGDSNParse {
		t.Fatalf("PG parse stage = %v/%q", err, sessionStartupFailureStage(err))
	}
}

func TestSessionPermissionSQLAllowsOnlyOpsFreshnessReadColumns(t *testing.T) {
	allowed := "(n.nspname='ops' AND c.relname='admin_principals' AND a.attname IN('authz_epoch','newapi_user_id','status'))"
	if strings.Count(sessionPoolPermissionsSQL, allowed) != 1 {
		t.Fatalf("session permission SQL must contain the exact Ops freshness read allowlist once")
	}
	for _, broad := range []string{
		"(n.nspname='ops' AND c.relname='admin_principals')",
		"(n.nspname='ops' AND c.relname='admin_principals' AND a.attname",
	} {
		if broad != allowed && strings.Contains(sessionPoolPermissionsSQL, broad+")") {
			t.Fatalf("session permission SQL contains broad Ops read allowlist %q", broad)
		}
	}
}

func TestRecoveryLockLogsOnlyConstantSessionStartupStage(t *testing.T) {
	cfg := sessionStartupFixture(t)
	cfg.RecoveryLock = true
	cfg.Session.ReaderKeyFile = filepath.Join(t.TempDir(), "sensitive-name-must-not-appear")
	var output bytes.Buffer
	err := runRecoveryLockedProcess(context.Background(), cfg, log.New(&output, "", 0))
	if !errors.Is(err, errSessionStartup) {
		t.Fatalf("locked startup error = %v", err)
	}
	if output.String() != "DR_RECOVERY_SESSION_STARTUP_STAGE=READER_KEY\n" {
		t.Fatalf("unsafe or ambiguous recovery diagnostic %q", output.String())
	}
}
