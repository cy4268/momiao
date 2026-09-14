package platform

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDatabaseMismatch = errors.New("database identity mismatch")

// CheckSameDatabase proves a single configured endpoint and live database match.
// It exposes no DSN/pool and makes no claim about either role's permissions.
func (s *Store) CheckSameDatabase(ctx context.Context, authority *pgxpool.Pool) error {
	if ctx == nil || s == nil || s.pool == nil || authority == nil {
		return ErrDatabaseMismatch
	}
	a, b := s.pool.Config(), authority.Config()
	if !singleDatabaseEndpoint(a) || !singleDatabaseEndpoint(b) || a.ConnConfig.Host != b.ConnConfig.Host || a.ConnConfig.Port != b.ConnConfig.Port || a.ConnConfig.Database != b.ConnConfig.Database {
		return ErrDatabaseMismatch
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const query = `SELECT json_build_array(current_database(),d.oid,inet_server_addr(),inet_server_port(),pg_postmaster_start_time())::text FROM pg_catalog.pg_database d WHERE d.datname=current_database()`
	var walletID, authorityID string
	if s.pool.QueryRow(bounded, query).Scan(&walletID) != nil || authority.QueryRow(bounded, query).Scan(&authorityID) != nil || walletID == "" || walletID != authorityID {
		return ErrDatabaseMismatch
	}
	return nil
}

func singleDatabaseEndpoint(c *pgxpool.Config) bool {
	if c == nil || c.ConnConfig == nil || len(c.ConnConfig.Fallbacks) != 0 {
		return false
	}
	host := c.ConnConfig.Host
	local := host == "localhost" || strings.HasPrefix(host, "/")
	if ip := net.ParseIP(host); ip != nil {
		local = ip.IsLoopback()
	}
	return local || c.ConnConfig.TLSConfig != nil && !c.ConnConfig.TLSConfig.InsecureSkipVerify
}
