package poker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type LobbyFilter struct {
	Query, AccessMode, BlindPreset, LifecycleState, Sort, Cursor string
	OpenSeatsOnly, SpectatorsOnly                                bool
	MaxSeats, Limit                                              int
}
type LobbyRuleset struct {
	Version              string `json:"version"`
	AntePostingMode      string `json:"ante_posting_mode"`
	EntryMode            string `json:"entry_mode"`
	InitialButtonVersion string `json:"initial_button_version"`
	EvaluatorVersion     string `json:"evaluator_version"`
	ShortcutVersion      string `json:"shortcut_version"`
	AlgorithmVersion     string `json:"algorithm_version"`
	DealVersion          string `json:"deal_version"`
}
type LobbyService struct {
	State             string   `json:"state"`
	ProductionReady   bool     `json:"production_ready"`
	Blockers          []string `json:"blockers"`
	MaintenanceScopes []string `json:"maintenance_scopes"`
}
type LobbyViewer struct {
	UserID              string  `json:"user_id"`
	AvailableChipsUnits string  `json:"available_chips_units"`
	WalletVersion       string  `json:"wallet_version"`
	PokerInPlayUnits    string  `json:"poker_in_play_units"`
	ProfileComplete     bool    `json:"profile_complete"`
	OwnedOpenTableID    *string `json:"owned_open_table_id"`
	CanCreate           bool    `json:"can_create"`
	CanJoin             bool    `json:"can_join"`
}
type LobbyActiveSession struct {
	SessionID        string `json:"session_id"`
	TableID          string `json:"table_id"`
	TableName        string `json:"table_name"`
	State            string `json:"state"`
	SeatNo           int    `json:"seat_no"`
	StackUnits       string `json:"stack_units"`
	CommittedUnits   string `json:"committed_units"`
	PokerInPlayUnits string `json:"poker_in_play_units"`
	SmallBlindUnits  string `json:"small_blind_units"`
	BigBlindUnits    string `json:"big_blind_units"`
	AnteUnits        string `json:"ante_units"`
	CanReconnect     bool   `json:"can_reconnect"`
}
type LobbyBlinds struct {
	SmallBlindUnits   string `json:"small_blind_units"`
	BigBlindUnits     string `json:"big_blind_units"`
	AnteUnits         string `json:"ante_units"`
	MinimumBuyInUnits string `json:"minimum_buyin_units"`
	MaximumBuyInUnits string `json:"maximum_buyin_units"`
}
type LobbyPreset struct {
	LobbyBlinds
	ID string `json:"id"`
}
type LobbyTable struct {
	LobbyBlinds
	TableID          string `json:"table_id"`
	TableVersion     string `json:"table_version"`
	Name             string `json:"name"`
	Visibility       string `json:"visibility"`
	BlindPresetID    string `json:"blind_preset_id"`
	LifecycleState   string `json:"lifecycle_state"`
	MaxSeats         int    `json:"max_seats"`
	OccupiedSeats    int    `json:"occupied_seats"`
	OpenSeatNumbers  []int  `json:"open_seat_numbers"`
	AcceptingPlayers bool   `json:"accepting_players"`
	AllowNewHands    bool   `json:"allow_new_hands"`
	AllowSpectators  bool   `json:"allow_spectators"`
	ChatEnabled      bool   `json:"chat_enabled"`
	CanJoin          bool   `json:"can_join"`
	CanSpectate      bool   `json:"can_spectate"`
	CanRequestAccess bool   `json:"can_request_access"`
	ruleset          string
	needsReview      bool
}
type LobbySnapshot struct {
	ServerNow     time.Time           `json:"server_now"`
	Service       LobbyService        `json:"service"`
	Ruleset       *LobbyRuleset       `json:"ruleset"`
	Viewer        LobbyViewer         `json:"viewer"`
	ActiveSession *LobbyActiveSession `json:"active_session"`
	CreateOptions struct {
		AccessModes      []string `json:"access_modes"`
		ChatConfigurable bool     `json:"chat_configurable"`
	} `json:"create_options"`
	BlindPresets []LobbyPreset `json:"blind_presets"`
	Tables       []LobbyTable  `json:"tables"`
	Page         struct {
		Limit      int     `json:"limit"`
		NextCursor *string `json:"next_cursor"`
	} `json:"page"`
}

func (s *Service) Lobby(ctx context.Context, userID int64, filter LobbyFilter) (LobbySnapshot, error) {
	return s.lobby(ctx, userID, nil, filter)
}

func (s *Service) LobbyForSession(ctx context.Context, auth AuthSession, filter LobbyFilter) (LobbySnapshot, error) {
	if !validAccessAuth(auth) {
		return LobbySnapshot{}, ErrInvalid
	}
	return s.lobby(ctx, auth.UserID, &auth, filter)
}

