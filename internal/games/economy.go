package games

import (
	"context"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func (s *Service) observeEconomy(ctx context.Context, tx pgx.Tx, p platform.EconomicPolicy, user, stake int64) (platform.UnifiedAssets, error) {
	if p.Version == "" {
		return platform.UnifiedAssets{}, nil
	}
	if stake > p.SinglePlayerMaxUnits {
		return platform.UnifiedAssets{}, ErrInvalidInput
	}
	return platform.ReadUnifiedAssetsInTx(ctx, tx, s.observer, user)
}
func capRound(ctx context.Context, tx pgx.Tx, p platform.EconomicPolicy, assets platform.UnifiedAssets, user int64, r *GameRound) error {
	if p.Version == "" {
		return nil
	}
	c, e := platform.CapPayout(p, assets.TotalUnits, r.StakeUnits, r.PayoutUnits, 1)
	if e != nil {
		return e
	}
	c.AssetsObservedAt = assets.ObservedAt
	if e = platform.RecordCapSettlementInTx(ctx, tx, "DIRECT_PLAY_ROUND", r.ID, user, c); e != nil {
		return e
	}
	v := c.PublicView()
	r.EconomySettlement = &v
	r.capWithheld = c.WithheldUnits
	return nil
}
