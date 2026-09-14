package fairness

import (
	"encoding/hex"
	"io"
	"reflect"
	"testing"
)

func TestIS07Golden(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	p, err := Derive("018f47a2-6e9d-7c31-8a4b-123456789abc", "019047a2-6e9d-7c31-8a4b-abcdef012345", []Contribution{{Seat: 7, Version: 5, Value: "beta"}, {Seat: 2, Version: 3, Value: "alpha"}}, seed)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(p.EffectiveSeed[:]) != "1d820ef6e80ca45951e3b9f6fb6a3cceb31a583445e1ba7f290012199d5643db" {
		t.Fatalf("effective %x", p.EffectiveSeed)
	}
	s, err := NewStream("018f47a2-6e9d-7c31-8a4b-123456789abc", "019047a2-6e9d-7c31-8a4b-abcdef012345", p.EffectiveSeed, seed)
	if err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 32)
	if _, err := io.ReadFull(s, block); err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(block) != "e45a7cb6998add6bf7305dc92853eca6630cc4cd05834efe45285e2f6bd415ce" {
		t.Fatalf("block0 %x", block)
	}
	if !reflect.DeepEqual(p.Deck[:15], []byte{46, 25, 19, 38, 44, 28, 6, 16, 22, 8, 11, 1, 47, 23, 35}) {
		t.Fatalf("deck prefix %v", p.Deck[:15])
	}
	if hex.EncodeToString(p.DeckHash[:]) != "b04f7b14688a3a1dfe9f42bfb64cc7edb708bd88e2d710e2798a878ff94bc70d" {
		t.Fatalf("deck hash %x", p.DeckHash)
	}
}
