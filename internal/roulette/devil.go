package roulette

// Rules ported from ESSEX-CV9/The-Parliament-Bot, AGPL-3.0, commit
// 6bfbda58b19e045062f4f5939bb9223d810eddb3, devilRouletteEngine.js.
// PVP always alternates cylinder initiative. Surrender is full-stake forfeiture;
// stolen items must still exist and be explicitly selected (no smart fallback).
import (
	"fmt"
	"io"
	"maps"
	"slices"
)

type DevilState struct {
	HP        [2]int          `json:"hp"`
	Items     [2][]string     `json:"items"`
	Known     [2]map[int]bool `json:"known"`
	Shells    []bool          `json:"shells"`
	Pointer   int             `json:"pointer"`
	Turn      int             `json:"turn"`
	First     int             `json:"first"`
	Saw       bool            `json:"saw"`
	Cuffed    [2]bool         `json:"cuffed"`
	Obscured  bool            `json:"obscured"`
	Ended     bool            `json:"ended"`
	Eligible  []int           `json:"eligible"`
	Forfeited [2]bool         `json:"forfeited"`
	Event     string          `json:"event"`
}

var itemOrder = []string{"handcuffs", "adrenaline", "saw", "magnifier", "inverter", "phone", "beer", "cigarette", "medicine"}
var itemWeights = []int{2, 2, 2, 3, 4, 4, 4, 5, 5}

