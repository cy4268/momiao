// Copyright QuantumNous and contributors. AGPL-3.0-or-later.
// Token ownership, masking and random-key generation reuse official new-api
// f116414284162ad15d8925f7bca494c109b83e93 controller/token.go,
// model/token.go and common/utils.go. Storage is momiao's existing pgx pool.
package platform

import (
	"context"
	crand "crypto/rand"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type APIKey struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Key            string `json:"key"`
	Status         int    `json:"status"`
	Purpose        string `json:"purpose"`
	CreatedTime    int64  `json:"created_time"`
	ExpiredTime    int64  `json:"expired_time"`
	RemainQuota    int64  `json:"remain_quota"`
	UsedQuota      int64  `json:"used_quota"`
	UnlimitedQuota bool   `json:"unlimited_quota"`
}
type APIKeyPage struct {
	Items    []APIKey `json:"items"`
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
}
type APIKeyInput struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Status             int    `json:"status"`
	Purpose            string `json:"purpose"`
	RemainQuota        int64  `json:"remain_quota"`
	UnlimitedQuota     bool   `json:"unlimited_quota"`
	ExpiredTime        int64  `json:"expired_time"`
	ModelLimitsEnabled *bool  `json:"model_limits_enabled"`
	ModelLimits        string `json:"model_limits"`
	AllowIPs           string `json:"allow_ips"`
	Group              string `json:"group"`
	CrossGroupRetry    bool   `json:"cross_group_retry"`
}

func MaskTokenKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return strings.Repeat("*", len(key))
	}
	if len(key) <= 8 {
		return key[:2] + "****" + key[len(key)-2:]
	}
	return key[:4] + "**********" + key[len(key)-4:]
}
func generateAPIKey() (string, error) {
	const keyChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 48)
	maxI := big.NewInt(int64(len(keyChars)))
	for i := range b {
		n, err := crand.Int(crand.Reader, maxI)
		if err != nil {
			return "", err
		}
		b[i] = keyChars[n.Int64()]
	}
	return string(b), nil
}

const apiKeyColumns = `id,name,key,status,purpose,extract(epoch FROM created_at)::bigint,
 coalesce(extract(epoch FROM expires_at)::bigint,-1),coalesce(greatest(quota_limit-used_quota,0),0),used_quota,quota_limit IS NULL`

func scanAPIKey(row pgx.Row) (APIKey, error) {
	var item APIKey
	err := row.Scan(&item.ID, &item.Name, &item.Key, &item.Status, &item.Purpose, &item.CreatedTime, &item.ExpiredTime, &item.RemainQuota, &item.UsedQuota, &item.UnlimitedQuota)
	item.Key = MaskTokenKey(item.Key)
	return item, err
}
func (s *Store) ListAPIKeys(ctx context.Context, user int64, page, size int) (APIKeyPage, error) {
	result := APIKeyPage{Items: []APIKey{}, Page: page, PageSize: size}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM relay.api_keys WHERE user_id=$1`, user).Scan(&result.Total); err != nil {
		return result, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+apiKeyColumns+` FROM relay.api_keys WHERE user_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3`, user, size, int64(page-1)*int64(size))
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanAPIKey(rows)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}
func (s *Store) ReadAPIKey(ctx context.Context, user, id int64) (APIKey, error) {
	return scanAPIKey(s.pool.QueryRow(ctx, `SELECT `+apiKeyColumns+` FROM relay.api_keys WHERE user_id=$1 AND id=$2`, user, id))
}
func (s *Store) RevealAPIKey(ctx context.Context, user, id int64) (string, error) {
	var key string
	err := s.pool.QueryRow(ctx, `SELECT key FROM relay.api_keys WHERE user_id=$1 AND id=$2`, user, id).Scan(&key)
	return key, err
}

func (s *Store) SaveAPIKey(ctx context.Context, user int64, input APIKeyInput, mode string) (APIKey, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return APIKey{}, err
	}
	defer rollback(tx)
	if err = RequireNoMaintenance(ctx, tx, "CHALDEA_USER_WRITES"); err != nil {
		return APIKey{}, err
	}
	var item APIKey
	var expiry *time.Time
	if input.ExpiredTime != -1 {
		value := time.Unix(input.ExpiredTime, 0)
		expiry = &value
	}
	var quota *int64
	if !input.UnlimitedQuota {
		quota = &input.RemainQuota
	}
	var models []string
	if input.ModelLimitsEnabled != nil && *input.ModelLimitsEnabled {
		models = strings.FieldsFunc(input.ModelLimits, func(r rune) bool { return r == ',' || r == '\n' })
		if models == nil {
			models = []string{}
		}
	}
	switch mode {
	case "create":
		key, e := generateAPIKey()
		if e != nil {
			return item, e
		}
		item, err = scanAPIKey(tx.QueryRow(ctx, `INSERT INTO relay.api_keys(user_id,key,name,purpose,quota_limit,expires_at,models) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+apiKeyColumns, user, key, input.Name, input.Purpose, quota, expiry, models))
	case "status":
		item, err = scanAPIKey(tx.QueryRow(ctx, `SELECT `+apiKeyColumns+` FROM relay.api_keys WHERE user_id=$1 AND id=$2 FOR UPDATE`, user, input.ID))
		if err != nil {
			return item, err
		}
		if input.Status == 1 && (item.ExpiredTime != -1 && item.ExpiredTime <= time.Now().Unix() || !item.UnlimitedQuota && item.RemainQuota <= 0) {
			return item, ErrInvalidMutation
		}
		item, err = scanAPIKey(tx.QueryRow(ctx, `UPDATE relay.api_keys SET status=$3 WHERE user_id=$1 AND id=$2 RETURNING `+apiKeyColumns, user, input.ID, input.Status))
	case "purpose":
		item, err = scanAPIKey(tx.QueryRow(ctx, `UPDATE relay.api_keys SET purpose=$3 WHERE user_id=$1 AND id=$2 RETURNING `+apiKeyColumns, user, input.ID, input.Purpose))
	case "edit":
		item, err = scanAPIKey(tx.QueryRow(ctx, `UPDATE relay.api_keys SET name=$3,quota_limit=$4::bigint+used_quota,expires_at=$5,models=CASE WHEN $6::boolean IS NULL THEN models ELSE $7::text[] END WHERE user_id=$1 AND id=$2 RETURNING `+apiKeyColumns, user, input.ID, input.Name, quota, expiry, input.ModelLimitsEnabled, models))
	}
	if err != nil {
		return item, err
	}
	return item, tx.Commit(ctx)
}
func (s *Store) DeleteAPIKey(ctx context.Context, user, id int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = RequireNoMaintenance(ctx, tx, "CHALDEA_USER_WRITES"); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `DELETE FROM relay.api_keys WHERE user_id=$1 AND id=$2`, user, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}
