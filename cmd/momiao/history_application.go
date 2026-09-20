package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/history"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker"
 "github.com/cy4268/momiao/internal/roulette"
	"github.com/cy4268/momiao/internal/session"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errHistoryStartup = errors.New("history startup failed")

type historyApplication struct {
	handler                *historyHTTP
	readerPool, workerPool *pgxpool.Pool
	pokerPool              *pgxpool.Pool
	cancel                 context.CancelFunc
	done                   chan struct{}
	once                   sync.Once
}

func (a *historyApplication) Close() {
	if a == nil {
		return
	}
	a.once.Do(func() {
		if a.cancel != nil {
			a.cancel()
			<-a.done
		}
		if a.workerPool != nil {
			a.workerPool.Close()
		}
		if a.readerPool != nil {
			a.readerPool.Close()
		}
		if a.pokerPool != nil {
			a.pokerPool.Close()
		}
	})
}

// Dependencies are already authenticated/domain services. This composition never
// starts actors, repairs games, creates accounts, provisions roles or migrates.
func openHistoryApplication(ctx context.Context, cfg historyConfig, authorityPool *pgxpool.Pool, sessions *session.Service, rounds *games.Service, wallet *platform.Store, rouletteService ...*roulette.Service) (app *historyApplication, err error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if len(rouletteService)>1 || ctx == nil || authorityPool == nil || sessions == nil || rounds == nil || wallet == nil {
		return nil, errHistoryStartup
	}
	a := &historyApplication{}
	defer func() {
		if err != nil {
			a.Close()
			app, err = nil, errHistoryStartup
		}
	}()
	startup, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err = wallet.CheckSameDatabase(startup, authorityPool); err != nil {
		return nil, err
	}
	a.readerPool, err = openPokerPool(startup, cfg.ReaderDSNFile)
	if err != nil {
		return nil, err
	}
	a.workerPool, err = openPokerPool(startup, cfg.WorkerDSNFile)
	if err != nil {
		return nil, err
	}
	if err = validateHistoryPools(startup, a.readerPool, a.workerPool, authorityPool); err != nil {
		return nil, err
	}
	// A read-only domain reader remains available even when live Poker is disabled.
	// It owns no actor, Redis connection, admission lease, or gameplay worker.
	a.pokerPool, err = openPokerPool(startup, cfg.PokerDSNFile)
	if err != nil {
		return nil, err
	}
	if err = validatePokerPools(startup, authorityPool, a.pokerPool); err != nil {
		return nil, err
	}
	keys, err := readPokerStateKeys(cfg.PokerKeyringFile)
	if err != nil {
		return nil, err
	}
	hands, err := poker.NewHistoryReader(a.pokerPool, keys)
	if err != nil {
		return nil, err
	}
	a.handler = &historyHTTP{sessions: sessions, list: history.NewReader(a.readerPool), rounds: rounds, poker: hands, wallet: wallet}
	if len(rouletteService)==1 { a.handler.roulette=rouletteService[0] }
	workerCtx, stop := context.WithCancel(ctx)
	a.cancel, a.done = stop, make(chan struct{})
	go func() {
		defer close(a.done)
		worker := history.NewWorker(a.workerPool)
		for {
			if worker.Run(workerCtx) == nil || workerCtx.Err() != nil {
				return
			}
			// Retry only the idempotent projection worker, never a business command.
			log.Print("history projection worker failed; retrying in 5 seconds")
			timer := time.NewTimer(5 * time.Second)
			select {
			case <-workerCtx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return a, nil
}

func validateHistoryPools(ctx context.Context, reader, worker, authority *pgxpool.Pool) error {
	if reader == nil || worker == nil || authority == nil || !samePokerEndpoint(reader.Config(), worker.Config()) || !samePokerEndpoint(reader.Config(), authority.Config()) {
		return errHistoryStartup
	}
	identity := func(pool *pgxpool.Pool, capability string) (string, string, error) {
		var role, database string
		var valid bool
		err := pool.QueryRow(ctx, `SELECT current_user,
 json_build_array(current_database(),d.oid,inet_server_addr(),inet_server_port(),pg_postmaster_start_time())::text,
 session_user=current_user AND current_user=$2::name
 AND r.rolcanlogin AND NOT r.rolinherit AND NOT r.rolsuper AND NOT r.rolcreatedb AND NOT r.rolcreaterole
 AND NOT r.rolreplication AND NOT r.rolbypassrls AND d.datdba<>r.oid
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_auth_members m WHERE m.member=r.oid)
 AND pg_catalog.has_function_privilege(r.oid,$1::text,'EXECUTE')
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_namespace n WHERE n.nspname IN('games','poker','economy','identity','roulette','public','roulette')
  AND (n.nspowner=r.oid OR pg_catalog.has_schema_privilege(r.oid,n.oid,'CREATE')))
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
  WHERE n.nspname IN('games','poker','economy','identity','roulette') AND c.relkind IN('r','p','v','m','f')
   AND (pg_catalog.has_any_column_privilege(r.oid,c.oid,'SELECT,INSERT,UPDATE,REFERENCES')
    OR pg_catalog.has_table_privilege(r.oid,c.oid,'DELETE,TRUNCATE,TRIGGER')))
 AND NOT EXISTS(SELECT 1 FROM pg_catalog.pg_proc f JOIN pg_catalog.pg_namespace n ON n.oid=f.pronamespace
  WHERE n.nspname IN('games','poker','economy','identity','roulette') AND f.prosecdef AND f.oid<>$1::regprocedure
   AND pg_catalog.has_function_privilege(r.oid,f.oid,'EXECUTE'))
 FROM pg_catalog.pg_roles r JOIN pg_catalog.pg_database d ON d.datname=current_database()
 WHERE r.rolname=current_user`, capability, pool.Config().ConnConfig.User).Scan(&role, &database, &valid)
		if err != nil || !valid {
			return "", "", errHistoryStartup
		}
		return role, database, nil
	}
	rRole, rDB, err := identity(reader, "games.history_list(bigint,jsonb)")
	if err != nil {
		return err
	}
	wRole, wDB, err := identity(worker, "games.history_ingest_batch(text,integer)")
	if err != nil || rRole == wRole || rDB != wDB {
		return errHistoryStartup
	}
	var authorityDB, authorityRole string
	err = authority.QueryRow(ctx, `SELECT current_user,json_build_array(current_database(),d.oid,inet_server_addr(),inet_server_port(),pg_postmaster_start_time())::text
 FROM pg_catalog.pg_database d WHERE d.datname=current_database()`).Scan(&authorityRole, &authorityDB)
	if err != nil || authorityDB != rDB || authorityRole == rRole || authorityRole == wRole {
		return errHistoryStartup
	}
	return nil
}
