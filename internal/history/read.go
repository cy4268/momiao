package history

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Reader struct{ pool *pgxpool.Pool }

func NewReader(pool *pgxpool.Pool) *Reader { return &Reader{pool: pool} }

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var slug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func validSource(source Source) bool { return source == Round || source == Session || source == Hand }

func (r *Reader) List(ctx context.Context, user int64, q Query) (Page, error) {
	page := Page{Items: []Summary{}, GameOptions: []GameOption{}}
	if q.Limit == 0 {
		q.Limit = 50
	}
	if user <= 0 || q.Limit < 1 || q.Limit > 100 || (q.RecordType != "" && !validSource(q.RecordType)) ||
		(q.Mode != "" && q.Mode != "DIRECT_PLAY" && q.Mode != "POKER") ||
		(q.GameSlug != "" && (len(q.GameSlug) > 128 || !slug.MatchString(q.GameSlug))) ||
		(q.ID != "" && !uuid.MatchString(q.ID)) || (q.ParentSourceID != "" && !uuid.MatchString(q.ParentSourceID)) ||
		!slices.Contains([]string{"", "WIN", "LOSS", "BREAK_EVEN", "CANCELLED", "REFUNDED"}, q.Result) ||
		!slices.Contains([]string{"", "PROCESSING", "SETTLED", "RECOVERING", "CANCELLED", "REFUNDED"}, q.Status) {
		return page, ErrQuery
	}
	var from, to time.Time
	for i, value := range []string{q.TimeFrom, q.TimeTo} {
		if value == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, value)
		_, offset := at.Zone()
		if err != nil || offset != 0 || at.Nanosecond()%1000 != 0 {
			return page, ErrQuery
		}
		at = at.UTC()
		if i == 0 {
			from = at
			q.TimeFrom = at.Format(time.RFC3339Nano)
		} else {
			to = at
			q.TimeTo = at.Format(time.RFC3339Nano)
		}
	}
	if !from.IsZero() && !to.IsZero() && !from.Before(to) {
		return page, ErrQuery
	}
	encoded, err := json.Marshal(q)
	if err != nil {
		return page, err
	}
	hash := sha256.Sum256(encoded)
	fingerprint := hex.EncodeToString(hash[:])
	var request map[string]any
	if err = json.Unmarshal(encoded, &request); err != nil {
		return page, err
	}
	if q.Cursor != "" {
		if len(q.Cursor) > 1024 {
			return page, ErrQuery
		}
		data, e := base64.RawURLEncoding.Strict().DecodeString(q.Cursor)
		if e != nil {
			return page, ErrQuery
		}
		var c cursor
		d := json.NewDecoder(bytes.NewReader(data))
		d.DisallowUnknownFields()
		if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || c.Version != 1 || c.UserID != user || c.Filter != fingerprint ||
			!validSource(c.Type) || !uuid.MatchString(c.ID) || c.Time.IsZero() || c.Time.Location() != time.UTC || c.Time.Nanosecond()%1000 != 0 {
			return page, ErrQuery
		}
		request["before_time"], request["before_type"], request["before_id"] = c.Time.Format(time.RFC3339Nano), c.Type, c.ID
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return page, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout='2s'"); err != nil {
		return page, err
	}
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT games.history_list($1,$2::jsonb)", user, request).Scan(&raw); err != nil {
		return page, err
	}
	if err = json.Unmarshal(raw, &page); err != nil {
		return page, err
	}
	for _, item := range page.Items {
		if item.UserID != strconv.FormatInt(user, 10) {
			return Page{}, ErrQuery
		}
	}
	if page.HasMore {
		if len(page.Items) == 0 {
			return Page{}, ErrQuery
		}
		last := page.Items[len(page.Items)-1]
		b, e := json.Marshal(cursor{1, user, fingerprint, last.OccurredAt.UTC(), last.RecordType, last.SourceID})
		if e != nil {
			return Page{}, e
		}
		next := base64.RawURLEncoding.EncodeToString(b)
		page.NextCursor = &next
	}
	return page, tx.Commit(ctx)
}
