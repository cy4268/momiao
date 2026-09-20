package roulette

// Rules adapted from The-Parliament-Bot pressureRouletteGame.js, AGPL-3.0,
// commit 6bfbda58b19e045062f4f5939bb9223d810eddb3. Discord penalties are not
// part of this native fixed-stake game. See docs/roulette.md for RNG ordering.
import (
	"fmt"
	"io"
	"maps"
	"slices"
)

const pressureEmpty = 0
const pressureLive = 1
const pressureDud = 2

type PressureState struct {
	Alive        []int        `json:"alive"`
	Turn         int          `json:"turn"`
	Phase        string       `json:"phase"`
	Pool         []int        `json:"pool"`
	PoolDuds     int          `json:"pool_duds"`
	Wave         int          `json:"wave"`
	Chambers     [6]int       `json:"chambers"`
	Revealed     [6]string    `json:"revealed"`
	Pointer      int          `json:"pointer"`
	Bullets      int          `json:"bullets"`
	GunDuds      int          `json:"gun_duds"`
	Charge       int          `json:"charge"`
	DebtOwner    *int         `json:"debt_owner"`
	Debt         int          `json:"debt"`
	Aggressor    *int         `json:"aggressor"`
	Holder       *int         `json:"holder"`
	RipInitiator *int         `json:"rip_initiator"`
	RipTarget    *int         `json:"rip_target"`
	UnloadUsed   [6]bool      `json:"unload_used"`
	TimeoutTiers [6]int       `json:"timeout_tiers"`
	Forfeited    [6]bool      `json:"forfeited"`
	Votes        map[int]bool `json:"votes"`
	ResumeMode   string       `json:"resume_mode"`
	Ended        bool         `json:"ended"`
	Eligible     []int        `json:"eligible"`
	Reason       string       `json:"reason"`
	Event        string       `json:"event"`
}

