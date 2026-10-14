package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

var ErrRelayGroupInUse = errors.New("分组仍有用户或渠道引用")

type ChannelSettings struct {
	ID             int64             `json:"id"`
	Name           string            `json:"name"`
	Type           int               `json:"type"`
	BaseURL        string            `json:"base_url"`
	Models         []string          `json:"models"`
	Groups         []string          `json:"groups"`
	ModelMapping   map[string]string `json:"model_mapping"`
	HeaderOverride map[string]string `json:"header_override"`
	Priority       int64             `json:"priority"`
	Weight         int64             `json:"weight"`
	Enabled        bool              `json:"enabled"`
}
type ChannelWrite struct {
	ChannelSettings
	Key string `json:"key"`
}
type OpsChannel struct {
	ChannelSettings
	KeyConfigured bool    `json:"key_configured"`
	Requests      int64   `json:"requests_24h"`
	Errors        int64   `json:"errors_24h"`
	ErrorRate     float64 `json:"error_rate"`
}
type OpsChannelPage struct {
	Items    []OpsChannel `json:"items"`
	Total    int64        `json:"total"`
	Page     int64        `json:"page"`
	PageSize int64        `json:"page_size"`
}
type OpsRelayFilter struct {
	Page, PageSize         int64
	Search, Group, Enabled string
}

// Validate once at the HTTP boundary. Imported advanced settings are untouched.
func ValidateChannel(v *ChannelWrite) error {
	v.Name = strings.TrimSpace(v.Name)
	v.Key = strings.TrimSpace(v.Key)
	v.BaseURL = strings.TrimRight(strings.TrimSpace(v.BaseURL), "/")
	if v.ID < 0 || v.Name == "" || !slices.Contains([]int{1, 14, 24, 41, 43}, v.Type) || v.Weight < 0 || v.Weight > 1000000 || v.ID == 0 && v.Key == "" {
		return ErrOpsInvalid
	}
	if v.BaseURL != "" {
		u, err := url.Parse(v.BaseURL)
		if err != nil || u.Host == "" || u.Scheme != "http" && u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return ErrOpsInvalid
		}
	}
	for _, id := range v.Models {
		if !ValidCatalogModelID(id) {
			return ErrOpsInvalid
		}
	}
	for _, name := range v.Groups {
		if strings.TrimSpace(name) == "" {
			return ErrOpsInvalid
		}
	}
	if v.Models == nil {
		v.Models = []string{}
	}
	if v.Groups == nil {
		v.Groups = []string{}
	}
	// Model order matters: the first one is used by the manual channel test.
	v.Groups = slices.Clone(v.Groups)
	slices.Sort(v.Groups)
	v.Groups = slices.Compact(v.Groups)
	if v.ModelMapping == nil {
		v.ModelMapping = map[string]string{}
	}
	if v.HeaderOverride == nil {
		v.HeaderOverride = map[string]string{}
	}
	return nil
}

const opsChannelJSON = `jsonb_build_object('id',c.id,'name',c.name,'type',c.type,'base_url',c.base_url,
 'models',c.models,'groups',c.groups,'model_mapping',c.model_mapping,'header_override',c.header_override,
 'priority',c.priority,'weight',c.weight,'enabled',c.enabled,'key_configured',c.key<>'')`