func validItem(item string) bool { return slices.Contains(itemOrder, item) }
func (s DevilState) clone() DevilState {
	s.Shells = slices.Clone(s.Shells)
	s.Eligible = slices.Clone(s.Eligible)
	for i := range 2 {
		s.Items[i] = slices.Clone(s.Items[i])
		s.Known[i] = maps.Clone(s.Known[i])
	}
	return s
}
func newDevil(r io.Reader) (DevilState, error) {
	s := DevilState{HP: [2]int{4, 4}}
	if e := s.reload(r, false); e != nil {
		return s, e
	}
	turn, e := randomInt(r, 2)
	s.Turn = turn
	s.First = turn
	s.Event = "双方各 4 点生命，对局开始。"
	return s, e
}
func (s *DevilState) reload(r io.Reader, alternate bool) error {
	n, e := randomInt(r, 4)
	if e != nil {
		return e
	}
	n += 5
	lo, hi := (4*n+5)/10, (6*n+5)/10
	live, e := randomInt(r, hi-lo+1)
	if e != nil {
		return e
	}
	live += lo
	s.Shells = make([]bool, n)
	for i := 0; i < live; i++ {
		s.Shells[i] = true
	}
	if e = shuffle(r, s.Shells); e != nil {
		return e
	}
	s.Pointer = 0
	s.Obscured = false
	s.Known = [2]map[int]bool{{}, {}}
	count, e := randomInt(r, 2)
	if e != nil {
		return e
	}
	count += 2
	for seat := range 2 {
		for range count {
			if len(s.Items[seat]) == 4 {
				break
			}
			total := 0
			for i, item := range itemOrder {
				if s.acquire(seat, item) {
					total += itemWeights[i]
				}
			}
			if total == 0 {
				break
			}
			draw, e := randomInt(r, total)
			if e != nil {
				return e
			}
			for i, item := range itemOrder {
				if s.acquire(seat, item) {
					draw -= itemWeights[i]
					if draw < 0 {
						s.Items[seat] = append(s.Items[seat], item)
						break
					}
				}
			}
		}
	}
	if alternate {
		s.Cuffed = [2]bool{}
		s.First = 1 - s.First
		s.Turn = s.First
	}
	return nil
}
func (s DevilState) acquire(seat int, item string) bool {
	max := 2
	switch item {
	case "saw", "handcuffs", "adrenaline", "inverter", "cigarette", "medicine":
		max = 1
	}
	count := 0
	for _, v := range s.Items[seat] {
		if v == item {
			count++
		}
	}
	if count >= max {
		return false
	}
	for _, group := range [][]string{{"cigarette", "medicine"}, {"saw", "handcuffs", "adrenaline"}} {
		if slices.Contains(group, item) {
			for _, v := range s.Items[seat] {
				if slices.Contains(group, v) {
					return false
				}
			}
		}
	}
	return true
}
func (s DevilState) itemLegal(seat int, item string) bool {
	if !validItem(item) || item == "adrenaline" {
		return false
	}
	return !(item == "phone" && s.Pointer+1 >= len(s.Shells) || item == "handcuffs" && (s.Pointer+1 >= len(s.Shells) || s.Cuffed[1-seat]))
}
func (s DevilState) actions(seat int) []Action {
	out := []Action{}
	if s.Ended || seat < 0 || seat > 1 {
		return out
	}
	out = append(out, Action{Kind: "SURRENDER"})
	if seat != s.Turn {
		return out
	}
	out = append(out, Action{Kind: "DEVIL_SHOOT", Target: "SELF"}, Action{Kind: "DEVIL_SHOOT", Target: "OPPONENT"})
	seen := map[string]bool{}
	for _, item := range s.Items[seat] {
		if seen[item] {
			continue
		}
		seen[item] = true
		if item == "adrenaline" {
			stolen := map[string]bool{}
			for _, other := range s.Items[1-seat] {
				if !stolen[other] && validItem(other) && other != "adrenaline" {
					out = append(out, Action{Kind: "DEVIL_ITEM", Item: item, StolenItem: other})
					stolen[other] = true
				}
			}
		} else if s.itemLegal(seat, item) {
			out = append(out, Action{Kind: "DEVIL_ITEM", Item: item})
		}
	}
	return out
}
func (s *DevilState) turn(next int) {
	for range 2 {
		if !s.Cuffed[next] {
			break
		}
		s.Cuffed[next] = false
		next = 1 - next
	}
	s.Turn = next
}
func (s *DevilState) consume() {
	for i := range 2 {
		delete(s.Known[i], s.Pointer)
	}
	s.Pointer++
	s.Obscured = false
}
func (s *DevilState) finish() {
	for i, hp := range s.HP {
		if hp <= 0 || s.Forfeited[i] {
			s.Ended = true
			s.Eligible = []int{1 - i}
		}
	}
}
func removeItem(items []string, item string) []string {
	index := slices.Index(items, item)
	return slices.Delete(items, index, index+1)
}
func applyDevil(original DevilState, seat int, a Action, r io.Reader) (DevilState, error) {
	if !ValidAction(a, true) || seat < 0 || seat > 1 || original.Ended {
		return original, ErrActionInvalid
	}
	if a.Kind == "TIMEOUT" {
		a = Action{Kind: "DEVIL_SHOOT", Target: "OPPONENT"}
	}
	if a.Kind != "GAME_LIMIT" && !slices.ContainsFunc(original.actions(seat), func(v Action) bool { return v == a }) {
		return original, ErrActionInvalid
	}
	s := original.clone()
	s.Event = ""
	switch a.Kind {
	case "GAME_LIMIT":
		s.Ended = true
		s.Eligible = []int{0, 1}
		s.Event = "对局达到时间上限，双方平分奖池。"
	case "SURRENDER":
		s.Forfeited[seat] = true
		s.finish()
		s.Event = "玩家认输，保留完整固定投入。"
	case "DEVIL_SHOOT":
		hit := s.Shells[s.Pointer]
		target := 1 - seat
		if a.Target == "SELF" {
			target = seat
		}
		damage := 1
		if s.Saw {
			damage = 2
		}
		s.Saw = false
		s.consume()
		if hit {
			s.HP[target] = max(0, s.HP[target]-damage)
			s.Event = fmt.Sprintf("座位 %d 命中座位 %d，失去 %d 点生命。", seat+1, target+1, damage)
		} else {
			s.Event = fmt.Sprintf("座位 %d 向座位 %d 射出空弹。", seat+1, target+1)
		}
		s.finish()
		if !s.Ended {
			if s.Pointer == len(s.Shells) {
				if e := s.reload(r, true); e != nil {
					return original, e
				}
				s.Event += " 弹仓重装，周期先手交替。"
			} else {
				next := 1 - seat
				if !hit && a.Target == "SELF" {
					next = seat
				}
				s.turn(next)
			}
		}
	case "DEVIL_ITEM":
		s.Items[seat] = removeItem(s.Items[seat], a.Item)
		item := a.Item
		if item == "adrenaline" {
			item = a.StolenItem
			s.Items[1-seat] = removeItem(s.Items[1-seat], item)
		}
		s.Event = fmt.Sprintf("座位 %d 使用 %s。", seat+1, item)
		switch item {
		case "magnifier":
			s.Known[seat][s.Pointer] = s.Shells[s.Pointer]
		case "phone":
			future, unknown := []int{}, []int{}
			for i := s.Pointer + 1; i < len(s.Shells); i++ {
				future = append(future, i)
				if _, ok := s.Known[seat][i]; !ok {
					unknown = append(unknown, i)
				}
			}
			if len(unknown) > 0 {
				future = unknown
			}
			if len(future) == 0 {
				break // A stolen last-shell phone is consumed without revealing a future shell.
			}
			i, e := randomInt(r, len(future))
			if e != nil {
				return original, e
			}
			index := future[i]
			s.Known[seat][index] = s.Shells[index]
		case "saw":
			s.Saw = true
		case "handcuffs":
			s.Cuffed[1-seat] = true
		case "cigarette":
			s.HP[seat] = min(4, s.HP[seat]+1)
		case "medicine":
			v, e := randomInt(r, 5)
			if e != nil {
				return original, e
			}
			if v < 2 {
				s.HP[seat] = min(4, s.HP[seat]+2)
			} else {
				s.HP[seat]--
			}
			s.finish()
		case "inverter":
			s.Shells[s.Pointer] = !s.Shells[s.Pointer]
			s.Obscured = true
			for i := range 2 {
				if v, ok := s.Known[i][s.Pointer]; ok {
					s.Known[i][s.Pointer] = !v
				}
			}
		case "beer":
			live := s.Shells[s.Pointer]
			s.consume()
			if live {
				s.Event += " 弹出实弹。"
			} else {
				s.Event += " 弹出空弹。"
			}
			if s.Pointer == len(s.Shells) {
				if e := s.reload(r, true); e != nil {
					return original, e
				}
				s.Event += " 弹仓重装，周期先手交替。"
			}
		}
	default:
		return original, ErrActionInvalid
	}
	return s, nil
}
func (s DevilState) public() DevilView {
	v := DevilView{Remaining: len(s.Shells) - s.Pointer, Saw: s.Saw, Cuffed: s.Cuffed}
	if !s.Obscured {
		live := 0
		for _, b := range s.Shells[s.Pointer:] {
			if b {
				live++
			}
		}
		blank := v.Remaining - live
		v.Live = &live
		v.Blank = &blank
	}
	return v
}
