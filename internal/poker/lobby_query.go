package poker

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/rivo/uniseg"
)

type lobbyCursor struct {
	Version  int    `json:"v"`
	Filter   string `json:"filter"`
	Sort     string `json:"sort"`
	TableID  string `json:"table_id"`
	BigBlind string `json:"big_blind_units"`
	Occupied int    `json:"occupied"`
	MaxSeats int    `json:"max_seats"`
}

func lobbyFilterHash(f LobbyFilter) string {
	f.Cursor = ""
	b, _ := json.Marshal(f)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func normalizeLobbyFilter(f LobbyFilter) (LobbyFilter, lobbyCursor, error) {
	var c lobbyCursor
	f.Query = strings.TrimSpace(f.Query)
	for _, field := range []*string{&f.AccessMode, &f.BlindPreset, &f.LifecycleState} {
		if *field == "" {
			*field = "ALL"
		}
	}
	if f.Sort == "" {
		f.Sort = "LOW_BLIND"
	}
	if f.Limit == 0 {
		f.Limit = 50
	}
	if !utf8.ValidString(f.Query) || uniseg.GraphemeClusterCount(f.Query) > 40 || strings.IndexFunc(f.Query, unicode.IsControl) >= 0 || f.Limit < 1 || f.Limit > 100 || f.MaxSeats != 0 && (f.MaxSeats < 2 || f.MaxSeats > 9) || len(f.BlindPreset) > 64 || f.AccessMode != "ALL" && f.AccessMode != "PUBLIC" && f.AccessMode != "PASSWORD" || f.Sort != "LOW_BLIND" && f.Sort != "HIGH_BLIND" && f.Sort != "NEAR_FULL" {
		return f, c, ErrInvalid
	}
	switch f.LifecycleState {
	case "ALL", "WAITING", "IN_HAND", "INTERMISSION", "PAUSED", "RECOVERING", "CLOSING", "CLOSED":
	default:
		return f, c, ErrInvalid
	}
	if f.Cursor != "" {
		if len(f.Cursor) > 512 {
			return f, c, ErrInvalid
		}
		body, err := base64.RawURLEncoding.Strict().DecodeString(f.Cursor)
		if err != nil || json.Unmarshal(body, &c) != nil {
			return f, c, ErrInvalid
		}
		canonical, _ := json.Marshal(c)
		blind, err := strconv.ParseInt(c.BigBlind, 10, 64)
		if string(canonical) != string(body) || c.Version != 1 || c.Filter != lobbyFilterHash(f) || c.Sort != f.Sort || !canonicalSessionID(c.TableID) || err != nil || !units(blind) || blind <= 0 || decimal(blind) != c.BigBlind || c.MaxSeats < 2 || c.MaxSeats > 9 || c.Occupied < 0 || c.Occupied > c.MaxSeats {
			return f, c, ErrInvalid
		}
	}
	f.Cursor = ""
	return f, c, nil
}

func readLobbyTables(ctx context.Context, tx pgx.Tx, f LobbyFilter, cursor lobbyCursor) ([]LobbyTable, *string, error) {
	order, after := "big_blind_units,table_id", "(big_blind_units,table_id)>($9::bigint,$10::uuid)"
	if f.Sort == "HIGH_BLIND" {
		order, after = "big_blind_units DESC,table_id", "(big_blind_units<$9 OR (big_blind_units=$9 AND table_id>$10::uuid))"
	} else if f.Sort == "NEAR_FULL" {
		order, after = "occupied::numeric/max_seats DESC,table_id", "(occupied*$12<$11*max_seats OR (occupied*$12=$11*max_seats AND table_id>$10::uuid))"
	}
	query := `WITH cursor_anchor AS (SELECT $9::bigint c_blind,$10::uuid c_table,$11::integer c_occupied,$12::integer c_max), summary AS (
 SELECT t.table_id,t.table_version,t.name,t.access_mode,t.blind_preset_version,t.ruleset_version,t.lifecycle_state,t.max_seats,
 count(s.session_id)::integer occupied,coalesce(array_agg(s.seat_no ORDER BY s.seat_no) FILTER(WHERE s.seat_no IS NOT NULL AND s.session_id IS NULL),'{}') free_seats,
 t.accepting_players,t.allow_new_hands,t.allow_spectators,t.chat_enabled,
 b.small_blind_units,b.big_blind_units,b.minimum_buyin_bb,b.maximum_buyin_bb,
 EXISTS(SELECT 1 FROM poker.recovery_state r WHERE r.table_id=t.table_id AND r.state='NEEDS_REVIEW') needs_review
 FROM poker.tables t JOIN poker.blind_preset_versions b ON b.version=t.blind_preset_version
 LEFT JOIN poker.seats s ON s.table_id=t.table_id
 WHERE (t.name ILIKE $1 ESCAPE E'\\' OR t.table_id::text ILIKE $1 ESCAPE E'\\')
 AND ($2='ALL' OR t.access_mode=$2) AND ($3=0 OR t.max_seats=$3)
 AND ($4='ALL' OR t.blind_preset_version=$4) AND ($5='ALL' OR t.lifecycle_state=$5)
 AND (NOT $6 OR t.allow_spectators) GROUP BY t.table_id,b.version)
 SELECT table_id::text,table_version,name,access_mode,blind_preset_version,ruleset_version,lifecycle_state,max_seats,occupied,free_seats,
 accepting_players,allow_new_hands,allow_spectators,chat_enabled,small_blind_units,big_blind_units,minimum_buyin_bb,maximum_buyin_bb,needs_review
 FROM summary CROSS JOIN cursor_anchor WHERE (NOT $7 OR occupied<max_seats) AND (NOT $8 OR ` + after + `) ORDER BY ` + order + ` LIMIT $13`
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(f.Query) + "%"
	hasCursor := cursor.TableID != ""
	if !hasCursor {
		cursor.TableID = "00000000-0000-0000-0000-000000000000"
	}
	blind, _ := strconv.ParseInt(cursor.BigBlind, 10, 64)
	rows, err := tx.Query(ctx, query, pattern, f.AccessMode, f.MaxSeats, f.BlindPreset, f.LifecycleState, f.SpectatorsOnly, f.OpenSeatsOnly, hasCursor, blind, cursor.TableID, cursor.Occupied, cursor.MaxSeats, f.Limit+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []LobbyTable{}
	for rows.Next() {
		var row LobbyTable
		var sb, bb int64
		var minimum, maximum int
		if err = rows.Scan(&row.TableID, &row.TableVersion, &row.Name, &row.Visibility, &row.BlindPresetID, &row.ruleset, &row.LifecycleState, &row.MaxSeats, &row.OccupiedSeats, &row.OpenSeatNumbers, &row.AcceptingPlayers, &row.AllowNewHands, &row.AllowSpectators, &row.ChatEnabled, &sb, &bb, &minimum, &maximum, &row.needsReview); err != nil {
			return nil, nil, err
		}
		if row.LobbyBlinds, err = lobbyBlinds(sb, bb, minimum, maximum); err != nil {
			return nil, nil, err
		}
		out = append(out, row)
	}
	var next *string
	if len(out) > f.Limit {
		out = out[:f.Limit]
		last := out[len(out)-1]
		b, _ := json.Marshal(lobbyCursor{1, lobbyFilterHash(f), f.Sort, last.TableID, last.BigBlindUnits, last.OccupiedSeats, last.MaxSeats})
		value := base64.RawURLEncoding.EncodeToString(b)
		next = &value
	}
	return out, next, rows.Err()
}