func (s *Store) ListOpsChannels(ctx context.Context, f OpsRelayFilter) (OpsChannelPage, error) {
	return listOpsChannels(ctx, s.pool, f)
}
func listOpsChannels(ctx context.Context, q announcementQuerier, f OpsRelayFilter) (OpsChannelPage, error) {
	result := OpsChannelPage{Items: []OpsChannel{}, Page: f.Page, PageSize: f.PageSize}
	where := ` WHERE ($1::text='' OR c.name ILIKE '%'||$1||'%' OR EXISTS(SELECT 1 FROM unnest(c.models) m WHERE m ILIKE '%'||$1||'%'))
 AND ($2::text='' OR $2=ANY(c.groups)) AND ($3::text='' OR c.enabled=($3='true'))`
	args := []any{f.Search, f.Group, f.Enabled}
	if err := q.QueryRow(ctx, `SELECT count(*) FROM relay.channels c`+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	rows, err := q.Query(ctx, `SELECT `+opsChannelJSON+`,stats.requests,stats.errors FROM relay.channels c
 LEFT JOIN LATERAL(SELECT count(*) AS requests,count(*) FILTER(WHERE kind=5 OR status_code>=400) AS errors
 FROM relay.logs WHERE channel_id=c.id AND kind IN(2,5) AND created_at>=now()-interval '24 hours') stats ON true`+where+` ORDER BY c.id DESC LIMIT $4 OFFSET $5`, append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item OpsChannel
		if err = rows.Scan(&item, &item.Requests, &item.Errors); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	for i := range result.Items {
		item := &result.Items[i]
		if item.Requests > 0 {
			item.ErrorRate = float64(item.Errors) / float64(item.Requests)
		}
	}
	return result, rows.Err()
}
func readOpsChannel(ctx context.Context, q announcementQuerier, id int64, lock bool) (OpsChannel, error) {
	query := `SELECT ` + opsChannelJSON + ` FROM relay.channels c WHERE c.id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	var item OpsChannel
	err := q.QueryRow(ctx, query, id).Scan(&item)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrOpsNotFound
	}
	return item, err
}
func (s *Store) OpsChannel(ctx context.Context, id int64) (OpsChannel, error) {
	return readOpsChannel(ctx, s.pool, id, false)
}

func channelAudit(v OpsChannel) any {
	// Extra headers may themselves contain credentials; record names, not values.
	names := make([]string, 0, len(v.HeaderOverride))
	for name := range v.HeaderOverride {
		names = append(names, name)
	}
	slices.Sort(names)
	v.HeaderOverride = nil
	return struct {
		Channel OpsChannel `json:"channel"`
		Headers []string   `json:"header_names"`
	}{v, names}
}
func relayOpsAudit(ctx context.Context, tx pgx.Tx, user int64, action, kind, target string, before, after any) error {
	if before == nil {
		before = json.RawMessage(`null`)
	}
	if after == nil {
		after = json.RawMessage(`null`)
	}
	_, err := tx.Exec(ctx, `INSERT INTO ops.audit_log(actor_user_id,action,target_type,target,before_state,after_state) VALUES($1,$2,$3,$4,$5,$6)`, user, action, kind, target, before, after)
	return err
}
func (s *Store) SaveOpsChannel(ctx context.Context, user int64, v ChannelWrite) (OpsChannel, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OpsChannel{}, err
	}
	defer rollback(tx)
	item, err := saveOpsChannel(ctx, tx, user, v)
	if err != nil {
		return item, err
	}
	return item, tx.Commit(ctx)
}
func saveOpsChannel(ctx context.Context, tx pgx.Tx, user int64, v ChannelWrite) (OpsChannel, error) {
	// Shared locks pair with group rename/delete; no cache invalidation is needed.
	rows, err := tx.Query(ctx, `SELECT name FROM relay.groups WHERE name=ANY($1) ORDER BY name FOR SHARE`, v.Groups)
	if err != nil {
		return OpsChannel{}, err
	}
	groups, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return OpsChannel{}, err
	}
	if len(groups) != len(v.Groups) {
		return OpsChannel{}, ErrOpsInvalid
	}
	var before any
	if v.ID != 0 {
		item, err := readOpsChannel(ctx, tx, v.ID, true)
		if err != nil {
			return item, err
		}
		before = channelAudit(item)
	} else {
		err = tx.QueryRow(ctx, `INSERT INTO relay.channels(name,type,key) VALUES($1,$2,$3) RETURNING id`, v.Name, v.Type, v.Key).Scan(&v.ID)
		if err != nil {
			return OpsChannel{}, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE relay.channels SET name=$2,type=$3,base_url=$4,models=$5,groups=$6,model_mapping=$7,
 priority=$8,weight=$9,enabled=$10,header_override=$11,key=CASE WHEN $12='' THEN key ELSE $12 END,updated_at=now() WHERE id=$1`,
		v.ID, v.Name, v.Type, v.BaseURL, v.Models, v.Groups, v.ModelMapping, v.Priority, v.Weight, v.Enabled, v.HeaderOverride, v.Key)
	if err != nil {
		return OpsChannel{}, err
	}
	item, err := readOpsChannel(ctx, tx, v.ID, false)
	if err != nil {
		return item, err
	}
	after := struct {
		Value      any  `json:"value"`
		KeyChanged bool `json:"key_changed"`
	}{channelAudit(item), v.Key != ""}
	return item, relayOpsAudit(ctx, tx, user, "CHANNEL_SAVE", "CHANNEL", strconv.FormatInt(v.ID, 10), before, after)
}
func (s *Store) ChangeOpsChannel(ctx context.Context, user, id int64, enabled *bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = changeOpsChannel(ctx, tx, user, id, enabled); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func changeOpsChannel(ctx context.Context, tx pgx.Tx, user, id int64, enabled *bool) error {
	before, err := readOpsChannel(ctx, tx, id, true)
	if err != nil {
		return err
	}
	action := "CHANNEL_DELETE"
	var after any
	if enabled == nil {
		_, err = tx.Exec(ctx, `DELETE FROM relay.channels WHERE id=$1`, id)
	} else {
		action = "CHANNEL_STATUS"
		_, err = tx.Exec(ctx, `UPDATE relay.channels SET enabled=$2,updated_at=now() WHERE id=$1`, id, *enabled)
		next := before
		next.Enabled = *enabled
		after = channelAudit(next)
	}
	if err != nil {
		return err
	}
	return relayOpsAudit(ctx, tx, user, action, "CHANNEL", strconv.FormatInt(id, 10), channelAudit(before), after)
}

type OpsGroup struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Ratio       string `json:"ratio"`
}
type GroupWrite struct {
	OpsGroup
	OriginalName string `json:"original_name"`
}

func ValidateGroup(v *GroupWrite) error {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || !ValidCatalogDecimal(v.Ratio) || v.OriginalName == "default" && v.Name != "default" {
		return ErrOpsInvalid
	}
	return nil
}
func (s *Store) OpsGroups(ctx context.Context) ([]OpsGroup, error) { return opsGroups(ctx, s.pool) }
func opsGroups(ctx context.Context, q announcementQuerier) ([]OpsGroup, error) {
	rows, err := q.Query(ctx, `SELECT name,description,ratio::text FROM relay.groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[OpsGroup])
	if items == nil {
		items = []OpsGroup{}
	}
	return items, err
}
func (s *Store) SaveOpsGroup(ctx context.Context, user int64, v GroupWrite) (OpsGroup, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return v.OpsGroup, err
	}
	defer rollback(tx)
	if err = saveOpsGroup(ctx, tx, user, v); err != nil {
		return v.OpsGroup, err
	}
	return v.OpsGroup, tx.Commit(ctx)
}
func saveOpsGroup(ctx context.Context, tx pgx.Tx, user int64, v GroupWrite) error {
	var before any
	if v.OriginalName != "" {
		var old OpsGroup
		err := tx.QueryRow(ctx, `SELECT name,description,ratio::text FROM relay.groups WHERE name=$1 FOR UPDATE`, v.OriginalName).Scan(&old.Name, &old.Description, &old.Ratio)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOpsNotFound
		}
		if err != nil {
			return err
		}
		before = old
		if _, err = tx.Exec(ctx, `UPDATE relay.groups SET name=$2,description=$3,ratio=$4::numeric WHERE name=$1`, v.OriginalName, v.Name, v.Description, v.Ratio); err != nil {
			return err
		}
		if v.Name != v.OriginalName {
			if _, err = tx.Exec(ctx, `UPDATE identity.users SET group_name=$2 WHERE group_name=$1`, v.OriginalName, v.Name); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE relay.channels SET groups=array_replace(groups,$1,$2),updated_at=now() WHERE $1=ANY(groups)`, v.OriginalName, v.Name); err != nil {
				return err
			}
		}
	} else {
		if _, err := tx.Exec(ctx, `INSERT INTO relay.groups(name,description,ratio) VALUES($1,$2,$3::numeric)`, v.Name, v.Description, v.Ratio); err != nil {
			return err
		}
	}
	return relayOpsAudit(ctx, tx, user, "GROUP_SAVE", "GROUP", v.Name, before, v.OpsGroup)
}
func (s *Store) DeleteOpsGroup(ctx context.Context, user int64, name string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = deleteOpsGroup(ctx, tx, user, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func deleteOpsGroup(ctx context.Context, tx pgx.Tx, user int64, name string) error {
	var old OpsGroup
	err := tx.QueryRow(ctx, `SELECT name,description,ratio::text FROM relay.groups WHERE name=$1 FOR UPDATE`, name).Scan(&old.Name, &old.Description, &old.Ratio)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOpsNotFound
	}
	if err != nil {
		return err
	}
	var used bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity.users WHERE group_name=$1) OR EXISTS(SELECT 1 FROM relay.channels WHERE $1=ANY(groups))`, name).Scan(&used)
	if err != nil {
		return err
	}
	if used || name == "default" {
		return ErrRelayGroupInUse
	}
	if _, err = tx.Exec(ctx, `DELETE FROM relay.groups WHERE name=$1`, name); err != nil {
		return err
	}
	return relayOpsAudit(ctx, tx, user, "GROUP_DELETE", "GROUP", name, old, nil)
}

type AvailabilityChannel struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Groups  []string `json:"groups"`
	Enabled bool     `json:"enabled"`
}
type ModelAvailability struct {
	Model    string                `json:"model"`
	Channels []AvailabilityChannel `json:"channels"`
}
type AvailabilityPage struct {
	Items    []ModelAvailability `json:"items"`
	Total    int64               `json:"total"`
	Page     int64               `json:"page"`
	PageSize int64               `json:"page_size"`
}

func (s *Store) RelayAvailability(ctx context.Context, f OpsRelayFilter) (AvailabilityPage, error) {
	return relayAvailability(ctx, s.pool, f)
}
func relayAvailability(ctx context.Context, q announcementQuerier, f OpsRelayFilter) (AvailabilityPage, error) {
	result := AvailabilityPage{Items: []ModelAvailability{}, Page: f.Page, PageSize: f.PageSize}
	const models = `SELECT DISTINCT unnest(models) AS model FROM relay.channels`
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (`+models+`) m WHERE model ILIKE '%'||$1||'%'`, f.Search).Scan(&result.Total); err != nil {
		return result, err
	}
	rows, err := q.Query(ctx, `SELECT m.model,(SELECT jsonb_agg(jsonb_build_object('id',c.id,'name',c.name,'groups',c.groups,'enabled',c.enabled) ORDER BY c.id)
 FROM relay.channels c WHERE m.model=ANY(c.models)) FROM (`+models+`) m WHERE m.model ILIKE '%'||$1||'%' ORDER BY m.model LIMIT $2 OFFSET $3`, f.Search, f.PageSize, (f.Page-1)*f.PageSize)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ModelAvailability
		if err = rows.Scan(&item.Model, &item.Channels); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

