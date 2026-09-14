package platform_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/history"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/testutil/pokerhistoryfixture"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const root24Wallet = "economy.history_transaction_read(bigint,uuid)"
const root24Snapshot = "games.history_record_snapshot(bigint,text,uuid)"

func root24Denied(t *testing.T, err error) {
	t.Helper()
	var e *pgconn.PgError
	if !errors.As(err, &e) || e.Code != "42501" {
		t.Fatalf("expected permission denial, got %T", err)
	}
}

func root24Exec(t *testing.T, pool *pgxpool.Pool, query string, allowed bool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), query)
	if allowed {
		pokerhistoryfixture.Check(t, err)
	} else {
		root24Denied(t, err)
	}
}

func root24Digest(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var digest string
	pokerhistoryfixture.Check(t, pool.QueryRow(context.Background(), `SELECT md5(jsonb_build_object(
      'registry',(SELECT jsonb_agg(to_jsonb(m) ORDER BY version) FROM platform_meta.schema_migrations m),
      'schemas',(SELECT jsonb_agg(to_jsonb(n) ORDER BY n.oid) FROM pg_namespace n
        WHERE n.nspname=ANY(ARRAY['platform_meta','identity','economy','games','poker','ops'])),
      'relations',(SELECT jsonb_agg(jsonb_build_array(c.oid,c.relname,c.relkind,c.relowner,c.relacl,c.reloptions) ORDER BY c.oid)
        FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=ANY(ARRAY['platform_meta','identity','economy','games','poker','ops'])),
      'columns',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.attrelid,a.attnum) FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid
        JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=ANY(ARRAY['platform_meta','identity','economy','games','poker','ops'])),
      'functions',(SELECT jsonb_agg(jsonb_build_array(p.oid,p.proowner,p.proacl,pg_get_functiondef(p.oid)) ORDER BY p.oid)
        FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=ANY(ARRAY['platform_meta','identity','economy','games','poker','ops'])),
      'constraints',(SELECT jsonb_agg(to_jsonb(c) ORDER BY c.oid) FROM pg_constraint c JOIN pg_namespace n ON n.oid=c.connamespace
        WHERE n.nspname=ANY(ARRAY['platform_meta','identity','economy','games','poker','ops'])),
      'triggers',(SELECT jsonb_agg(to_jsonb(g) ORDER BY g.oid) FROM pg_trigger g JOIN pg_class c ON c.oid=g.tgrelid
        JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=ANY(ARRAY['platform_meta','identity','economy','games','poker','ops']))
      )::text)`).Scan(&digest))
	return digest
}

