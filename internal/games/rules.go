// Package games implements the first Direct Play games and their durable service.
package games

import (
	"errors"

	"github.com/cy4268/momiao/internal/games/fairness"
)

type FairRound = fairness.Round

var (
	ErrInvalidInput   = fairness.ErrInvalidInput
	ErrInvalidConfig  = errors.New("games: invalid configuration")
	ErrConfigMismatch = errors.New("games: configuration binding mismatch")
)
