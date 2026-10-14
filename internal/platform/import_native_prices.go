package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type PriceImportResult struct {
	Prices   int      `json:"prices"`
	Unpriced []string `json:"unpriced_models"`
}

// Materialize official f1164142 effective rates once. Runtime billing only uses
// the resulting final numeric prices, not a second native ratio system.
func (s *Store) ImportNativePrices(ctx context.Context, source *pgx.Conn) (PriceImportResult, error) {
	result := PriceImportResult{Unpriced: []string{}}
	from, err := source.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer from.Rollback(ctx)
	var raw []byte
	err = from.QueryRow(ctx, `SELECT jsonb_object_agg(key,value::jsonb) FROM options WHERE key=ANY($1::text[])`, []string{"ModelRatio", "CompletionRatio", "CacheRatio", "CreateCacheRatio", "ModelPrice", "billing_setting.billing_mode", "billing_setting.billing_expr"}).Scan(&raw)
	if err != nil {
		return result, err
	}
	var options struct {
		Input      map[string]json.Number `json:"ModelRatio"`
		Output     map[string]json.Number `json:"CompletionRatio"`
		Read       map[string]json.Number `json:"CacheRatio"`
		Write      map[string]json.Number `json:"CreateCacheRatio"`
		Request    map[string]json.Number `json:"ModelPrice"`
		Mode       map[string]string      `json:"billing_setting.billing_mode"`
		Expression map[string]string      `json:"billing_setting.billing_expr"`
	}
	if err = json.Unmarshal(raw, &options); err != nil {
		return result, err
	}
	rows, err := from.Query(ctx, `SELECT DISTINCT unnest(string_to_array(models,',')) AS model FROM channels ORDER BY model`)
	if err != nil {
		return result, err
	}
	models, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return result, err
	}
	for name := range options.Input {
		models = append(models, name)
	}
	for name := range options.Request {
		models = append(models, name)
	}
	for name := range options.Mode {
		models = append(models, name)
	}
	slices.Sort(models)
	models = slices.Compact(models)
	to, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer rollback(to)
	if _, err = to.Exec(ctx, `LOCK TABLE relay.prices IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return result, err
	}
	var occupied bool
	if err = to.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM relay.prices)`).Scan(&occupied); err != nil {
		return result, err
	}
	if occupied {
		return result, fmt.Errorf("price import requires empty target table")
	}
	// The only expression currently deployed is a linear duration charge. Flatten
	// it to per_second/default_seconds; never evaluate an arbitrary DSL at runtime.
	duration := regexp.MustCompile(`^tier\("[^"]+", \(param\("duration"\) == nil \? ([0-9]+) : param\("duration"\)\) \* ([0-9]+(?:\.[0-9]+)?)\)$`)
	for _, model := range models {
		match := FormatMatchingModelName(model)
		input, hasInput := options.Input[match]
		perRequest, hasRequest := options.Request[match]
		var requestPrice, secondPrice any
		seconds := 0
		if options.Mode[model] == "tiered_expr" {
			parts := duration.FindStringSubmatch(options.Expression[model])
			if parts == nil {
				return result, fmt.Errorf("price expression needs explicit conversion: %s", model)
			}
			seconds, err = strconv.Atoi(parts[1])
			if err != nil {
				return result, err
			}
			secondPrice = parts[2]
			input = "0"
		} else if hasRequest {
			requestPrice = perRequest.String()
			input = "0"
		} else if !hasInput {
			result.Unpriced = append(result.Unpriced, model)
			continue
		}
		output, locked := getHardcodedCompletionModelRatio(match)
		completion := strconv.FormatFloat(output, 'f', -1, 64)
		if configured, ok := options.Output[match]; ok && (!locked || strings.Contains(match, "/")) {
			completion = configured.String()
		}
		cacheRead, cacheWrite := "1", "1.25"
		if value, ok := options.Read[model]; ok {
			cacheRead = value.String()
		}
		if value, ok := options.Write[model]; ok {
			cacheWrite = value.String()
		}
		_, err = to.Exec(ctx, `INSERT INTO relay.prices(model,input,output,cache_read,cache_write,cache_write_1h,per_request,per_second,default_seconds)
 VALUES($1,$2::numeric*1000000,$2::numeric*$3::numeric*1000000,$2::numeric*$4::numeric*1000000,
 $2::numeric*$5::numeric*1000000,$2::numeric*$5::numeric*1.6*1000000,$6::numeric*500000,$7::numeric*500000/1000000,$8)`,
			model, input.String(), completion, cacheRead, cacheWrite, requestPrice, secondPrice, seconds)
		if err != nil {
			return result, err
		}
		result.Prices++
	}
	if err = from.Commit(ctx); err != nil {
		return result, err
	}
	return result, to.Commit(ctx)
}