func root24Profile(t *testing.T, f *pokerhistoryfixture.Fixture, application *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	roles := []string{f.PlatformRole, f.PokerRole, f.ReaderRole, f.WorkerRole}
	var valid bool
	pokerhistoryfixture.Check(t, f.Pool.QueryRow(ctx, `SELECT count(*)=5 AND bool_and(NOT rolsuper AND NOT rolcreatedb
      AND NOT rolcreaterole AND NOT rolreplication AND NOT rolbypassrls AND NOT rolinherit AND rolcanlogin=(rolname<>$1))
      AND NOT EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles r ON r.oid IN(m.member,m.roleid) WHERE r.rolname=ANY($2))
      AND (SELECT pg_get_userbyid(datdba)=$1 FROM pg_database WHERE datname=current_database())
      FROM pg_roles WHERE rolname=ANY($2)`, f.OwnerRole, append([]string{f.OwnerRole}, roles...)).Scan(&valid))
	if !valid {
		t.Fatal("current24 five-role ownership, LOGIN, NOINHERIT or membership profile changed")
	}
	pokerhistoryfixture.Check(t, f.Pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM pg_namespace n
      WHERE n.nspname=ANY(ARRAY['platform_meta','identity','economy','games','poker','ops'])
      AND (pg_get_userbyid(n.nspowner)<>$1 OR EXISTS(SELECT 1 FROM pg_class c WHERE c.relnamespace=n.oid AND pg_get_userbyid(c.relowner)<>$1)
        OR EXISTS(SELECT 1 FROM pg_proc p WHERE p.pronamespace=n.oid AND pg_get_userbyid(p.proowner)<>$1)))`, f.OwnerRole).Scan(&valid))
	if !valid {
		t.Fatal("fixture owner does not own every application schema, relation and function")
	}
	capabilities := []struct {
		signature, call string
		mask            int
	}{
		{"games.history_capture_display()", "SELECT games.history_capture_display()", 0},
		{"games.history_ingest_batch(text,integer)", "SELECT games.history_ingest_batch('POKER_SESSION',1)", 8},
		{"games.history_list(bigint,jsonb)", "SELECT games.history_list(101,'{}')", 4},
		{root24Snapshot, "SELECT games.history_record_snapshot(101,'POKER_SESSION','00000000-0000-4000-8000-000000000099')", 3},
		{root24Wallet, "SELECT economy.history_transaction_read(101,'00000000-0000-4000-8000-000000000099')", 1},
		{"poker.history_funding_transactions(bigint,uuid,uuid[])", "SELECT poker.history_funding_transactions(101,'00000000-0000-4000-8000-000000000099','{}'::uuid[])", 2},
	}
	for i, pool := range []*pgxpool.Pool{application, f.Poker, f.Reader, f.Worker} {
		role := roles[i]
		pokerhistoryfixture.Check(t, pool.QueryRow(ctx, "SELECT current_database()=$1 AND current_user=$2 AND session_user=$2", f.Database, role).Scan(&valid))
		if !valid {
			t.Fatal("current24 runtime pool is not its independent LOGIN")
		}
		pokerhistoryfixture.Check(t, f.Pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM pg_namespace n
          WHERE n.nspname=ANY(ARRAY['platform_meta','identity','economy','games','poker','ops','public']) AND has_schema_privilege($1,n.oid,'CREATE'))`, role).Scan(&valid))
		if !valid {
			t.Fatal("runtime has schema CREATE")
		}
		for _, table := range []string{"history_display_snapshots", "history_index", "history_ingestion_cursors", "history_source_rows"} {
			pokerhistoryfixture.Check(t, f.Pool.QueryRow(ctx, "SELECT NOT has_any_column_privilege($1,$2,'SELECT,INSERT,UPDATE,REFERENCES') AND NOT has_table_privilege($1,$2,'DELETE,TRUNCATE,TRIGGER')", role, "games."+table).Scan(&valid))
			if !valid {
				t.Fatal("runtime has direct History table or column capability")
			}
			root24Exec(t, pool, "SELECT 1 FROM games."+table+" LIMIT 0", false)
		}
		for _, capability := range capabilities {
			allowed := capability.mask&(1<<i) != 0
			pokerhistoryfixture.Check(t, f.Pool.QueryRow(ctx, "SELECT has_function_privilege($1,$2,'EXECUTE')=$3", role, capability.signature, allowed).Scan(&valid))
			if !valid {
				t.Fatalf("unexpected function capability: %s", capability.signature)
			}
			root24Exec(t, pool, capability.call, allowed)
		}
		for _, command := range []struct{ privilege, sql string }{
			{"SELECT", "SELECT password_phc FROM poker.table_access_credentials LIMIT 0"},
			{"INSERT", "INSERT INTO poker.table_access_credentials(table_id,password_phc) SELECT NULL::uuid,NULL::text WHERE false"},
			{"UPDATE", "UPDATE poker.table_access_credentials SET password_phc=password_phc WHERE false"},
			{"DELETE", "DELETE FROM poker.table_access_credentials WHERE false"},
			{"TRUNCATE", "TRUNCATE poker.table_access_credentials"},
		} {
			allowed := i == 1 && (command.privilege == "SELECT" || command.privilege == "INSERT")
			pokerhistoryfixture.Check(t, f.Pool.QueryRow(ctx, "SELECT has_table_privilege($1,'poker.table_access_credentials',$2)=$3", role, command.privilege, allowed).Scan(&valid))
			if !valid {
				t.Fatal("unexpected PHC table privilege")
			}
			root24Exec(t, pool, command.sql, allowed)
		}
		pokerhistoryfixture.Check(t, f.Pool.QueryRow(ctx, `SELECT NOT has_any_column_privilege($1,'poker.table_access_credentials','UPDATE,REFERENCES')
          AND NOT has_table_privilege($1,'poker.table_access_credentials','TRIGGER') AND ($2 OR NOT has_any_column_privilege($1,'poker.table_access_credentials','SELECT,INSERT'))`, role, i == 1).Scan(&valid))
		if !valid {
			t.Fatal("unexpected PHC column or trigger privilege")
		}
		if i > 0 {
			pokerhistoryfixture.Check(t, f.Pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
              WHERE n.nspname=ANY(ARRAY['economy','ops']) AND c.relkind IN('r','p','v','m','f')
              AND (has_any_column_privilege($1,c.oid,'SELECT,INSERT,UPDATE,REFERENCES') OR has_table_privilege($1,c.oid,'DELETE,TRUNCATE,TRIGGER')))`, role).Scan(&valid))
			if !valid {
				t.Fatal("non-platform runtime has raw Economy or Ops table/column access")
			}
			for _, table := range []string{"economy.wallet_balances", "economy.wallet_ledger", "ops.maintenance_windows"} {
				root24Exec(t, pool, "SELECT 1 FROM "+table+" LIMIT 0", false)
			}
		}
		if i > 1 {
			pokerhistoryfixture.Check(t, f.Pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
              WHERE n.nspname=ANY(ARRAY['identity','economy','games','poker','ops']) AND c.relkind IN('r','p','v','m','f')
              AND (has_any_column_privilege($1,c.oid,'SELECT,INSERT,UPDATE,REFERENCES') OR has_table_privilege($1,c.oid,'DELETE,TRUNCATE,TRIGGER')))`, role).Scan(&valid))
			if !valid {
				t.Fatal("History runtime has direct source table or column access")
			}
			root24Exec(t, pool, "SELECT setup_cipher FROM poker.hands LIMIT 0", false)
			root24Exec(t, pool, "SELECT 1 FROM games.game_rounds LIMIT 0", false)
		}
	}
	root24Exec(t, application, "SELECT 1 FROM economy.wallet_balances LIMIT 0", true)
	root24Exec(t, application, "SELECT 1 FROM games.game_rounds LIMIT 0", true)
	root24Exec(t, f.Poker, "SELECT 1 FROM poker.tables LIMIT 0", true)
	pokerhistoryfixture.Check(t, f.Pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM pg_proc p
      CROSS JOIN LATERAL aclexplode(coalesce(p.proacl,acldefault('f',p.proowner))) a
      WHERE p.proname IN('history_capture_display','history_ingest_batch','history_list','history_record_snapshot','history_transaction_read','history_funding_transactions')
      AND a.grantee=0)`).Scan(&valid))
	if !valid {
		t.Fatal("PUBLIC has a History capability")
	}
	pokerhistoryfixture.Check(t, f.Pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM pg_class c CROSS JOIN LATERAL aclexplode(c.relacl) a
      WHERE c.oid='poker.table_access_credentials'::regclass AND a.grantee=0)
      AND NOT EXISTS(SELECT 1 FROM pg_attribute c CROSS JOIN LATERAL aclexplode(c.attacl) a
      WHERE c.attrelid='poker.table_access_credentials'::regclass AND a.grantee=0)`).Scan(&valid))
	if !valid {
		t.Fatal("PUBLIC has a PHC table or column capability")
	}
}

