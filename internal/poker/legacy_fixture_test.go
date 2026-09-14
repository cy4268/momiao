package poker

// Adapter for pre-control PG acceptance fixtures. It deliberately uses an
// explicitly synthetic control store and auth validator; the new TestControl
// suite is the real PG+TLS Redis proof. No production authority bypass exists.
import (
	"context"
	"strconv"
	"sync"
	"testing"
)

type fixtureControls struct {
	mu     sync.Mutex
	values map[string]string
}

func (f *fixtureControls) ControlLoad(_ context.Context, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.values[id], nil
}
func (f *fixtureControls) ControlAssign(_ context.Context, id, old, next string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.values[id] != old {
		return false, nil
	}
	f.values[id] = next
	return true, nil
}
func (f *fixtureControls) ControlRenew(_ context.Context, id, owner string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.values[id] == owner, nil
}
func (f *fixtureControls) ControlRelease(_ context.Context, id, owner string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.values[id] != owner {
		return false, nil
	}
	delete(f.values, id)
	return true, nil
}
func fixtureAuth(_ context.Context, a AuthSession) error {
	if a.UserID < 910001 || a.UserID > 910003 || a.SessionVersion != 1 || a.SecurityEpoch != 1 {
		return ErrDenied
	}
	return nil
}
func legacyTestNew(opts Options) (*Service, error) {
	opts.Controls = &fixtureControls{values: map[string]string{}}
	opts.ValidateSession = fixtureAuth
	return New(opts)
}
func legacyRef(t *testing.T, s *Service, table string, user int64) ControlRef {
	t.Helper()
	s.mu.Lock()
	var found ControlRef
	for _, r := range s.connections {
		if r.UserID == user && r.TableID == table && r.ControlEpoch > 0 {
			found = r
			break
		}
	}
	s.mu.Unlock()
	if found.ConnectionID != "" {
		return found
	}
	return connected(t, s, fixtureRef(table, user), true).Ref
}
func legacySession(t *testing.T, s *Service, c SessionCommand) SessionCommand {
	t.Helper()
	ref := legacyRef(t, s, c.TableID, c.UserID)
	v, e := s.ViewForConnection(context.Background(), ref)
	if e != nil {
		t.Fatal(e)
	}
	c.Control = &ref
	c.ExpectedTableVersion, _ = strconv.ParseUint(v.TableVersion, 10, 64)
	if v.Hand != nil {
		c.HandID = v.Hand.HandID
		c.ExpectedHandVersion, _ = strconv.ParseUint(v.Hand.HandVersion, 10, 64)
	}
	return c
}
func legacyAct(t *testing.T, s *Service, c ActCommand) (Receipt, error) {
	ref := legacyRef(t, s, c.TableID, c.UserID)
	v, e := s.ViewForConnection(context.Background(), ref)
	if e != nil {
		return Receipt{}, e
	}
	c.Control = &ref
	c.ControlEpoch = ref.ControlEpoch
	c.TableVersion, _ = strconv.ParseUint(v.TableVersion, 10, 64)
	c.HandVersion, _ = strconv.ParseUint(v.Hand.HandVersion, 10, 64)
	return s.Act(context.Background(), c)
}
