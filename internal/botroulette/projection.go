package botroulette

import "github.com/cy4268/momiao/internal/roulette"

// These are positive whitelists, not aliases or serialization of RoomView.
// Additions to domain structs cannot silently become part of the Bot contract.
func projectBinding(b roulette.Binding) Binding {
	return Binding{ConfigID: b.ConfigID, ConfigHash: b.ConfigHash, PolicyID: b.PolicyID, PolicyHash: b.PolicyHash, Ruleset: b.Ruleset, Algorithm: b.Algorithm, Stream: b.Stream}
}
func projectPlayers(in []roulette.PlayerView) []PlayerView {
	out := make([]PlayerView, 0, len(in))
	for _, p := range in {
		out = append(out, PlayerView{Seat: p.Seat, Name: p.Name, Ready: p.Ready, Alive: p.Alive, Forfeited: p.Forfeited, HP: p.HP, ItemCount: p.ItemCount})
	}
	return out
}
func projectPublic(v roulette.PublicRoomView) PublicRoom {
	out := PublicRoom{ID: v.ID, Game: v.Game, Title: v.Title, Version: v.Version, Sequence: v.Sequence, State: v.State, TargetPlayers: v.TargetPlayers, StakeUnits: v.StakeUnits, PoolUnits: v.PoolUnits, Binding: projectBinding(v.Binding), ServerSeedHash: v.ServerSeedHash, TurnSeat: v.TurnSeat, ServerNow: v.ServerNow, Deadline: v.Deadline, GameDeadline: v.GameDeadline, Players: projectPlayers(v.Players), Log: []PublicEvent{}}
	for _, e := range v.Log[max(0, len(v.Log)-20):] {
		out.Log = append(out.Log, PublicEvent{Sequence: e.Sequence, At: e.At, Seat: e.Seat, Kind: e.Kind, Text: e.Text})
	}
	if d := v.Devil; d != nil {
		out.Devil = &DevilView{Remaining: d.Remaining, Live: d.Live, Blank: d.Blank, Saw: d.Saw, Cuffed: d.Cuffed}
	}
	if p := v.Pressure; p != nil {
		out.Pressure = &PressureView{ActualLoad: p.ActualLoad, ForcedShots: p.ForcedShots, Order: append([]int{}, p.Order...), Pointer: p.Pointer, UnloadSkipsShot: p.UnloadSkipsShot, TimeoutTier: p.TimeoutTier, Phase: p.Phase, Chambers: p.Chambers, PoolRemaining: p.PoolRemaining, Duds: p.Duds, Charge: p.Charge, Loaded: p.Loaded, Forced: p.Forced, Aggressor: p.Aggressor, RiposteTarget: p.RiposteTarget, Votes: map[int]bool{}}
		for seat, agree := range p.Votes {
			out.Pressure.Votes[seat] = agree
		}
	}
	return out
}
func projectLobby(v roulette.Lobby) LobbyReply {
	out := LobbyReply{SchemaVersion: "1", Game: v.Game, State: v.State, Binding: projectBinding(v.Binding), MinimumUnits: v.MinimumUnits, StepUnits: v.StepUnits, AvailableUnits: v.AvailableUnits, OwnRoundID: v.OwnRoundID, Rooms: []LobbyRoom{}, NextCursor: v.NextCursor}
	for _, r := range v.Rooms {
		out.Rooms = append(out.Rooms, LobbyRoom{ID: r.ID, Game: r.Game, Title: r.Title, Version: r.Version, State: r.State, TargetPlayers: r.TargetPlayers, StakeUnits: r.StakeUnits, PoolUnits: r.PoolUnits, Players: projectPlayers(r.Players)})
	}
	return out
}
func projectAction(a roulette.Action) Action {
	return Action{Kind: a.Kind, Target: a.Target, Item: a.Item, StolenItem: a.StolenItem, Agree: a.Agree}
}
func domainAction(a Action) roulette.Action {
	return roulette.Action{Kind: a.Kind, Target: a.Target, Item: a.Item, StolenItem: a.StolenItem, Agree: a.Agree}
}
func projectState(v roulette.RoomView, own *string) StateReply {
	public := roulette.PublicRoomView{ID: v.ID, Game: v.Game, Title: v.Title, Version: v.Version, Sequence: v.Sequence, State: v.State, TargetPlayers: v.TargetPlayers, StakeUnits: v.StakeUnits, PoolUnits: v.PoolUnits, Binding: v.Binding, ServerSeedHash: v.ServerSeedHash, TurnSeat: v.TurnSeat, ServerNow: v.ServerNow, Deadline: v.Deadline, GameDeadline: v.GameDeadline, Players: v.Players, Log: v.Log, Devil: v.Devil, Pressure: v.Pressure}
	out := StateReply{SchemaVersion: "1", Room: projectPublic(public), OwnRoundID: own, Actions: []Action{}}
	for _, a := range v.Actions {
		out.Actions = append(out.Actions, projectAction(a))
	}
	if s := v.Self; s != nil {
		out.Self = &SelfView{Seat: s.Seat, AvailableUnits: s.AvailableUnits, Items: append([]string{}, s.Items...), Intel: []IntelEntry{}}
		for _, i := range s.Intel {
			out.Self.Intel = append(out.Self.Intel, IntelEntry{Index: i.Index, Live: i.Live})
		}
	}
	if p := v.EconomicPolicy; p != nil {
		out.EconomicPolicy = &EconomicPolicy{Version: p.Version, Hash: p.Hash, SinglePlayerMaxUnits: p.SinglePlayerMaxUnits, AssetCapUnits: p.AssetCapUnits, CapMode: p.CapMode}
	}
	if p := v.EconomySettlement; p != nil {
		out.EconomySettlement = &PayoutCapView{PolicyVersion: p.PolicyVersion, PolicyHash: p.PolicyHash, GrossPayoutUnits: p.GrossPayoutUnits, CreditedPayoutUnits: p.CreditedPayoutUnits, WithheldUnits: p.WithheldUnits, ActualNetUnits: p.ActualNetUnits, NeutralReturnUnits: p.NeutralReturnUnits}
	}
	return out
}
func receiptReply(requestID, status string, r *roulette.Receipt) ReceiptReply {
	out := ReceiptReply{SchemaVersion: "1", Status: status, RequestID: requestID}
	if r != nil {
		out.Receipt = &Receipt{RoundID: r.RoundID, Version: r.Version, Sequence: r.Sequence, State: r.State}
	}
	return out
}
