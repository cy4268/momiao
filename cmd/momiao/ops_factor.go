package main

import (
	"context"
	"github.com/cy4268/momiao/internal/bffauth"
	"github.com/cy4268/momiao/internal/session"
)

type opsFactorAdapter struct{ auth *bffauth.Service }

func (a opsFactorAdapter) Password(ctx context.Context, current session.RequestSession, token, password string) (opsFreshFactorResult, error) {
	value, err := a.auth.OpsPassword(ctx, current, token, password)
	return opsFreshFactorResult{Proof: value.Proof, Require2FA: value.Require2FA, FlowToken: value.FlowToken}, err
}
func (a opsFactorAdapter) TwoFactor(ctx context.Context, current session.RequestSession, token, code string) (opsFreshFactorResult, error) {
	value, err := a.auth.OpsTwoFactor(ctx, current, token, code)
	return opsFreshFactorResult{Proof: value.Proof, Require2FA: value.Require2FA, FlowToken: value.FlowToken}, err
}
func (a opsFactorAdapter) Cancel(ctx context.Context, current session.RequestSession, operationID string) error {
	return a.auth.CancelOpsDiscord(ctx, current, operationID)
}
func (a opsFactorAdapter) DiscordStart(ctx context.Context, current session.RequestSession, operationID, challengeID, challengeToken string) (string, error) {
	value, err := a.auth.OpsDiscordStart(ctx, current, operationID, challengeID, challengeToken)
	return value.AuthorizationURL, err
}
func (a opsFactorAdapter) DiscordCallback(ctx context.Context, current session.RequestSession, operationID, challengeID string, input opsDiscordCallbackInput) (opsFreshFactorResult, error) {
	value, err := a.auth.OpsDiscordCallback(ctx, current, operationID, challengeID, bffauth.DiscordCallbackInput{Code: input.Code, Error: input.Error, ErrorDescription: input.ErrorDescription, State: input.State})
	return opsFreshFactorResult{Proof: value.Proof, Require2FA: value.Require2FA, FlowToken: value.FlowToken}, err
}
func (a opsFactorAdapter) DiscordTwoFactor(ctx context.Context, current session.RequestSession, operationID, challengeID, flowToken, code string) (opsFreshFactorResult, error) {
	value, err := a.auth.OpsDiscordTwoFactor(ctx, current, operationID, challengeID, flowToken, code)
	return opsFreshFactorResult{Proof: value.Proof, Require2FA: value.Require2FA, FlowToken: value.FlowToken}, err
}