func pressureSeat(v int) *int { return &v }
func (s PressureState) clone() PressureState {
	s.Alive = slices.Clone(s.Alive)
	s.Pool = slices.Clone(s.Pool)
	s.Eligible = slices.Clone(s.Eligible)
	s.Votes = maps.Clone(s.Votes)
	return s
}
func newPressure(r io.Reader, seats []int) (PressureState, error) {
	s := PressureState{Alive: slices.Clone(seats), Phase: "FIRE", Votes: map[int]bool{}}
	if len(seats) < 3 || len(seats) > 6 {
		return s, ErrInvalidInput
	}
	seen := map[int]bool{}
	for _, v := range seats {
		if v < 0 || v > 5 || seen[v] {
			return s, ErrInvalidInput
		}
		seen[v] = true
	}
	if e := shuffle(r, s.Alive); e != nil {
		return s, e
	}
	s.Turn = s.Alive[0]
	if e := s.preparePool(r); e != nil {
		return s, e
	}
	s.draw(1)
	return s, s.spin(r)
}
func (s *PressureState) preparePool(r io.Reader) error {
	n, e := randomInt(r, 3)
	if e != nil {
		return e
	}
	s.PoolDuds = n + 1
	s.Pool = make([]int, 9)
	for i := range s.Pool {
		s.Pool[i] = pressureLive
		if i < s.PoolDuds {
			s.Pool[i] = pressureDud
		}
	}
	s.Wave++
	return shuffle(r, s.Pool)
}
func (s *PressureState) draw(n int) (total, duds int) {
	for range min(n, len(s.Pool)) {
		v := s.Pool[0]
		s.Pool = s.Pool[1:]
		total++
		if v == pressureDud {
			duds++
		}
	}
	s.Bullets += total
	s.GunDuds += duds
	return
}
func (s *PressureState) spin(r io.Reader) error {
	positions := []int{0, 1, 2, 3, 4, 5}
	if e := shuffle(r, positions); e != nil {
		return e
	}
	s.Chambers = [6]int{}
	s.Revealed = [6]string{"UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN"}
	s.Pointer = 0
	for i := 0; i < s.Bullets; i++ {
		v := pressureLive
		if i < s.GunDuds {
			v = pressureDud
		}
		s.Chambers[positions[i]] = v
	}
	return nil
}
func (s *PressureState) reload(r io.Reader) error {
	positions := []int{}
	for i, v := range s.Revealed {
		if v == "UNKNOWN" {
			positions = append(positions, i)
		}
	}
	if len(positions) == 0 {
		s.draw(1)
		return s.spin(r)
	}
	n, duds := s.draw(1)
	if e := shuffle(r, positions); e != nil {
		return e
	}
	for i := 0; i < n; i++ {
		v := pressureLive
		if i < duds {
			v = pressureDud
		}
		s.Chambers[positions[i]] = v
	}
	return nil
}
func (s PressureState) next(seat int) int {
	i := slices.Index(s.Alive, seat)
	return s.Alive[(i+1)%len(s.Alive)]
}
func (s PressureState) forced() int {
	if s.Charge >= 2 {
		return 2
	}
	return 1
}
func (s PressureState) loadCount() int { return min(1+s.Charge, 6-s.Bullets, len(s.Pool)) }
func (s PressureState) debt(seat int) int {
	if s.DebtOwner != nil && *s.DebtOwner == seat {
		return s.Debt
	}
	return 0
}
func (s *PressureState) clearDebt(seat int) {
	if s.DebtOwner != nil && *s.DebtOwner == seat {
		s.Debt = 0
		s.DebtOwner = nil
	}
}
func (s *PressureState) finish(reason string) {
	s.Ended = true
	s.Phase = "ENDED"
	s.Eligible = slices.Clone(s.Alive)
	s.Reason = reason
}
func (s *PressureState) remove(seat int) {
	s.Alive = slices.DeleteFunc(s.Alive, func(v int) bool { return v == seat })
	s.clearDebt(seat)
	if s.Holder != nil && *s.Holder == seat || s.Aggressor != nil && *s.Aggressor == seat {
		s.Holder = nil
		s.Aggressor = nil
	}
	if s.RipTarget != nil && *s.RipTarget == seat || s.RipInitiator != nil && *s.RipInitiator == seat {
		s.RipTarget = nil
		s.RipInitiator = nil
	}
	if len(s.Alive) <= 1 {
		s.finish("LAST_SURVIVOR")
	}
}
func (s *PressureState) pass() {
	seat := s.Turn
	s.Turn = s.next(seat)
	s.Charge = 0
	s.Phase = "FIRE"
	if s.Holder != nil && *s.Holder == seat {
		s.Holder = nil
		s.Aggressor = nil
	}
}