// Manual probes use the saved configuration, including imported overrides, and
// the first enabled key. They never advance the live multi-key polling cursor.
func (s *Store) OpsProbeChannel(ctx context.Context, id int64) (RelayChannel, string, error) {
	var c RelayChannel
	var models []string
	err := s.pool.QueryRow(ctx, `SELECT id,type,name,base_url,key,other,models,model_mapping,header_override,param_override,setting,settings,channel_info FROM relay.channels WHERE id=$1`, id).Scan(&c.ID, &c.Type, &c.Name, &c.BaseURL, &c.Key, &c.Other, &models, &c.ModelMapping, &c.HeaderOverride, &c.ParamOverride, &c.Setting, &c.Settings, &c.Info)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrOpsNotFound
	}
	if err != nil {
		return c, "", err
	}
	keys, enabled, err := c.enabledKeys()
	if err != nil {
		return c, "", err
	}
	if len(enabled) == 0 {
		return c, "", ErrRelayChannel
	}
	c.KeyIndex = enabled[0]
	c.Key = keys[c.KeyIndex]
	model := ""
	if len(models) > 0 {
		model = models[0]
	}
	c.UpstreamModel = model
	if mapped := c.ModelMapping[model]; mapped != "" {
		c.UpstreamModel = mapped
	}
	return c, model, nil
}
