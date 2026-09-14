// game-config-export emits the exact compiled Slot/Blackjack configuration for
// offline artifact binding. It has no database, credentials or activation path.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cy4268/momiao/internal/games"
	"github.com/cy4268/momiao/internal/games/slot"
)

func main() {
	out := map[string]any{}
	for _, game := range []string{"slot", "blackjack"} {
		version := "01993200-0000-7000-8000-000000000004"
		build := games.SlotV1
		if game == "blackjack" {
			version = "01993200-0000-7000-8000-000000000005"
			build = games.BlackjackV1
		}
		c, err := build(version)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		b := c.Binding()
		payload := c.CanonicalJSON()
		plain := sha256.Sum256(payload)
		out[game] = map[string]any{"config_version": version, "schema": b.SchemaVersion, "ruleset": b.RulesetVersion, "algorithm": b.AlgorithmVersion, "implementation": "direct." + game + ".v1", "canonical_payload": json.RawMessage(payload), "canonical_payload_sha256": hex.EncodeToString(plain[:]), "config_hash": hex.EncodeToString(b.Hash[:])}
	}
	frozen, _ := json.Marshal(slot.FrozenConfig())
	h := sha256.Sum256(frozen)
	out["slot_frozen_resource_sha256"] = hex.EncodeToString(h[:])
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
