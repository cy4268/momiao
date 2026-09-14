package transport

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"unicode/utf8"
)

// SafeNumber preserves the frozen JSON-number envelope while rejecting values
// browsers cannot compare exactly. Payload decimal strings are not coerced.
type SafeNumber uint64

func (n *SafeNumber) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return errVersion
	}
	for _, c := range b {
		if c < '0' || c > '9' {
			return errVersion
		}
	}
	v, err := strconv.ParseUint(string(b), 10, 64)
	if err != nil || v > MaxSafeInteger {
		return errVersion
	}
	*n = SafeNumber(v)
	return nil
}
func (n SafeNumber) MarshalJSON() ([]byte, error) {
	if uint64(n) > MaxSafeInteger {
		return nil, errVersion
	}
	return []byte(strconv.FormatUint(uint64(n), 10)), nil
}

type clientEnvelope struct {
	Type                 string          `json:"type"`
	RequestID            string          `json:"request_id"`
	TableID              string          `json:"table_id"`
	HandID               *string         `json:"hand_id"`
	ExpectedTableVersion SafeNumber      `json:"expected_table_version"`
	ExpectedHandVersion  SafeNumber      `json:"expected_hand_version"`
	ControlEpoch         SafeNumber      `json:"control_epoch"`
	ActionID             *string         `json:"action_id"`
	Payload              json.RawMessage `json:"payload"`
}

// Walk tokens first: encoding/json otherwise accepts duplicate/case-folded
// fields. Token walking is size-bounded by both HTTP and WS callers.
func uniqueJSON(d *json.Decoder, depth int) error {
	if depth > 32 {
		return errInvalid
	}
	tok, err := d.Token()
	if err != nil {
		return errInvalid
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			s, ok := k.(string)
			if e != nil || !ok || seen[s] {
				return errInvalid
			}
			seen[s] = true
			if err := uniqueJSON(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueJSON(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errInvalid
	}
	_, err = d.Token()
	return err
}
func strictJSON(b []byte, out any, fields ...string) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || b[0] != '{' || !utf8.Valid(b) {
		return errInvalid
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := uniqueJSON(d, 0); err != nil {
		return errInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return errInvalid
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(b, &raw) != nil {
		return errInvalid
	}
	allowed := map[string]bool{}
	for _, f := range fields {
		allowed[f] = true
	}
	for k := range raw {
		if !allowed[k] {
			return errInvalid
		}
		if bytes.Equal(bytes.TrimSpace(raw[k]), []byte("null")) {
			// JSON null is allowed only by an explicit nullable pointer slot;
			// encoding/json's silent null-to-false/zero conversion is forbidden.
			typ := reflect.TypeOf(out).Elem()
			nullable := false
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				if f.Tag.Get("json") == k && f.Type.Kind() == reflect.Pointer {
					nullable = true
					break
				}
			}
			if !nullable {
				return errInvalid
			}
		}
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	return nil
}
func decodeClient(b []byte) (clientEnvelope, error) {
	var e clientEnvelope
	err := strictJSON(b, &e, "type", "request_id", "table_id", "hand_id", "expected_table_version", "expected_hand_version", "control_epoch", "action_id", "payload")
	if err != nil {
		return e, err
	}
	// Require every envelope slot, including nullable fields. This also prevents
	// absent optimistic-concurrency fields from silently becoming zero.
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(b, &raw)
	if len(raw) != 9 || !validKey(e.RequestID) || !validUUID(e.TableID) || e.HandID != nil && !validUUID(*e.HandID) || e.ActionID != nil && !validKey(*e.ActionID) || len(e.Payload) == 0 || e.Payload[0] != '{' {
		return e, errInvalid
	}
	return e, nil
}
func validKey(s string) bool {
	if len(s) < 16 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func validUUID(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	b := make([]byte, 0, 32)
	for _, c := range []byte(s) {
		if c != '-' {
			b = append(b, c)
		}
	}
	_, err := hex.DecodeString(string(b))
	return err == nil
}
func validAmount(s string, positive bool) bool {
	if s == "" || len(s) > 19 || len(s) > 1 && s[0] == '0' {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return err == nil && n%500000 == 0 && (!positive || n > 0)
}
