package platform

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// RegistrationReceipt is a committed native receipt, never client input.
type RegistrationReceipt struct {
	Ordinal        int64     `json:"ordinal"`
	OperationID    string    `json:"operation_id"`
	NativeUserID   int64     `json:"native_user_id"`
	DiscordSubject string    `json:"discord_subject"`
	Source         string    `json:"source"`
	PolicyVersion  string    `json:"policy_version"`
	CreatedAt      time.Time `json:"created_at"`
}
type RegistrationPage struct {
	Receipts   []RegistrationReceipt `json:"receipts"`
	NextCursor int64                 `json:"next_cursor"`
}

var registrationUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var registrationSubject = regexp.MustCompile(`^[1-9][0-9]{16,19}$`)
var ErrRegistrationReceipt = errors.New("invalid registration receipt page")

func ValidateRegistrationPage(page RegistrationPage, after int64, limit int) error {
	if after < 0 || limit < 1 || limit > 100 || page.Receipts == nil || len(page.Receipts) > limit {
		return ErrRegistrationReceipt
	}
	cursor := after
	users, operations := map[int64]bool{}, map[string]bool{}
	for _, r := range page.Receipts {
		if r.Ordinal != cursor+1 || r.NativeUserID <= 0 || !registrationUUID.MatchString(r.OperationID) || !registrationSubject.MatchString(r.DiscordSubject) || r.Source != "NEW_DISCORD_REGISTRATION" || len(r.PolicyVersion) == 0 || len(r.PolicyVersion) > 64 || !utf8.ValidString(r.PolicyVersion) || strings.ContainsAny(r.PolicyVersion, "\r\n\x00") || r.CreatedAt.IsZero() || users[r.NativeUserID] || operations[r.OperationID] {
			return ErrRegistrationReceipt
		}
		users[r.NativeUserID], operations[r.OperationID] = true, true
		cursor = r.Ordinal
	}
	if page.NextCursor != cursor {
		return ErrRegistrationReceipt
	}
	return nil
}
