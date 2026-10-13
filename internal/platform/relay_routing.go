// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
// Key eligibility, priority/weight selection and multi-key state follow official
// new-api@f116414284162ad15d8925f7bca494c109b83e93 model/token.go,
// model/channel_cache.go and model/channel.go, using the existing pgx store.
package platform

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

var (
	ErrRelayKey     = errors.New("API key invalid or unavailable")
	ErrRelayModel   = errors.New("API key does not allow this model")
	ErrRelayChannel = errors.New("当前分组无可用渠道")
	ErrRelayPrice   = errors.New("model price is not configured")
)

type RelayIdentity struct {
	UserID, KeyID, Reserve  int64
	KeyName, Purpose, Group string
	Models                  []string // nil allows all; an empty, non-nil list allows none.
}

const relayIdentitySQL = `SELECT u.id,k.id,k.name,k.purpose,u.group_name,k.models,coalesce(b.balance_units,0)
 FROM relay.api_keys k JOIN identity.users u ON u.id=k.user_id
 LEFT JOIN economy.wallet_balances b ON b.newapi_user_id=u.id AND b.asset_type='RESERVE_API_CREDIT'
 WHERE k.key=$1 AND k.status=1 AND (k.expires_at IS NULL OR k.expires_at>now())
 AND (k.quota_limit IS NULL OR k.used_quota<k.quota_limit) AND u.status=1 AND u.deleted_at IS NULL`

func scanRelayIdentity(row pgx.Row) (RelayIdentity, error) {
	var identity RelayIdentity
	err := row.Scan(&identity.UserID, &identity.KeyID, &identity.KeyName, &identity.Purpose, &identity.Group, &identity.Models, &identity.Reserve)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrRelayKey
	}
	return identity, err
}

func (s *Store) AuthenticateRelay(ctx context.Context, key string) (RelayIdentity, error) {
	// ponytail: query current state for this small site; add a short TTL only if
	// profiling warrants it. Key revocation and account bans take effect immediately.
	return scanRelayIdentity(s.pool.QueryRow(ctx, relayIdentitySQL, key))
}

func (s *Store) RelayModelsForKey(ctx context.Context, identity RelayIdentity) ([]string, error) {
	items, err := s.RelayModels(ctx, identity.Group)
	if err != nil {
		return nil, err
	}
	if identity.Models != nil {
		items = slices.DeleteFunc(items, func(model string) bool { return !slices.Contains(identity.Models, model) })
	}
	return items, nil
}

type RelayChannel struct {
	ID, Priority, Weight                             int64
	Type                                             int
	Name, BaseURL, Key, Other, UpstreamModel         string
	ModelMapping                                     map[string]string
	HeaderOverride, ParamOverride, Setting, Settings json.RawMessage
	Info                                             relayChannelInfo
	KeyIndex                                         int
}

type relayChannelInfo struct {
	IsMultiKey   bool        `json:"is_multi_key"`
	Status       map[int]int `json:"multi_key_status_list"`
	Mode         string      `json:"multi_key_mode"`
	PollingIndex int         `json:"multi_key_polling_index"`
}

type RelayRoute struct {
	Identity RelayIdentity
	Channel  RelayChannel
}

func (s *Store) PrepareRelay(ctx context.Context, key, model string, excluded []int64) (RelayRoute, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RelayRoute{}, err
	}
	defer rollback(tx)
	route, err := prepareRelay(ctx, tx, key, model, excluded)
	if err != nil {
		return route, err
	}
	return route, tx.Commit(ctx)
}