func (s *Service) lobby(ctx context.Context, userID int64, auth *AuthSession, filter LobbyFilter) (LobbySnapshot, error) {
	var out LobbySnapshot
	f, cursor, err := normalizeLobbyFilter(filter)
	if userID <= 0 || err != nil {
		return out, ErrInvalid
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return out, ErrClosed
	}
	requestCtx := ctx
	ctx, cancel := context.WithTimeout(requestCtx, 3*time.Second)
	defer cancel()
	tx, err := s.opts.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout='2s'"); err != nil {
		return out, err
	}
	facts, err := readAdmissionFacts(ctx, tx, approvedRulesetVersion)
	if err != nil {
		return out, err
	}
	out.ServerNow = facts.now
	if out.Viewer, err = readLobbyIdentity(ctx, tx, userID); err != nil {
		return out, err
	}
	var balance, version int64
	if err = tx.QueryRow(ctx, `SELECT balance_units,version FROM economy.poker_lobby_balance($1)`, userID).Scan(&balance, &version); err != nil {
		return out, err
	}
	out.Viewer.AvailableChipsUnits, out.Viewer.WalletVersion = decimal(balance), decimal(version)
	session, err := readSessionFrom(ctx, tx, userID, "")
	if err != nil {
		return out, err
	}
	if session != nil {
		out.ActiveSession = &LobbyActiveSession{session.SessionID, session.TableID, session.TableName, session.State, session.SeatNo, session.StackUnits, session.CommittedUnits, session.PokerInPlayUnits, decimal(session.smallBlind), decimal(session.bigBlind), "0", true}
		out.Viewer.PokerInPlayUnits = session.PokerInPlayUnits
	}
	if out.BlindPresets, err = readLobbyPresets(ctx, tx); err != nil {
		return out, err
	}
	facts.ready = facts.ready && len(out.BlindPresets) > 0
	if f.BlindPreset != "ALL" {
		found := false
		for _, p := range out.BlindPresets {
			found = found || p.ID == f.BlindPreset
		}
		if !found {
			return out, ErrInvalid
		}
	}
	out.Service = LobbyService{"READY", facts.ready, []string{}, facts.scopes}
	if len(facts.scopes) > 0 {
		out.Service.State, out.Service.Blockers = "MAINTENANCE", []string{ErrMaintenance.Error()}
	}
	if facts.ready {
		out.Ruleset = &facts.ruleset
	} else {
		out.Service.State = "CONFIG_INCOMPLETE"
		out.Service.Blockers = append(out.Service.Blockers, ErrRulesetIncomplete.Error())
	}
	out.Viewer.CanJoin = newWorkEligible(facts, out.Viewer.ProfileComplete, session != nil)
	out.Viewer.CanCreate = out.Viewer.CanJoin && out.Viewer.OwnedOpenTableID == nil
	// Implemented creation capabilities, never substitutes for a table's facts.
	out.CreateOptions.AccessModes = []string{"PUBLIC"}
	if s.passwordEnabled() {
		out.CreateOptions.AccessModes = append(out.CreateOptions.AccessModes, "PASSWORD")
	}
	out.CreateOptions.ChatConfigurable = true
	out.Page.Limit = f.Limit
	if out.Tables, out.Page.NextCursor, err = readLobbyTables(ctx, tx, f, cursor); err != nil {
		return out, err
	}
	var accessUntil time.Time
	accessReady := auth != nil && s.passwordEnabled()
	credentials := make(map[string]string)
	if accessReady {
		for _, table := range out.Tables {
			if table.Visibility != "PASSWORD" {
				continue
			}
			mode, phc, credentialErr := readAccessCredential(ctx, tx, table.TableID)
			if credentialErr != nil {
				return out, credentialErr
			}
			if mode == "PASSWORD" {
				credentials[table.TableID] = phc
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	cancel()
	// Redis may consume its entire budget without cancelling a PG commit.
	accessCtx, stopAccess := context.WithTimeout(requestCtx, 3*time.Second)
	defer stopAccess()
	if accessReady {
		if accessUntil, err = s.passwordDeadline(accessCtx, *auth); err != nil {
			return out, err
		}
	}
	for i := range out.Tables {
		t := &out.Tables[i]
		available := facts.ready && t.ruleset == approvedRulesetVersion && !t.needsReview && t.LifecycleState != "CLOSED" && t.LifecycleState != "CLOSING" && t.LifecycleState != "RECOVERING"
		granted := t.Visibility == "PUBLIC"
		if t.Visibility == "PASSWORD" {
			t.CanRequestAccess = accessReady && t.LifecycleState != "CLOSED" && (session == nil || session.TableID == t.TableID)
			if accessReady {
				if phc := credentials[t.TableID]; phc != "" {
					var grantErr error
					granted, grantErr = s.accessValid(accessCtx, t.TableID, *auth, phc, accessUntil)
					if grantErr != nil {
						granted = false
					}
				}
			}
			if !granted {
				t.OpenSeatNumbers = []int{}
			}
		}
		t.CanJoin = granted && out.Viewer.CanJoin && available && len(t.OpenSeatNumbers) > 0 && tableJoinable(t.LifecycleState, t.Visibility, t.AcceptingPlayers, t.needsReview)
		ownerEntry := out.Viewer.OwnedOpenTableID != nil && *out.Viewer.OwnedOpenTableID == t.TableID
		t.CanSpectate = granted && session == nil && available && (t.AllowSpectators || ownerEntry)
	}
	return out, nil
}
