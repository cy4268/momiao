package games

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"

	"github.com/cy4268/momiao/internal/games/fairness"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func lockUserGame(ctx context.Context, tx pgx.Tx, user int64, slug string) error {
	if user <= 0 {
		return ErrInvalidInput
	}
	hash := sha256.Sum256([]byte("games.user.v1\x00" + strconv.FormatInt(user, 10) + "\x00" + slug))
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(binary.BigEndian.Uint64(hash[:8])))
	return err
}
func preference(ctx context.Context, tx pgx.Tx, user int64, slug string) (ClientSeedPreference, error) {
	var p ClientSeedPreference
	err := tx.QueryRow(ctx, `SELECT client_seed,version FROM games.client_seed_preferences WHERE newapi_user_id=$1 AND game_slug=$2`, user, slug).Scan(&p.Seed, &p.Version)
	if !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return p, err
	}
	p.Seed = "cs1-" + hex.EncodeToString(raw[:])
	p.Version = 1
	_, err = tx.Exec(ctx, `INSERT INTO games.client_seed_preferences(newapi_user_id,game_slug,client_seed) VALUES($1,$2,$3)`, user, slug, p.Seed)
	return p, err
}

const commitmentColumns = `commitment_id::text,reserved_round_id::text,encode(server_seed_hash,'hex'),nonce,client_seed,client_seed_version,game_config_version_id::text,encode(game_config_hash,'hex'),ruleset_version,algorithm_version,fairness_stream_version,wager_policy_version_id::text,encode(wager_policy_hash,'hex'),resource_versions`

