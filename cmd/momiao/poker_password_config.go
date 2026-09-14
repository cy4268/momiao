package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/poker"
)

type pokerPasswordConfig struct {
	Policy             poker.PasswordPolicy
	OwnerAttempts      uint32
	OwnerWindow        time.Duration
	OwnerTableAttempts uint32
	OwnerTableWindow   time.Duration
}

func readPokerPasswordConfig(lookup func(string) (string, bool)) (*pokerPasswordConfig, error) {
	if lookup == nil {
		return nil, errPokerConfig
	}
	names := []string{
		"POKER_TABLE_PASSWORD_ARGON2_MEMORY_KIB",
		"POKER_TABLE_PASSWORD_ARGON2_TIME",
		"POKER_TABLE_PASSWORD_ARGON2_PARALLELISM",
		"POKER_TABLE_PASSWORD_VERIFY_PROFILES_JSON",
		"POKER_TABLE_PASSWORD_MAX_CONCURRENT",
		"POKER_TABLE_PASSWORD_MAX_WORK_MEMORY_KIB",
		"POKER_TABLE_PASSWORD_OWNER_ATTEMPTS",
		"POKER_TABLE_PASSWORD_OWNER_WINDOW_MS",
		"POKER_TABLE_PASSWORD_OWNER_TABLE_ATTEMPTS",
		"POKER_TABLE_PASSWORD_OWNER_TABLE_WINDOW_MS",
	}
	values, present := map[string]string{}, map[string]bool{}
	any := false
	for _, name := range names {
		values[name], present[name] = lookup(name)
		any = any || present[name]
	}
	if !any {
		return nil, nil
	}
	for _, name := range names {
		if name != "POKER_TABLE_PASSWORD_VERIFY_PROFILES_JSON" && (!present[name] || values[name] == "") {
			return nil, errPokerConfig
		}
	}
	read := func(name string, max uint64) (uint64, error) {
		return canonicalPasswordUint(values[name], max)
	}
	memory, e1 := read(names[0], 262144)
	iterations, e2 := read(names[1], 10)
	lanes, e3 := read(names[2], 16)
	concurrent, e4 := read(names[4], 16)
	workMemory, e5 := read(names[5], 262144)
	owner, e6 := read(names[6], uint64(^uint32(0)))
	ownerWindow, e7 := read(names[7], 86400000)
	ownerTable, e8 := read(names[8], uint64(^uint32(0)))
	ownerTableWindow, e9 := read(names[9], 86400000)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || e6 != nil || e7 != nil || e8 != nil || e9 != nil || owner == 0 || ownerWindow == 0 || ownerTable == 0 || ownerTableWindow == 0 {
		return nil, errPokerConfig
	}
	policy := poker.PasswordPolicy{Current: poker.PasswordConfig{MemoryKiB: uint32(memory), Time: uint32(iterations), Parallelism: uint8(lanes)}, MaxConcurrent: uint8(concurrent), MaxWorkMemoryKiB: uint32(workMemory)}
	if present[names[3]] {
		policy.VerifyProfiles, e1 = readPokerPasswordProfiles(values[names[3]])
		if e1 != nil {
			return nil, errPokerConfig
		}
	}
	if poker.ValidatePasswordPolicy(policy) != nil {
		return nil, errPokerConfig
	}
	return &pokerPasswordConfig{policy, uint32(owner), time.Duration(ownerWindow) * time.Millisecond, uint32(ownerTable), time.Duration(ownerTableWindow) * time.Millisecond}, nil
}

func canonicalPasswordUint(raw string, max uint64) (uint64, error) {
	if raw == "" || len(raw) > 1 && raw[0] == '0' {
		return 0, errPokerConfig
	}
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, errPokerConfig
		}
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 || n > max || strconv.FormatUint(n, 10) != raw {
		return 0, errPokerConfig
	}
	return n, nil
}

func readPokerPasswordProfiles(raw string) ([]poker.PasswordConfig, error) {
	body := bytes.TrimSpace([]byte(raw))
	if len(raw) > 4096 || len(body) == 0 || body[0] != '[' || !json.Valid(body) {
		return nil, errPokerConfig
	}
	d := json.NewDecoder(bytes.NewReader(body))
	if !nativeself.UniqueJSON(d, 0) {
		return nil, errPokerConfig
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errPokerConfig
	}
	var rows []map[string]json.RawMessage
	if json.Unmarshal(body, &rows) != nil || rows == nil || len(rows) > 4 {
		return nil, errPokerConfig
	}
	out := make([]poker.PasswordConfig, 0, len(rows))
	for _, row := range rows {
		if len(row) != 3 || row["memory_kib"] == nil || row["time"] == nil || row["parallelism"] == nil {
			return nil, errPokerConfig
		}
		memory, e1 := canonicalPasswordUint(string(row["memory_kib"]), 262144)
		iterations, e2 := canonicalPasswordUint(string(row["time"]), 10)
		lanes, e3 := canonicalPasswordUint(string(row["parallelism"]), 16)
		if e1 != nil || e2 != nil || e3 != nil {
			return nil, errPokerConfig
		}
		out = append(out, poker.PasswordConfig{MemoryKiB: uint32(memory), Time: uint32(iterations), Parallelism: uint8(lanes)})
	}
	return out, nil
}
