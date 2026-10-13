// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
// User log ownership, explicit model filters and descending pagination reuse
// new-api@f116414284162ad15d8925f7bca494c109b83e93 model/log.go and model/token.go.
package platform

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

type RelayLog struct {
	ID               int64  `json:"id"`
	CreatedAt        int64  `json:"created_at"`
	Type             int    `json:"type"`
	TokenName        string `json:"token_name"`
	ModelName        string `json:"model_name"`
	Quota            int64  `json:"quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
}
type RelayLogPage struct {
	Items    []RelayLog `json:"items"`
	Total    int64      `json:"total"`
	Page     int64      `json:"page"`
	PageSize int64      `json:"page_size"`
}
type RelayLogFilter struct {
	Page, PageSize, Type, Start, End int64
	Model                            string
}
type RelayGroup struct {
	Ratio float64 `json:"ratio"`
	Desc  string  `json:"desc"`
}

func (s *Store) ListRelayLogs(ctx context.Context, user int64, f RelayLogFilter) (RelayLogPage, error) {
	result := RelayLogPage{Items: []RelayLog{}, Page: f.Page, PageSize: f.PageSize}
	where := ` WHERE user_id=$1 AND ($2::smallint=0 OR kind=$2)
 AND ($3::bigint=0 OR created_at>=to_timestamp($3::bigint)) AND ($4::bigint=0 OR created_at<=to_timestamp($4::bigint))`
	model := f.Model
	if strings.Contains(model, "%") {
		where += ` AND model LIKE $5 ESCAPE '!'`
		model = strings.NewReplacer("!", "!!", "_", "!_").Replace(model)
	} else {
		where += ` AND ($5::text='' OR model=$5)`
	}
	args := []any{user, f.Type, f.Start, f.End, model}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM relay.logs`+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := s.pool.Query(ctx, `SELECT id,extract(epoch FROM created_at)::bigint,kind,key_name,model,cost_units,prompt_tokens,completion_tokens FROM relay.logs`+where+` ORDER BY id DESC LIMIT $6 OFFSET $7`, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item RelayLog
		if err = rows.Scan(&item.ID, &item.CreatedAt, &item.Type, &item.TokenName, &item.ModelName, &item.Quota, &item.PromptTokens, &item.CompletionTokens); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}
func (s *Store) ReadRelayGroup(ctx context.Context, name string) (map[string]RelayGroup, error) {
	var item RelayGroup
	err := s.pool.QueryRow(ctx, `SELECT ratio::double precision,description FROM relay.groups WHERE name=$1`, name).Scan(&item.Ratio, &item.Desc)
	return map[string]RelayGroup{name: item}, err
}
func (s *Store) RelayModels(ctx context.Context, group string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT unnest(models) AS model FROM relay.channels WHERE enabled AND $1=ANY(groups) ORDER BY model`, group)
	if err != nil {
		return nil, err
	}
	items, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if items == nil {
		items = []string{}
	}
	return items, err
}
func (s *Store) ReadRelayUsage(ctx context.Context, user int64) (used, requests int64, err error) {
	err = s.pool.QueryRow(ctx, `SELECT coalesce(sum(cost_units),0),count(*) FROM relay.logs WHERE user_id=$1 AND kind IN (2,5)`, user).Scan(&used, &requests)
	return
}
