package platform

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

var (
	ErrInvalidNickname  = errors.New("invalid nickname")
	ErrNicknameReserved = errors.New("reserved nickname")
)

// ValidateNickname preserves normalized capitalization and returns the separate
// case-folded uniqueness key. Reserved matching is exact equality after removing
// the four allowed separators, not exhaustive homoglyph or content moderation.
func ValidateNickname(raw string) (string, string, error) {
	if !utf8.ValidString(raw) || len(raw) > 4096 {
		return "", "", ErrInvalidNickname
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp, unicode.S) ||
			unicode.Is(unicode.Properties["Variation_Selector"], r) || unicode.Is(unicode.Properties["Other_Default_Ignorable_Code_Point"], r) || r == 0x034f || r == 0x20e3 || r == 0x2139 {
			return "", "", ErrInvalidNickname
		}
	}
	display := strings.Join(strings.FieldsFunc(norm.NFKC.String(raw), func(r rune) bool { return r == ' ' }), " ")
	if len(display) > 4096 {
		return "", "", ErrInvalidNickname
	}
	gr := uniseg.NewGraphemes(display)
	count := 0
	for gr.Next() {
		count++
		attached := false
		for _, r := range gr.Runes() {
			switch {
			case unicode.IsLetter(r) || unicode.IsNumber(r):
				attached = true
			case unicode.IsMark(r):
				if !attached {
					return "", "", ErrInvalidNickname
				}
			case strings.ContainsRune(" _-·", r):
				attached = false
			default:
				return "", "", ErrInvalidNickname
			}
		}
	}
	if count < 1 || count > 24 {
		return "", "", ErrInvalidNickname
	}
	key := cases.Fold().String(display)
	if len(key) > 8192 {
		return "", "", ErrInvalidNickname
	}
	reserved := strings.Map(func(r rune) rune {
		if strings.ContainsRune(" _-·", r) {
			return -1
		}
		return r
	}, key)
	switch reserved {
	case "admin", "administrator", "moderator", "official", "system", "support", "chaldea", "newapi", "管理员", "官方", "系统", "客服", "迦勒底", "momiao":
		return "", "", ErrNicknameReserved
	}
	return display, key, nil
}

// ShortAccountID is a private presentation identifier, never an auth credential.
func ShortAccountID(userID int64) string {
	digest := sha256.Sum256([]byte("chaldea-short-account-id-v1\x00" + strconv.FormatInt(userID, 10)))
	return fmt.Sprintf("CA-%X", digest[:6])
}