func scanCommitment(row pgx.Row) (Commitment, error) {
	var c Commitment
	err := row.Scan(&c.ID, &c.ReservedRoundID, &c.ServerSeedHash, &c.Nonce, &c.ClientSeed, &c.ClientVersion, &c.ConfigVersion, &c.ConfigHash, &c.Ruleset, &c.Algorithm, &c.Stream, &c.PolicyVersion, &c.PolicyHash, &c.Resources)
	return c, err
}
func compatible(c Commitment, config runtimeConfig, p ClientSeedPreference) bool {
	if c.ConfigVersion != config.Entry.Config.Version || c.ConfigHash != config.Entry.Config.Hash || c.Ruleset != config.Entry.Config.Ruleset || c.Algorithm != config.Entry.Config.Algorithm || c.Stream != fairness.StreamVersion || c.PolicyVersion != config.Policy.ID || c.PolicyHash != config.Policy.Hash || c.ClientVersion != p.Version || c.ClientSeed != p.Seed {
		return false
	}
	var resources map[string]string
	if json.Unmarshal(c.Resources, &resources) != nil {
		return false
	}
	raw, _ := json.Marshal(resources)
	return bytes.Equal(raw, expectedResources(config.Config))
}
func (s *Service) nextCommitment(ctx context.Context, tx pgx.Tx, user int64, slug string, config runtimeConfig, p ClientSeedPreference) (Commitment, error) {
	c, err := scanCommitment(tx.QueryRow(ctx, `SELECT `+commitmentColumns+` FROM games.fairness_commitments WHERE newapi_user_id=$1 AND game_slug=$2 AND state='AVAILABLE' FOR UPDATE`, user, slug))
	if err == nil && compatible(c, config, p) {
		return c, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return c, err
	}
	if err == nil {
		if _, err = tx.Exec(ctx, `UPDATE games.fairness_commitments SET state='INVALIDATED' WHERE commitment_id=$1`, c.ID); err != nil {
			return c, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO games.fairness_nonce_cursors(newapi_user_id,game_slug) VALUES($1,$2) ON CONFLICT DO NOTHING`, user, slug); err != nil {
		return c, err
	}
	var nonce int64
	var exhausted bool
	err = tx.QueryRow(ctx, `SELECT next_nonce,exhausted FROM games.fairness_nonce_cursors WHERE newapi_user_id=$1 AND game_slug=$2 FOR UPDATE`, user, slug).Scan(&nonce, &exhausted)
	if err != nil {
		return c, err
	}
	if exhausted {
		return c, ErrNonceExhausted
	}
	next := nonce
	if nonce < math.MaxInt64 {
		next++
	}
	if _, err = tx.Exec(ctx, `UPDATE games.fairness_nonce_cursors SET next_nonce=$3,exhausted=$4 WHERE newapi_user_id=$1 AND game_slug=$2`, user, slug, next, nonce == math.MaxInt64); err != nil {
		return c, err
	}
	id, err := newUUID()
	if err != nil {
		return c, err
	}
	round, err := newUUID()
	if err != nil {
		return c, err
	}
	var seed [32]byte
	if _, err = rand.Read(seed[:]); err != nil {
		return c, err
	}
	hash := sha256.Sum256(seed[:])
	binding := config.Config.Binding()
	c = Commitment{ID: id, ReservedRoundID: round, ServerSeedHash: hex.EncodeToString(hash[:]), Nonce: nonce, ClientSeed: p.Seed, ClientVersion: p.Version, ConfigVersion: binding.Version, ConfigHash: hex.EncodeToString(binding.Hash[:]), Ruleset: binding.RulesetVersion, Algorithm: binding.AlgorithmVersion, Stream: fairness.StreamVersion, PolicyVersion: config.Policy.ID, PolicyHash: config.Policy.Hash, Resources: expectedResources(config.Config)}
	gcmNonce, encrypted, err := s.sealSeed(user, slug, c, seed[:])
	if err != nil {
		return c, err
	}
	policyHash, _ := hex.DecodeString(c.PolicyHash)
	_, err = tx.Exec(ctx, `INSERT INTO games.fairness_commitments(commitment_id,reserved_round_id,newapi_user_id,game_slug,nonce,client_seed,client_seed_version,state,server_seed_hash,key_version,gcm_nonce,ciphertext,ruleset_version,algorithm_version,fairness_stream_version,game_config_version_id,game_config_hash,wager_policy_version_id,wager_policy_hash,resource_versions) VALUES($1,$2,$3,$4,$5,$6,$7,'AVAILABLE',$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, c.ID, c.ReservedRoundID, user, slug, c.Nonce, c.ClientSeed, c.ClientVersion, hash[:], s.keyring.Active, gcmNonce, encrypted, c.Ruleset, c.Algorithm, c.Stream, c.ConfigVersion, binding.Hash[:], c.PolicyVersion, policyHash, []byte(c.Resources))
	return c, err
}
func (s *Service) Bootstrap(ctx context.Context, user int64, slug string) (Bootstrap, error) {
	var b Bootstrap
	if slug != "dice" && slug != "scratch" && slug != "summon" && slug != "slot" && slug != "blackjack" {
		return b, ErrNotFound
	}
	if err := s.store.EnsureAccount(ctx, user); err != nil {
		return b, err
	}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := lockUserGame(ctx, tx, user, slug); err != nil {
			return err
		}
		runtime, err := resolveRuntime(ctx, tx, slug, true)
		if err != nil {
			return err
		}
		b.Game = runtime.Entry
		b.WagerPolicy = runtime.Policy
		b.EntryAction = runtime.Entry.State
		if err = tx.QueryRow(ctx, `SELECT balance_units FROM economy.wallet_balances WHERE newapi_user_id=$1 AND asset_type='AVAILABLE_CHIPS'`, user).Scan(&b.AvailableUnits); err != nil {
			return err
		}
		b.Latest, err = s.latestRound(ctx, tx, user, slug)
		if err != nil {
			return err
		}
		if b.Latest != nil && slug == "scratch" && b.Latest.PresentationCompletedAt == nil {
			b.ScratchBlocker = b.Latest
			b.EntryAction = "RESUME"
		}
		if b.Latest != nil && slug == "blackjack" && b.Latest.State == "PLAYER_TURN" {
			b.Active = b.Latest
			b.EntryAction = "RESUME"
		}
		b.ClientSeed, err = preference(ctx, tx, user, slug)
		if err != nil {
			return err
		}
		if runtime.Entry.State == "PLAY" && b.ScratchBlocker == nil && b.Active == nil {
			c, err := s.nextCommitment(ctx, tx, user, slug, runtime, b.ClientSeed)
			if err != nil {
				return err
			}
			b.Next = &c
		}
		return nil
	})
	return b, err
}
func (s *Service) SetClientSeed(ctx context.Context, user int64, slug, seed string) (Commitment, error) {
	var c Commitment
	if !validClientSeed(seed) {
		return c, ErrInvalidInput
	}
	err := s.store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := lockUserGame(ctx, tx, user, slug); err != nil {
			return err
		}
		runtime, err := resolveRuntime(ctx, tx, slug, true)
		if err != nil {
			return err
		}
		if runtime.Entry.Config == nil {
			return ErrUnavailable
		}
		p, err := preference(ctx, tx, user, slug)
		if err != nil {
			return err
		}
		if p.Version == math.MaxInt64 {
			return platform.ErrBalanceOverflow
		}
		// A retry of the same seed is a no-op, preserving the reserved commitment.
		if p.Seed != seed {
			p.Seed = seed
			p.Version++
			if _, err = tx.Exec(ctx, `UPDATE games.client_seed_preferences SET client_seed=$3,version=$4 WHERE newapi_user_id=$1 AND game_slug=$2`, user, slug, p.Seed, p.Version); err != nil {
				return err
			}
		}
		c, err = s.nextCommitment(ctx, tx, user, slug, runtime, p)
		return err
	})
	return c, err
}
func fairnessInput(slug string, c Commitment) (FairRound, error) {
	id, err := uuidBytes(c.ReservedRoundID)
	if err != nil {
		return FairRound{}, err
	}
	hash, err := hex.DecodeString(c.ServerSeedHash)
	if err != nil || len(hash) != 32 {
		return FairRound{}, ErrInvalidInput
	}
	config, err := hex.DecodeString(c.ConfigHash)
	if err != nil || len(config) != 32 {
		return FairRound{}, ErrInvalidInput
	}
	return FairRound{ID: id, Game: slug, ClientSeed: c.ClientSeed, Nonce: c.Nonce, StreamVersion: c.Stream, AlgorithmVersion: c.Algorithm, ConfigVersion: c.ConfigVersion, ConfigHash: [32]byte(config), ServerSeedHash: [32]byte(hash)}, nil
}
