package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/relaykit/types"
)

// One writer and idle timer span every channel attempt and the eventual body.
// Only comments are sent before an upstream succeeds, so retries remain valid.
// In JSON mode the heartbeat is insignificant leading whitespace, which keeps
// Cloudflare's 100s header deadline from cutting slow non-stream requests.
type streamResponse struct {
	ctx                context.Context
	cancel             context.CancelFunc
	w                  http.ResponseWriter
	mu                 sync.Mutex
	timer              *time.Timer
	nextHeartbeat      time.Time
	done               chan struct{}
	stop               chan struct{}
	committed, stopped bool
	json               bool
}

func newStreamResponse(ctx context.Context, w http.ResponseWriter, grace time.Duration) *streamResponse {
	return newKeepaliveResponse(ctx, w, grace, false)
}

func newJSONResponse(ctx context.Context, w http.ResponseWriter, grace time.Duration) *streamResponse {
	return newKeepaliveResponse(ctx, w, grace, true)
}

func newKeepaliveResponse(ctx context.Context, w http.ResponseWriter, grace time.Duration, jsonBody bool) *streamResponse {
	ctx, cancel := context.WithCancel(ctx)
	s := &streamResponse{ctx: ctx, cancel: cancel, w: w, timer: time.NewTimer(grace), nextHeartbeat: time.Now().Add(grace), done: make(chan struct{}), stop: make(chan struct{}), json: jsonBody}
	heartbeat := []byte(": ping\n\n")
	if jsonBody {
		heartbeat = []byte("\n")
	}
	go func() {
		defer close(s.done)
		defer s.timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stop:
				return
			case <-s.timer.C:
				s.mu.Lock()
				// A normal frame may have reset the timer after this tick was
				// received but before the writer lock became available.
				if remaining := time.Until(s.nextHeartbeat); remaining > 0 {
					s.timer.Reset(remaining)
					s.mu.Unlock()
					continue
				}
				err := s.writeLocked(heartbeat, http.StatusOK)
				s.mu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()
	return s
}

func (s *streamResponse) writeLocked(raw []byte, status int) error {
	if s.stopped {
		return context.Canceled
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if !s.committed {
		if s.json {
			s.w.Header().Set("Content-Type", "application/json")
		} else {
			s.w.Header().Set("Content-Type", "text/event-stream")
		}
		s.w.Header().Set("Cache-Control", "no-store")
		s.w.Header().Set("X-Accel-Buffering", "no")
		s.w.WriteHeader(status)
		s.committed = true
	}
	_, err := s.w.Write(raw)
	if err == nil {
		err = http.NewResponseController(s.w).Flush()
	}
	if err != nil {
		s.cancel() // Also interrupt client.Do if the downstream has disconnected.
		return err
	}
	s.nextHeartbeat = time.Now().Add(15 * time.Second)
	s.timer.Reset(time.Until(s.nextHeartbeat))
	return nil
}

func (s *streamResponse) write(raw []byte, status int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.writeLocked(raw, status)
	return s.committed, err
}

// finish writes a complete JSON body and stops the heartbeat in the same lock.
func (s *streamResponse) finish(raw []byte, status int) (bool, error) {
	s.mu.Lock()
	err := s.writeLocked(raw, status)
	s.stopLocked()
	committed := s.committed
	s.mu.Unlock()
	<-s.done
	return committed, err
}

func (s *streamResponse) stopLocked() {
	if !s.stopped {
		s.stopped = true
		s.timer.Stop()
		close(s.stop)
	}
}

func (s *streamResponse) close() {
	s.mu.Lock()
	s.stopLocked()
	s.mu.Unlock()
	s.cancel()
	<-s.done
}

func (s *streamResponse) fail(detail *upstreamError, format types.RelayFormat) {
	s.mu.Lock()
	// Stop atomically with the final error, so no heartbeat can follow it.
	s.stopLocked()
	if s.committed && s.json {
		// The 200 status is already on the wire; the body still carries the error.
		_ = json.NewEncoder(s.w).Encode(map[string]any{"error": detail})
		_ = http.NewResponseController(s.w).Flush()
	} else if s.committed {
		// The business deadline may already have expired. The downstream write
		// deadline has 30s of headroom for this final error; no normal writes or
		// heartbeats resume. Do not turn a provider failure into cancellation.
		raw, _ := json.Marshal(map[string]any{"type": "error", "error": detail})
		prefix := ""
		if format != types.RelayFormatOpenAI {
			prefix = "event: error\n"
		}
		_, _ = s.w.Write(append([]byte(prefix+"data: "), append(raw, '\n', '\n')...))
		_ = http.NewResponseController(s.w).Flush()
	} else {
		writeUpstreamError(s.w, detail)
	}
	s.mu.Unlock()
	<-s.done
}
