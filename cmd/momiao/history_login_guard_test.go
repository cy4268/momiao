package main

import (
	"os"
	"strings"
	"testing"
)

// This is a source guard, not restricted-role database startup acceptance.
func TestHistoryPoolExactLoginSourceGuard(t *testing.T) {
	for _, tc := range []struct{ file, predicate, binding string }{
		{"history_application.go", "session_user=current_user AND current_user=$2::name", "capability, pool.Config().ConnConfig.User"},
		{"poker_application.go", "session_user=current_user AND current_user=$1::name", "pool.Config().ConnConfig.User).Scan(&role"},
	} {
		b, err := os.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), tc.predicate) || !strings.Contains(string(b), tc.binding) {
			t.Errorf("%s must validate the original login and configured pool user, not only current role", tc.file)
		}
	}
}
