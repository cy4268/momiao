package poker

import (
	"context"
	"strings"
	"testing"
)

func TestControlTableNameFortyGraphemeBoundary(t *testing.T) {
	f := newControlFixture(t)
	for _, n := range []int{40, 41} {
		t.Run(string(rune('A'+n-40)), func(t *testing.T) {
			_, err := f.service.CreateTable(context.Background(), CreateTableCommand{UserID: 910001, Key: "table-name-grapheme-" + uuid(), Name: strings.Repeat("👨‍👩‍👧‍👦", n), BlindPreset: "5-10", MaxSeats: 2})
			if n == 40 && err != nil {
				t.Fatal("forty extended graphemes rejected", err)
			}
			if n == 41 && err == nil {
				t.Fatal("forty-one extended graphemes accepted")
			}
		})
	}
}
