package poker

import (
	"strconv"
	"time"

	"github.com/cy4268/momiao/internal/poker/engine"
)

const liveTimelineLimit = 128

type TimelineView struct {
	HandID       string              `json:"hand_id,omitempty"`
	LastSequence string              `json:"last_sequence"`
	Truncated    bool                `json:"truncated"`
	Events       []TimelineEventView `json:"events"`
}

type TimelineEventView struct {
	Sequence   string    `json:"sequence"`
	HandVersion string    `json:"hand_version"`
	Type       string    `json:"type"`
	Street     string    `json:"street"`
	SeatNo     int       `json:"seat_no,omitempty"`
	DeltaUnits string    `json:"delta_units,omitempty"`
	ToUnits    string    `json:"to_units,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// publicTimelineType is an allowlist. Adding an engine event does not make it
// public until it is explicitly reviewed here; private deal metadata, action
// IDs, cards and visibility markers have no projection fields at all.
func publicTimelineType(kind string) bool {
	switch kind {
	case "HAND_COMMITTED", "POST_SB", "POST_BB", "FOLD", "CHECK", "CALL", "BET", "RAISE", "ALL_IN",
		"AUTO_FOLD", "AUTO_CHECK", "DISCONNECTED", "RECONNECTED", "RECOVERING", "RESUMED", "ACTOR_CHANGED",
		"ALL_IN_RUNOUT", "DEAL_FLOP", "DEAL_TURN", "DEAL_RIVER", "RETURN_UNCALLED", "SHOWDOWN", "POT_AWARD", "SYSTEM_SETTLEMENT":
		return true
	default:
		return false
	}
}

func liveTimeline(handID string, state engine.State) TimelineView {
	view := TimelineView{HandID: handID, LastSequence: "0", Events: []TimelineEventView{}}
	for _, event := range state.Events {
		if !publicTimelineType(event.Type) {
			continue
		}
		item := TimelineEventView{
			Sequence: strconv.FormatUint(event.Sequence, 10), HandVersion: strconv.FormatUint(event.Version, 10),
			Type: event.Type, Street: string(event.Street), SeatNo: event.Seat, OccurredAt: event.At,
		}
		if event.Delta != 0 {
			item.DeltaUnits = decimal(event.Delta)
		}
		if event.To != 0 {
			item.ToUnits = decimal(event.To)
		}
		view.Events = append(view.Events, item)
		view.LastSequence = item.Sequence
	}
	if len(view.Events) > liveTimelineLimit {
		view.Truncated = true
		view.Events = append([]TimelineEventView{}, view.Events[len(view.Events)-liveTimelineLimit:]...)
	}
	return view
}
