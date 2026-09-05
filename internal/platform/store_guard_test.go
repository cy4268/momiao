package platform

import (
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

// Parse and reject the configured database before connecting or mutating it.
func validateTestDatabase(dsn string) error {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(config.ConnConfig.Database, "momiao_test_") {
		return fmt.Errorf("integration database name must start with momiao_test_")
	}
	return nil
}