func prepareRelay(ctx context.Context, tx pgx.Tx, key, model string, excluded []int64) (RelayRoute, error) {
	var route RelayRoute
	identity, err := scanRelayIdentity(tx.QueryRow(ctx, relayIdentitySQL, key))
	if err != nil {
		return route, err
	}
	route.Identity = identity
	if identity.Models != nil && !slices.Contains(identity.Models, model) {
		return route, ErrRelayModel
	}
	if identity.Reserve <= 0 {
		return route, ErrInsufficientBalance
	}
	if err = RequireNoMaintenance(ctx, tx, "CHALDEA_USER_WRITES"); err != nil {
		return route, err
	}
	route.Channel, err = selectRelayChannel(ctx, tx, identity.Group, model, excluded)
	if err != nil {
		return route, err
	}
	var priced bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM relay.prices p,relay.groups g WHERE p.model=$1 AND g.name=$2)`, model, identity.Group).Scan(&priced)
	if err != nil {
		return route, err
	}
	if !priced {
		return route, ErrRelayPrice
	}
	return route, nil
}

func selectRelayChannel(ctx context.Context, tx pgx.Tx, group, model string, excluded []int64) (RelayChannel, error) {
	rows, err := tx.Query(ctx, `SELECT id,type,name,base_url,key,other,priority,weight,model_mapping,
 header_override,param_override,setting,settings,channel_info FROM relay.channels
 WHERE enabled AND $1=ANY(groups) AND $2=ANY(models) AND NOT(id=ANY(coalesce($3::bigint[],'{}')))
 ORDER BY priority DESC,id`, group, model, excluded)
	if err != nil {
		return RelayChannel{}, err
	}
	defer rows.Close()
	var candidates []RelayChannel
	var weight int64
	for rows.Next() {
		var channel RelayChannel
		if err = rows.Scan(&channel.ID, &channel.Type, &channel.Name, &channel.BaseURL, &channel.Key, &channel.Other,
			&channel.Priority, &channel.Weight, &channel.ModelMapping, &channel.HeaderOverride, &channel.ParamOverride,
			&channel.Setting, &channel.Settings, &channel.Info); err != nil {
			return RelayChannel{}, err
		}
		_, enabled, e := channel.enabledKeys()
		if e != nil {
			return RelayChannel{}, e
		}
		if len(enabled) == 0 {
			continue
		}
		if len(candidates) > 0 && channel.Priority != candidates[0].Priority {
			break
		}
		candidates = append(candidates, channel)
		weight += channel.Weight
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return RelayChannel{}, err
	}
	if len(candidates) == 0 {
		return RelayChannel{}, ErrRelayChannel
	}
	// Official selection treats an all-zero priority tier as equal weights.
	index := rand.IntN(len(candidates))
	if weight > 0 {
		value := rand.Int64N(weight)
		for i, channel := range candidates {
			value -= channel.Weight
			if value < 0 {
				index = i
				break
			}
		}
	}
	channel := candidates[index]
	if channel.Info.IsMultiKey && channel.Info.Mode == "polling" {
		// Only the selected polling channel is locked, never while doing HTTP I/O.
		err = tx.QueryRow(ctx, `SELECT key,channel_info FROM relay.channels WHERE id=$1 FOR UPDATE`, channel.ID).Scan(&channel.Key, &channel.Info)
		if err != nil {
			return channel, err
		}
	}
	keys, enabled, err := channel.enabledKeys()
	if err != nil {
		return channel, err
	}
	if len(enabled) == 0 {
		return channel, ErrRelayChannel
	}
	channel.KeyIndex = enabled[0]
	if channel.Info.IsMultiKey {
		switch channel.Info.Mode {
		case "random":
			channel.KeyIndex = enabled[rand.IntN(len(enabled))]
		case "polling":
			for _, i := range enabled {
				if i >= channel.Info.PollingIndex {
					channel.KeyIndex = i
					break
				}
			}
			_, err = tx.Exec(ctx, `UPDATE relay.channels SET channel_info=jsonb_set(channel_info,'{multi_key_polling_index}',to_jsonb($2::integer)) WHERE id=$1`, channel.ID, (channel.KeyIndex+1)%len(keys))
			if err != nil {
				return channel, err
			}
		}
	}
	channel.Key = keys[channel.KeyIndex]
	channel.UpstreamModel = model
	if mapped := channel.ModelMapping[model]; mapped != "" {
		channel.UpstreamModel = mapped
	}
	return channel, nil
}

func (c RelayChannel) enabledKeys() ([]string, []int, error) {
	if !c.Info.IsMultiKey {
		if strings.TrimSpace(c.Key) == "" {
			return nil, nil, nil
		}
		return []string{c.Key}, []int{0}, nil
	}
	var keys []string
	if strings.HasPrefix(strings.TrimSpace(c.Key), "[") {
		var values []json.RawMessage
		if err := json.Unmarshal([]byte(c.Key), &values); err != nil {
			return nil, nil, err
		}
		for _, value := range values {
			keys = append(keys, string(value))
		}
	} else {
		keys = strings.Split(strings.Trim(c.Key, "\n"), "\n")
	}
	var enabled []int
	for i, key := range keys {
		status, exists := c.Info.Status[i]
		if strings.TrimSpace(key) != "" && (!exists || status == 1) {
			enabled = append(enabled, i)
		}
	}
	return keys, enabled, nil
}