func TestRootHistoryCurrent24(t *testing.T) {
	t.Run("accepted_profile_and_sources", func(t *testing.T) {
		f := pokerhistoryfixture.Open(t)
		ctx := context.Background()
		cfg := f.Poker.Config()
		cfg.ConnConfig.User = f.PlatformRole
		application, err := pgxpool.NewWithConfig(ctx, cfg)
		pokerhistoryfixture.Check(t, err)
		defer application.Close()
		before := root24Digest(t, f.Pool)
		pokerhistoryfixture.Check(t, f.Owner.Migrate(ctx))
		if before != root24Digest(t, f.Pool) {
			t.Fatal("idempotent Migrate changed registry/applied_at or schema")
		}
		root24Profile(t, f, application)
		for _, pollution := range []struct {
			role, capability string
			grantOption      bool
		}{
			{f.PokerRole, root24Wallet, false}, {f.ReaderRole, root24Wallet, false}, {f.WorkerRole, root24Wallet, false},
			{f.ReaderRole, root24Snapshot, false}, {f.WorkerRole, root24Snapshot, false},
			{f.PokerRole, "poker.history_funding_transactions(bigint,uuid,uuid[])", true},
			{f.PlatformRole, "poker.history_funding_transactions(bigint,uuid,uuid[])", false},
			{f.ReaderRole, "poker.history_funding_transactions(bigint,uuid,uuid[])", false},
			{f.WorkerRole, "poker.history_funding_transactions(bigint,uuid,uuid[])", false},
		} {
			object := "EXECUTE ON FUNCTION " + pollution.capability + " TO " + pgx.Identifier{pollution.role}.Sanitize()
			revoke, installer, runtime := "EXECUTE", "deploy/sql/runtime-grants-0023-history-details.psql", "platform"
			if pollution.grantOption {
				object, revoke = object+" WITH GRANT OPTION", "GRANT OPTION FOR EXECUTE"
			}
			if pollution.capability == "poker.history_funding_transactions(bigint,uuid,uuid[])" {
				installer, runtime = "deploy/sql/runtime-grants-0024-history-poker-details.psql", "poker"
			}
			f.SQL(t, "GRANT "+object)
			f.Grant(t, installer, runtime, false)
			f.SQL(t, "REVOKE "+revoke+" ON FUNCTION "+pollution.capability+" FROM "+pgx.Identifier{pollution.role}.Sanitize())
			f.Grant(t, installer, runtime, true)
			root24Profile(t, f, application)
		}
		// Local fixture sources: one real domain-created Dice Round, two constructed
		// Sessions and one constructed two-participant Hand; no Native/live Host claim.
		id := func(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }
		_, err = f.Owner.Apply(ctx, platform.Mutation{UserID: 101, Asset: platform.AvailableChips, DeltaUnits: 5000000000, BizType: "ROOT24_FIXTURE", BizID: id(90), EntryType: "GRANT", IdempotencyKey: id(90)})
		pokerhistoryfixture.Check(t, err)
		service, err := games.NewService(f.Runtime, games.Keyring{Active: "root24", Keys: map[string][32]byte{"root24": {1}}})
		pokerhistoryfixture.Check(t, err)
		boot, err := service.Bootstrap(ctx, 101, "dice")
		pokerhistoryfixture.Check(t, err)
		_, err = service.Create(ctx, 101, "dice", id(91), boot.Next.ID, games.CreateInput{Type: "DICE", Wager: "10", Choice: "BIG"})
		pokerhistoryfixture.Check(t, err)
		f.SQL(t, `INSERT INTO poker.tables(table_id,owner_newapi_user_id,name,max_seats,blind_preset_version,ruleset_version)
          VALUES($1,101,'Root history table',2,'5-10','poker-cash-v1-20260906');
          INSERT INTO poker.sessions(session_id,newapi_user_id,table_id,seat_no,display_name_snapshot,initial_buyin_units,current_stack_units)
          VALUES($2,101,$1,1,'Player A',200000000,200000000),($3,102,$1,2,'Player B',200000000,200000000);
          INSERT INTO poker.hands(hand_id,table_id,hand_no,state,button_seat,runtime_epoch,setup_cipher) VALUES($4,$1,1,'COMMITTED',1,0,'\x00');
          INSERT INTO poker.hand_participants(hand_id,seat_no,session_id,newapi_user_id,hand_start_stack_units,contribution_version,contribution_cipher)
          VALUES($4,1,$2,101,200000000,1,'\x00'),($4,2,$3,102,200000000,1,'\x00')`, id(1), id(11), id(12), id(21))
		reader, worker := history.NewReader(f.Reader), history.NewWorker(f.Worker)
		for i, source := range []history.Source{history.Round, history.Session, history.Hand} {
			progress, e := worker.Step(ctx, source, 200)
			pokerhistoryfixture.Check(t, e)
			if progress.Busy || progress.WrittenRows != []int{1, 2, 2}[i] {
				t.Fatal("actual History LOGIN failed to ingest every local source owner")
			}
			for _, user := range []int64{101, 102} {
				page, e := reader.List(ctx, user, history.Query{RecordType: source})
				pokerhistoryfixture.Check(t, e)
				want := 1
				if user == 102 && source == history.Round {
					want = 0
				}
				if len(page.Items) != want {
					t.Fatal("History source ownership count mismatch")
				}
				for _, item := range page.Items {
					if item.UserID != fmt.Sprint(user) || item.RecordType != source || item.Snapshot.MetadataOrigin != "creation_snapshot" {
						t.Fatal("History owner, source type or metadata provenance changed")
					}
				}
			}
		}
		f.SQL(t, `UPDATE poker.tables SET name='Renamed root table'; UPDATE games.game_registry SET publication_state='RETIRED' WHERE game_slug='texas-holdem';
          INSERT INTO games.game_registry(game_slug,title,sort_order,publication_state,configured_runtime_state,implementation_key)
          VALUES('root24-future','Future root game',99,'PUBLISHED','UNAVAILABLE','unimplemented')`)
		page, err := reader.List(ctx, 101, history.Query{RecordType: history.Session})
		pokerhistoryfixture.Check(t, err)
		retired, future := false, false
		for _, option := range page.GameOptions {
			retired = retired || option.GameSlug == "texas-holdem" && option.Retired
			future = future || option.GameSlug == "root24-future"
		}
		if len(page.Items) != 1 || page.Items[0].Snapshot.TableName == nil || *page.Items[0].Snapshot.TableName != "Root history table" || !retired || !future {
			t.Fatal("History snapshot or dynamic game options lost their source contract")
		}
		f.SQL(t, "REVOKE EXECUTE ON FUNCTION games.history_list(bigint,jsonb) FROM "+pgx.Identifier{f.ReaderRole}.Sanitize())
		_, err = reader.List(ctx, 101, history.Query{})
		root24Denied(t, err)
		f.Grant(t, "deploy/sql/runtime-grants-0021-history.psql", "platform", true)
		root24Profile(t, f, application)
		f.SQL(t, "REVOKE EXECUTE ON FUNCTION games.history_ingest_batch(text,integer) FROM "+pgx.Identifier{f.WorkerRole}.Sanitize())
		_, err = worker.Step(ctx, history.Session, 200)
		root24Denied(t, err)
		f.Grant(t, "deploy/sql/runtime-grants-0021-history.psql", "platform", true)
		root24Profile(t, f, application)
		t.Log("current24 registry, effective/actual four-LOGIN ACL, nine pollution rejections and real Reader/Worker source ownership verified")
	})
	for _, fault := range []struct{ name, sql string }{
		{"checksum_mismatch", "UPDATE platform_meta.schema_migrations SET checksum=repeat('0',64) WHERE version=22"},
		{"registry_gap", "DELETE FROM platform_meta.schema_migrations WHERE version=22"},
		{"unknown_version", "INSERT INTO platform_meta.schema_migrations(version,checksum) VALUES(25,repeat('0',64))"},
	} {
		t.Run(fault.name, func(t *testing.T) {
			f := pokerhistoryfixture.Open(t) // Dedicated disposable fault DB; never a successful migration receipt.
			f.SQL(t, fault.sql)
			before := root24Digest(t, f.Pool)
			if err := f.Owner.Migrate(context.Background()); !errors.Is(err, platform.ErrMigrationMismatch) {
				t.Fatalf("Migrate accepted %s or returned another failure: %v", fault.name, err)
			}
			if before != root24Digest(t, f.Pool) {
				t.Fatal("rejected Migrate changed registry or schema")
			}
		})
	}
}
