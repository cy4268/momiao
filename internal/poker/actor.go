package poker

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"sync"
	"time"
)

type tableMutation func(context.Context, pgx.Tx, *tableRow) (Receipt, error)
type actorRequest struct {
	ctx   context.Context
	run   tableMutation
	reply chan actorReply
}
type actorReply struct {
	receipt Receipt
	err     error
}
type tableActor struct {
	s              *Service
	id             string
	epoch          uint64
	mailbox        chan actorRequest
	stop, finished chan struct{}
	ready          chan struct{}
	initErr        error
	once           sync.Once
}

func (a *tableActor) close() { a.once.Do(func() { close(a.stop) }); <-a.finished }
func (s *Service) publish(id string, version uint64) {
	if s.opts.AfterCommit != nil {
		s.opts.AfterCommit(id, version)
	}
}
func (s *Service) actor(ctx context.Context, id string, replace bool) (*tableActor, error) {
	s.actorMu.Lock()
	defer s.actorMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrClosed
	}
	if a := s.actors[id]; a != nil && !replace {
		s.mu.Unlock()
		return a, nil
	}
	s.mu.Unlock()
	tx, err := s.opts.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	var epoch uint64
	if err = tx.QueryRow(ctx, "UPDATE poker.tables SET runtime_epoch=runtime_epoch+1 WHERE table_id=$1 AND lifecycle_state<>'CLOSED' RETURNING runtime_epoch", id).Scan(&epoch); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrClosed
	}
	if old := s.actors[id]; old != nil {
		old.once.Do(func() { close(old.stop) })
	}
	a := &tableActor{s: s, id: id, epoch: epoch, mailbox: make(chan actorRequest, s.opts.MailboxCapacity), stop: make(chan struct{}), finished: make(chan struct{}), ready: make(chan struct{})}
	s.actors[id] = a
	s.mu.Unlock()
	go a.loop()
	return a, nil
}
func (s *Service) mutate(ctx context.Context, id string, run tableMutation) (Receipt, error) {
	a, err := s.actor(ctx, id, false)
	if err != nil {
		return Receipt{}, err
	}
	select {
	case <-a.ready:
	case <-ctx.Done():
		return Receipt{}, ctx.Err()
	case <-a.stop:
		return Receipt{}, ErrClosed
	}
	if a.initErr != nil {
		return Receipt{}, a.initErr
	}
	q := actorRequest{ctx: ctx, run: run, reply: make(chan actorReply, 1)}
	select {
	case a.mailbox <- q:
	case <-ctx.Done():
		return Receipt{}, ctx.Err()
	case <-a.stop:
		return Receipt{}, ErrClosed
	default:
		return Receipt{}, ErrBusy
	}
	select {
	case r := <-q.reply:
		return r.receipt, r.err
	case <-ctx.Done():
		return Receipt{}, ctx.Err()
	case <-a.stop:
		return Receipt{}, ErrClosed
	}
}
func (a *tableActor) loop() {
	defer close(a.finished)
	defer a.once.Do(func() { close(a.stop) })
	// Hydration happens before any mailbox request or timer is processed.
	ctx, cancel := context.WithTimeout(context.Background(), controlIOTimeout)
	_, a.initErr = a.execute(ctx, a.s.recoverMutation)
	cancel()
	if errors.Is(a.initErr, ErrNeedsReview) {
		a.initErr = nil
	}
	close(a.ready)
	if a.initErr != nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	renew := time.NewTicker(ControlRenewInterval)
	defer renew.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-renew.C:
			ctx, cancel := context.WithTimeout(context.Background(), controlIOTimeout)
			a.s.renewTable(ctx, a.id)
			cancel()
		case q := <-a.mailbox:
			r, e := a.execute(q.ctx, q.run)
			q.reply <- actorReply{r, e}
			if errors.Is(e, ErrFenced) {
				return
			}
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			r, err := a.execute(ctx, a.s.tickMutation)
			if err == nil && r.Status == "AUTO_START" {
				_, err = a.execute(ctx, a.s.prepareHand(0, ""))
				if err == nil {
					_, _ = a.execute(ctx, a.s.dealCommitted)
				}
			}
			cancel()
			if errors.Is(err, ErrFenced) {
				return
			}
		}
	}
}
func (a *tableActor) execute(ctx context.Context, run tableMutation) (Receipt, error) {
	tx, err := a.s.opts.Pool.Begin(ctx)
	if err != nil {
		return Receipt{}, err
	}
	defer rollback(tx)
	t, err := loadTable(ctx, tx, a.id, true)
	if err != nil {
		return Receipt{}, err
	}
	commitAttempted := false
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if !commitAttempted && tx.Rollback(cleanup) == nil && t.onRollback != nil {
			t.onRollback(cleanup)
		}
	}()
	if t.Epoch != a.epoch {
		return Receipt{}, ErrFenced
	}
	if t.NeedsReview && !opsMayEnterNeedsReview(ctx) {
		return Receipt{}, ErrNeedsReview
	}
	// All held domain rows are ordered before a narrow function locks the wallet.
	if _, err = tx.Exec(ctx, "SELECT seat_no FROM poker.seats WHERE table_id=$1 ORDER BY seat_no FOR UPDATE", a.id); err != nil {
		return Receipt{}, err
	}
	if _, err = tx.Exec(ctx, "SELECT session_id FROM poker.sessions WHERE table_id=$1 AND state<>'SETTLED' ORDER BY seat_no FOR UPDATE", a.id); err != nil {
		return Receipt{}, err
	}
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&t.Now); err != nil {
		return Receipt{}, err
	}
	r, err := run(ctx, tx, &t)
	if errors.Is(err, ErrCorruptSnapshot) && !isOpsCommand(ctx) {
		r, err = a.s.isolateHand(ctx, tx, &t)
	}
	if err != nil {
		return Receipt{}, err
	}
	if r.Duplicate || r.Status == "NOOP" {
		return r, nil
	}
	if t.beforeCommit != nil {
		if err = t.beforeCommit(ctx, &r); err != nil {
			return Receipt{}, err
		}
	}
	t.Version++
	r.Version = t.Version
	if r.TableID == "" {
		r.TableID = t.ID
	}
	if _, err = tx.Exec(ctx, "UPDATE poker.tables SET table_version=$2 WHERE table_id=$1 AND runtime_epoch=$3", t.ID, t.Version, a.epoch); err != nil {
		return Receipt{}, err
	}
	commitAttempted = true
	if err = tx.Commit(ctx); err != nil {
		return Receipt{}, err
	}
	if t.onCommit != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		t.onCommit(cleanup)
		cancel()
	}
	a.s.publish(t.ID, t.Version)
	return r, nil
}