// Empty-gun resolution precedes the pending choice/forced pass, including unload.
func (s *PressureState) resume(r io.Reader, mode string) error {
	if s.Ended {
		return nil
	}
	if s.Bullets == 0 {
		if len(s.Pool) == 0 {
			s.Phase = "VOTE"
			s.Votes = map[int]bool{}
			s.ResumeMode = mode
			s.Event += " 弹池耗尽，进入和局投票。"
			return nil
		}
		if e := s.reload(r); e != nil {
			return e
		}
		s.Event += " 自动补入 1 发，保留已公开弹巢。"
	}
	if mode == "forcedPass" {
		s.pass()
	} else {
		s.Phase = mode
	}
	return nil
}
func (s PressureState) actions(seat int) []Action {
	out := []Action{}
	if s.Ended || !slices.Contains(s.Alive, seat) {
		return out
	}
	if s.Phase == "VOTE" {
		if _, ok := s.Votes[seat]; !ok {
			yes, no := true, false
			out = append(out, Action{Kind: "PRESSURE_VOTE", Agree: &yes}, Action{Kind: "PRESSURE_VOTE", Agree: &no})
		}
		return out
	}
	if seat != s.Turn {
		return out
	}
	if s.Phase == "FIRE" {
		out = append(out, Action{Kind: "PRESSURE_FIRE"})
		if s.RipTarget == nil {
			out = append(out, Action{Kind: "SURRENDER"})
			if !s.UnloadUsed[seat] {
				out = append(out, Action{Kind: "PRESSURE_UNLOAD"})
			}
		}
		return out
	}
	if s.Phase == "CHOICE" && s.debt(seat) == 0 {
		out = append(out, Action{Kind: "PRESSURE_PASS"}, Action{Kind: "PRESSURE_AGAIN"})
		if s.loadCount() > 0 {
			out = append(out, Action{Kind: "PRESSURE_CHARGE"})
		}
		if s.RipTarget == nil && s.Holder != nil && *s.Holder == seat && s.Aggressor != nil && slices.Contains(s.Alive, *s.Aggressor) {
			out = append(out, Action{Kind: "PRESSURE_RIPOSTE"})
		}
	}
	return out
}
func sameAction(a, b Action) bool {
	return a.Kind == b.Kind && a.Target == b.Target && a.Item == b.Item && a.StolenItem == b.StolenItem && (a.Agree == nil && b.Agree == nil || a.Agree != nil && b.Agree != nil && *a.Agree == *b.Agree)
}
func (s *PressureState) shot(r io.Reader, unload bool) error {
	seat := s.Turn
	index := s.Pointer
	bullet := s.Chambers[index]
	hit := bullet == pressureLive
	s.Revealed[index] = "EMPTY"
	s.Chambers[index] = pressureEmpty
	s.Pointer = (index + 1) % 6
	if bullet != pressureEmpty {
		s.Bullets--
		if bullet == pressureDud {
			s.GunDuds--
			s.Revealed[index] = "DUD_SPENT"
		} else {
			s.Revealed[index] = "LIVE_SPENT"
			s.Charge = 0
		}
	}
	names := []string{"空膛", "实弹 · 淘汰", "哑弹"}
	s.Event += fmt.Sprintf("座位 %d 击发第 %d 格：%s。", seat+1, index+1, names[bullet])
	mode := "CHOICE"
	if s.RipTarget != nil {
		next := s.next(*s.RipInitiator)
		if hit && next == seat {
			next = s.next(seat)
		}
		s.RipTarget = nil
		s.RipInitiator = nil
		s.Turn = next
		if hit {
			s.remove(seat)
		}
		if hit || next != seat {
			mode = "FIRE"
		}
	} else {
		if s.debt(seat) > 0 {
			s.Debt--
			if s.Debt == 0 {
				s.DebtOwner = nil
			}
		}
		if hit {
			next := s.next(seat)
			s.remove(seat)
			s.Turn = next
			mode = "FIRE"
		} else if s.debt(seat) > 0 {
			mode = "FIRE"
		} else if unload {
			mode = "forcedPass"
		}
	}
	return s.resume(r, mode)
}
func applyPressure(original PressureState, seat int, a Action, r io.Reader) (PressureState, error) {
	if !ValidAction(a, true) || original.Ended || seat < 0 || seat > 5 {
		return original, ErrActionInvalid
	}
	s := original.clone()
	s.Event = ""
	if a.Kind == "GAME_LIMIT" {
		s.finish("GAME_LIMIT")
		s.Event = "对局达到时间上限，存活者平分奖池。"
		return s, nil
	}
	if a.Kind == "TIMEOUT" {
		if seat != s.Turn {
			return original, ErrActionInvalid
		}
		if s.Phase == "VOTE" {
			for _, v := range s.Alive {
				if _, ok := s.Votes[v]; !ok {
					s.Votes[v] = true
				}
			}
			s.finish("VOTE_DRAW")
			s.Event = "投票到期，未投票者视为同意；存活者平分奖池。"
			return s, nil
		}
		s.TimeoutTiers[seat] = min(2, s.TimeoutTiers[seat]+1)
		a = Action{Kind: "PRESSURE_FIRE"}
		if s.Phase == "CHOICE" && s.debt(seat) == 0 {
			a.Kind = "PRESSURE_PASS"
		}
		s.Event = "行动超时，系统代行。 "
	}
	if !slices.ContainsFunc(s.actions(seat), func(v Action) bool { return sameAction(v, a) }) {
		return original, ErrActionInvalid
	}
	var e error
	switch a.Kind {
	case "PRESSURE_FIRE":
		e = s.shot(r, false)
	case "PRESSURE_PASS":
		s.pass()
		s.Event += "持枪者传枪，蓄力与反手权清除。"
	case "PRESSURE_AGAIN":
		s.Charge++
		s.Phase = "FIRE"
		s.Event += "持枪者选择再开一枪，蓄力增加。"
	case "PRESSURE_CHARGE":
		n, forced := s.loadCount(), s.forced()
		s.draw(n)
		if e = s.spin(r); e != nil {
			return original, e
		}
		s.pass()
		s.Aggressor = pressureSeat(seat)
		s.Holder = pressureSeat(s.Turn)
		if forced == 2 {
			s.DebtOwner = pressureSeat(s.Turn)
			s.Debt = forced
		} else {
			s.DebtOwner = nil
			s.Debt = 0
		}
		s.Event += fmt.Sprintf("座位 %d 加压装入 %d 发；座位 %d 需开 %d 枪。筹码投入不变。", seat+1, n, s.Turn+1, forced)
	case "PRESSURE_UNLOAD":
		skip := s.debt(seat) == 1
		s.clearDebt(seat)
		if s.Bullets > 0 {
			n, err := randomInt(r, s.Bullets)
			if err != nil {
				return original, err
			}
			s.Bullets--
			if n < s.GunDuds {
				s.GunDuds--
			}
		}
		if e = s.spin(r); e != nil {
			return original, e
		}
		s.UnloadUsed[seat] = true
		s.Charge = 0
		s.Event = "消耗一次退弹，弃掉 1 发并重转弹巢。 "
		if skip {
			s.Event += "抵消剩余强制枪，随后强制传枪。"
			e = s.resume(r, "forcedPass")
		} else {
			e = s.shot(r, true)
		}
	case "PRESSURE_RIPOSTE":
		s.RipInitiator = pressureSeat(seat)
		s.RipTarget = pressureSeat(*s.Aggressor)
		s.Turn = *s.Aggressor
		s.Aggressor = nil
		s.Holder = nil
		s.Charge = 0
		s.Phase = "FIRE"
		s.Event = fmt.Sprintf("座位 %d 反手还击；座位 %d 必须开 1 枪，随后跳过发起者。", seat+1, s.Turn+1)
	case "SURRENDER":
		next := s.next(seat)
		s.Forfeited[seat] = true
		s.Charge = 0
		s.remove(seat)
		s.Turn = next
		if s.Ended {
			s.Reason = "FORFEIT"
		}
		s.Event = fmt.Sprintf("座位 %d 认输，固定投入留在奖池。", seat+1)
		e = s.resume(r, "FIRE")
	case "PRESSURE_VOTE":
		s.Votes[seat] = *a.Agree
		if !*a.Agree {
			if e = s.preparePool(r); e != nil {
				return original, e
			}
			s.draw(1)
			if e = s.spin(r); e != nil {
				return original, e
			}
			mode := s.ResumeMode
			s.ResumeMode = ""
			s.Votes = map[int]bool{}
			s.Event = "有人反对和局，新波次已装填。"
			e = s.resume(r, mode)
		} else {
			s.Event = fmt.Sprintf("座位 %d 同意和局。", seat+1)
			if len(s.Votes) == len(s.Alive) {
				s.finish("VOTE_DRAW")
				s.Event += " 全员同意，存活者平分奖池。"
			}
		}
	default:
		return original, ErrActionInvalid
	}
	if e != nil {
		return original, e
	}
	return s, nil
}
func (s PressureState) public() PressureView {
	return PressureView{Phase: s.Phase, Chambers: s.Revealed, PoolRemaining: len(s.Pool), Duds: s.PoolDuds, Charge: s.Charge, Loaded: s.Bullets, Forced: s.debt(s.Turn), Aggressor: s.Aggressor, RiposteTarget: s.RipTarget, Votes: maps.Clone(s.Votes), ActualLoad: s.loadCount(), ForcedShots: s.forced(), Order: slices.Clone(s.Alive), Pointer: s.Pointer, UnloadSkipsShot: s.debt(s.Turn) == 1, TimeoutTier: s.TimeoutTiers[s.Turn]}
}
