package games

import "github.com/cy4268/momiao/internal/games/fairness"

func testSeed() []byte {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	return seed
}

func vectorRound() FairRound {
	commitment, _ := fairness.SeedCommitment(testSeed())
	return FairRound{
		ID:   [16]byte{0x01, 0x8f, 0x47, 0xa2, 0x6e, 0x9d, 0x7c, 0x31, 0x8a, 0x4b, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		Game: "dice", ClientSeed: "client-seed-demo", Nonce: 42,
		StreamVersion: fairness.StreamVersion, AlgorithmVersion: "dice-map-v1",
		ConfigVersion: "fixture-v1", ConfigHash: [32]byte{1}, ServerSeedHash: commitment,
	}
}
