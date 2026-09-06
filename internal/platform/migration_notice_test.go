package platform

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestMigrationNoticeFactsAndExactlyOnce(t *testing.T) {
	s := bootstrapStore(t)
	ctx := context.Background()
	n, err := s.ReadMigrationNotice(ctx, 42, false)
	if err != nil || n.State != "UNVERIFIED" {
		t.Fatalf("missing source passed: %+v %v", n, err)
	}
	n, err = s.ReadMigrationNotice(ctx, 42, true)
	if err != nil || n.State != "NOT_REQUIRED" {
		t.Fatalf("explicit no applicability: %+v %v", n, err)
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO identity.account_refs(newapi_user_id) VALUES(42),(43);
 INSERT INTO identity.migration_notice_versions(version,title,body,completed_at,evidence_ref) VALUES(1,'Synthetic completed migration','Synthetic facts only',now()-interval '1 hour','synthetic-reviewed-batch');
 INSERT INTO identity.migration_notice_requirements(newapi_user_id,version) VALUES(42,1)`)
	if err != nil {
		t.Fatal(err)
	}
	n, err = s.ReadMigrationNotice(ctx, 42, true)
	if err != nil || n.State != "REQUIRED" || n.RequiredVersion != 1 {
		t.Fatal("real requirement was bypassed by declaration")
	}
	if _, err = s.AcknowledgeMigrationNotice(ctx, 43, 1); !errors.Is(err, ErrMigrationNoticeStale) {
		t.Fatal("cross-user ack accepted")
	}
	var wg sync.WaitGroup
	results := make(chan MigrationNotice, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r, e := s.AcknowledgeMigrationNotice(ctx, 42, 1); results <- r; failures <- e }()
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	stamp := ""
	for r := range results {
		if r.State != "ACKNOWLEDGED" || r.AcknowledgedAt == nil {
			t.Fatal("missing original ack")
		}
		v := r.AcknowledgedAt.String()
		if stamp != "" && stamp != v {
			t.Fatal("replay replaced original ack timestamp")
		}
		stamp = v
	}
	for table, want := range map[string]int{"identity.migration_notice_acknowledgements": 1, "identity.master_profiles": 0, "economy.wallet_balances": 0, "economy.asset_transactions": 0, "rewards.registration_grants": 0} {
		var count int
		if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != want {
			t.Fatalf("ack crossed boundary %s count=%d error=%v", table, count, err)
		}
	}
	for _, sql := range []string{`UPDATE identity.migration_notice_acknowledgements SET acknowledged_at=now()`, `DELETE FROM identity.migration_notice_versions`, `TRUNCATE identity.migration_notice_acknowledgements`} {
		if _, err = s.pool.Exec(ctx, sql); err == nil {
			t.Fatal("immutable notice fact changed")
		}
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO identity.migration_notice_versions(version,title,body,completed_at,evidence_ref) VALUES(2,'Later synthetic version','Still only facts',now(),'synthetic-reviewed-v2'); INSERT INTO identity.migration_notice_requirements(newapi_user_id,version) VALUES(42,2)`)
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.AcknowledgeMigrationNotice(ctx, 42, 1)
	if err != nil || old.RequiredVersion != 1 || old.AcknowledgedAt == nil || old.AcknowledgedAt.String() != stamp {
		t.Fatal("new version broke original acknowledgement replay", err)
	}
	latest, err := s.ReadMigrationNotice(ctx, 42, false)
	if err != nil || latest.RequiredVersion != 2 || latest.State != "REQUIRED" {
		t.Fatal("old replay bypassed newer notice")
	}
}
