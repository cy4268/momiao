package poker

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/cy4268/momiao/internal/poker/fairness"
	"github.com/jackc/pgx/v5"
	"strconv"
	"time"
)

type FairnessContribution struct {
	Seat    int    `json:"seat_no"`
	Version string `json:"contribution_version"`
	Value   string `json:"contribution"`
}
type FairnessView struct {
	HandID              string                 `json:"hand_id"`
	Released            bool                   `json:"released"`
	RevealAt            *time.Time             `json:"reveal_at,omitempty"`
	ServerSeedHash      string                 `json:"server_seed_hash"`
	DeckHash            string                 `json:"deck_hash"`
	AlgorithmVersion    string                 `json:"algorithm_version"`
	ServerSeed          string                 `json:"server_seed,omitempty"`
	EffectiveClientSeed string                 `json:"effective_client_seed,omitempty"`
	Contributions       []FairnessContribution `json:"contributions,omitempty"`
	Deck                []int                  `json:"deck,omitempty"`
}

// Fairness is a separate participant-authorized endpoint. It is never embedded
// in table broadcasts; membership survives cashout because it uses the hand set.
func (s *Service) Fairness(ctx context.Context, hand string, user int64) (FairnessView, error) {
	tx, err := s.opts.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return FairnessView{}, err
	}
	defer rollback(tx)
	return readFairness(ctx, tx, s.opts.Keyring, hand, user)
}

func readFairness(ctx context.Context, tx pgx.Tx, keys Keyring, hand string, user int64) (FairnessView, error) {
	var status, table string
	var now time.Time
	var seedHash, deckHash, effectiveHash, seedCipher, setupCipher []byte
	v := FairnessView{HandID: hand}
	err := tx.QueryRow(ctx, `SELECT h.table_id::text,h.state,h.setup_cipher,f.server_seed_hash,f.deck_hash,f.effective_client_seed_hash,f.encrypted_server_seed,f.algorithm_version,f.full_fairness_reveal_at,clock_timestamp() FROM poker.hands h JOIN poker.hand_fairness f USING(hand_id) WHERE h.hand_id=$1 AND EXISTS(SELECT 1 FROM poker.hand_participants hp WHERE hp.hand_id=h.hand_id AND hp.newapi_user_id=$2)`, hand, user).Scan(&table, &status, &setupCipher, &seedHash, &deckHash, &effectiveHash, &seedCipher, &v.AlgorithmVersion, &v.RevealAt, &now)
	if errors.Is(err, pgx.ErrNoRows) {
		return FairnessView{}, ErrDenied
	}
	if err != nil {
		return v, err
	}
	v.ServerSeedHash = hex.EncodeToString(seedHash)
	v.DeckHash = hex.EncodeToString(deckHash)
	if !fairnessReleased(status, v.RevealAt, now) {
		return v, nil
	}
	seed, err := openCipher(keys, hand+":server_seed", seedCipher)
	if err != nil {
		return v, err
	}
	plain, err := openCipher(keys, hand+":setup", setupCipher)
	if err != nil {
		return v, err
	}
	var setup handSetup
	if err = json.Unmarshal(plain, &setup); err != nil {
		return v, err
	}
	proof, err := fairness.Derive(table, hand, setup.Contributions, seed)
	if err != nil {
		return v, err
	}
	if v.AlgorithmVersion != fairness.AlgorithmVersion || !equal(proof.ServerSeedHash[:], seedHash) || !equal(proof.DeckHash[:], deckHash) || !equal(proof.EffectiveSeed[:], effectiveHash) || len(setup.Config.Deck) != 52 {
		return v, ErrInvalid
	}
	for i, card := range proof.Deck {
		if uint8(setup.Config.Deck[i]) != card {
			return v, ErrInvalid
		}
	}
	v.Released = true
	v.ServerSeed = hex.EncodeToString(seed)
	v.EffectiveClientSeed = hex.EncodeToString(proof.EffectiveSeed[:])
	v.Deck = make([]int, len(proof.Deck))
	for i, card := range proof.Deck {
		v.Deck[i] = int(card)
	}
	for _, c := range setup.Contributions {
		v.Contributions = append(v.Contributions, FairnessContribution{Seat: c.Seat, Version: strconv.FormatUint(c.Version, 10), Value: c.Value})
	}
	return v, nil
}

func fairnessReleased(status string, revealAt *time.Time, now time.Time) bool {
	return status == "SETTLED" && revealAt != nil && !revealAt.After(now)
}
