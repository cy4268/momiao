package platform

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ImportNativeWorkspace preserves channel configuration and log metadata, not
// prompts, response content, IPs or native audit payloads. Balances stay intact.
func (s *Store) ImportNativeWorkspace(ctx context.Context, source *pgx.Conn) (map[string]int64, error) {
	from, err := source.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer from.Rollback(ctx)
	to, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollback(to)
	if _, err = to.Exec(ctx, `LOCK TABLE relay.groups,relay.channels,relay.logs IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return nil, err
	}
	var occupied bool
	if err = to.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM relay.groups) OR EXISTS(SELECT 1 FROM relay.channels) OR EXISTS(SELECT 1 FROM relay.logs)`).Scan(&occupied); err != nil {
		return nil, err
	}
	if occupied {
		return nil, fmt.Errorf("workspace import requires empty target tables")
	}
	result := make(map[string]int64)
	for _, table := range []struct {
		name    string
		columns []string
		query   string
	}{
		{"groups", []string{"name", "description", "ratio"}, `WITH
 ratios AS (SELECT coalesce((SELECT value::jsonb FROM options WHERE key='GroupRatio'),'{}'::jsonb) AS value),
 descriptions AS (SELECT coalesce((SELECT value::jsonb FROM options WHERE key='UserUsableGroups'),'{}'::jsonb) AS value),
 names AS (SELECT "group" AS name FROM users UNION SELECT unnest(string_to_array("group",',')) FROM channels UNION SELECT jsonb_object_keys(value) FROM ratios)
 SELECT name,coalesce(descriptions.value->>name,name),coalesce((ratios.value->>name)::numeric,1)
 FROM names CROSS JOIN ratios CROSS JOIN descriptions WHERE name<>'' ORDER BY name`},
		{"channels", []string{"id", "name", "type", "base_url", "key", "models", "groups", "model_mapping", "priority", "weight", "enabled", "header_override", "param_override", "channel_info", "setting", "settings", "other", "created_at", "updated_at"},
			`SELECT id,name,type,coalesce(base_url,''),key,string_to_array(coalesce(models,''),','),string_to_array(coalesce("group",''),','),
 coalesce(nullif(model_mapping,'')::jsonb,'{}'),coalesce(priority,0),coalesce(weight,0),status=1,
 coalesce(nullif(header_override,'')::jsonb,'{}'),coalesce(nullif(param_override,'')::jsonb,'{}'),coalesce(channel_info::jsonb,'{}'),
 coalesce(nullif(setting,'')::jsonb,'{}'),coalesce(nullif(settings,'')::jsonb,'{}'),coalesce(other,''),to_timestamp(created_time),to_timestamp(created_time)
 FROM channels ORDER BY id`},
		{"logs", []string{"id", "user_id", "key_id", "key_name", "model", "channel_id", "group_name", "kind", "prompt_tokens", "completion_tokens", "cache_read_tokens", "cache_write_tokens", "cache_write_tokens_1h", "cost_units", "duration_ms", "is_stream", "status_code", "error", "created_at"},
			`SELECT id,user_id,coalesce(token_id,0),coalesce(token_name,''),coalesce(model_name,''),coalesce(channel_id,0),coalesce("group",''),type,
 coalesce(prompt_tokens,0),coalesce(completion_tokens,0),coalesce((metadata->>'cache_tokens')::bigint,0),
 coalesce((metadata->>'cache_write_tokens')::bigint,(metadata->>'cache_creation_tokens')::bigint,
 coalesce((metadata->>'cache_creation_tokens_5m')::bigint,0)+coalesce((metadata->>'cache_creation_tokens_1h')::bigint,0)),
 coalesce((metadata->>'cache_creation_tokens_1h')::bigint,0),coalesce(quota,0),coalesce(use_time,0)::bigint*1000,coalesce(is_stream,false),
 CASE WHEN type=2 THEN 200 ELSE 0 END,CASE WHEN type=5 THEN 'UPSTREAM_ERROR' ELSE '' END,to_timestamp(created_at)
 FROM (SELECT *,nullif(other,'')::jsonb AS metadata FROM logs) source_logs ORDER BY id`},
	} {
		rows, err := from.Query(ctx, table.query)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", table.name, err)
		}
		values, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([]any, error) { return row.Values() })
		if err != nil {
			return nil, err
		}
		result[table.name], err = to.CopyFrom(ctx, pgx.Identifier{"relay", table.name}, table.columns, pgx.CopyFromRows(values))
		if err != nil {
			return nil, fmt.Errorf("copy %s: %w", table.name, err)
		}
		if table.name != "groups" {
			var high int64
			if err = from.QueryRow(ctx, `SELECT greatest(coalesce(max(id),0),coalesce(pg_sequence_last_value(pg_get_serial_sequence($1,'id')::regclass),0)) FROM `+table.name, table.name).Scan(&high); err != nil {
				return nil, err
			}
			if _, err = to.Exec(ctx, `SELECT setval(pg_get_serial_sequence($1,'id'),$2+1,false)`, "relay."+table.name, high); err != nil {
				return nil, err
			}
		}
	}
	if err = from.Commit(ctx); err != nil {
		return nil, err
	}
	return result, to.Commit(ctx)
}
