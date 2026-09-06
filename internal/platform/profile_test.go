package platform

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func TestProfileNickname(t *testing.T) {
	for _, tc := range []struct{ raw, display, key string }{
		{"  Ａlice   Smith  ", "Alice Smith", "alice smith"},
		{"Straße", "Straße", "strasse"}, {"ALICE", "ALICE", "alice"},
		{"Cafe\u0301", "Café", "café"}, {"御主_甲-7·乙", "御主_甲-7·乙", "御主_甲-7·乙"},
		{strings.Repeat("q\u0301", 24), strings.Repeat("q\u0301", 24), strings.Repeat("q\u0301", 24)},
	} {
		display, key, err := ValidateNickname(tc.raw)
		if err != nil || display != tc.display || key != tc.key {
			t.Errorf("%q => %q %q %v", tc.raw, display, key, err)
		}
	}
	for _, raw := range []string{"", "   ", "\u0301", "_\u0301", "a\n", "a\t", "a\u200b", "a\u200c", "a\u200d", "a\u202e", "a\ufe0f", "1\u20e3", "a\U000e0100", "a\u034f", "\u3164", "\u115f", "😀", "🇨🇳", "👩‍🚀", "a/b", "©", "ℹ", "™", "㊗", "a\x00", string([]byte{255}), strings.Repeat("q\u0301", 25)} {
		if _, _, err := ValidateNickname(raw); !errors.Is(err, ErrInvalidNickname) {
			t.Errorf("invalid %q: %v", raw, err)
		}
	}
	for _, raw := range []string{"Admin", "ＡＤＭＩＮ", "A_d-m·i n", "Administrator", "Moderator", "Official", "System", "Support", "Chaldea", "NewAPI", "管理员", "官 方", "系统", "客服", "迦勒底", "ＭＯＭＩＡＯ"} {
		if _, _, err := ValidateNickname(raw); !errors.Is(err, ErrNicknameReserved) {
			t.Errorf("reserved %q: %v", raw, err)
		}
	}
	if _, _, err := ValidateNickname("Administer"); err != nil {
		t.Fatal("reserved matching must be exact separator-stripped equality", err)
	}
}

func TestProfileShortAccountID(t *testing.T) {
	for _, tc := range []struct {
		user int64
		want string
	}{{1, "CA-1168B35FDDF8"}, {9007199254740993, "CA-76F010C76AD3"}, {9223372036854775807, "CA-E75D15B97C60"}} {
		if got := ShortAccountID(tc.user); got != tc.want {
			t.Errorf("%d: %s", tc.user, got)
		}
	}
}

func TestProfileNormalizedStorageBound(t *testing.T) {
	// U+0344 expands under NFKC; raw input can fit while normalized text exceeds
	// the profile column limit. This must be a validation error, not a DB outage.
	for _, raw := range []string{"q" + strings.Repeat("\u0344", 1400), strings.Repeat("q", 4097)} {
		if _, _, err := ValidateNickname(raw); !errors.Is(err, ErrInvalidNickname) {
			t.Fatal("storage limit accepted", err)
		}
	}
}

func TestProfileMigrationManifest(t *testing.T) {
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil || len(files) < 8 {
		t.Fatalf("want migration baseline through bootstrap v8: %v %v", files, err)
	}
	original, err := migrations.ReadFile("migrations/0001_wallet.sql")
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(original)) != "6db5d8fac468dbfac4eebe78dfd9af60f4c680b3206836898e61237b377b7cd9" {
		t.Fatal("original migration bytes changed", err)
	}
}
