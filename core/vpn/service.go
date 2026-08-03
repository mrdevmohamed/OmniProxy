// Package vpn implements the VPN Service (PRD §7.1): the connection state
// machine (Disconnected/Connecting/Connected/Reconnecting/Error), connect /
// disconnect / reconnect with backoff, and VPNSession tracking.
package vpn

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"omniproxy/core/api"
	"omniproxy/core/log"
	"omniproxy/core/models"
)

// ErrBusy is returned when an operation cannot be honored in the current state.
var ErrBusy = errors.New("vpn: busy")

// ProfileResolver resolves a server id to a profile at connect time.
type ProfileResolver interface {
	Resolve(serverID string) (*models.ServerProfile, error)
}

// Tunneler is the tunnel lifecycle the service drives.
type Tunneler interface {
	Start(p *models.ServerProfile, mode models.ConnectionMode) error
	Stop() error
}

// RetryPolicy computes reconnect backoff. MaxAttempts <= 0 means unlimited.
type RetryPolicy interface {
	Backoff(attempt int) time.Duration
	MaxAttempts() int
}

// DefaultRetryPolicy backs off 1s, 2s, 4s ... capped at 30s, up to 5 attempts.
type DefaultRetryPolicy struct{}

// Backoff implements RetryPolicy.
func (DefaultRetryPolicy) Backoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	d := time.Second
	for i := 0; i < attempt && d < 30*time.Second; i++ {
		d *= 2
	}
	return d
}

// MaxAttempts implements RetryPolicy.
func (DefaultRetryPolicy) MaxAttempts() int { return 5 }

type command int

const (
	cmdDisconnect command = iota
	cmdReconnect
)

// runState tracks one live run loop and its session. Threading the session
// through the loop (rather than reading the shared one) keeps teardown and
// bookkeeping correct when a new connect replaces the previous session.
type runState struct {
	ctx     context.Context
	cancel  context.CancelFunc
	cmds    chan command
	done    chan struct{}
	session *models.VPNSession
}

// Service implements the connection state machine.
type Service struct {
	profiles ProfileResolver
	tunnel   Tunneler
	logger   *log.Logger
	events   *api.EventBus

	mu      sync.Mutex
	state   models.ConnectionState
	session *models.VPNSession
	active  *runState

	auto  bool
	retry RetryPolicy
}

// NewService returns a service. events may be nil (no event emission).
func NewService(profiles ProfileResolver, t Tunneler, logger *log.Logger, events *api.EventBus) *Service {
	return &Service{
		profiles: profiles,
		tunnel:   t,
		logger:   logger,
		events:   events,
		state:    models.StateDisconnected,
		auto:     true,
		retry:    DefaultRetryPolicy{},
	}
}

// SetAutoReconnect enables or disables automatic retry with backoff.
func (s *Service) SetAutoReconnect(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auto = on
}

// SetRetryPolicy replaces the reconnect backoff policy.
func (s *Service) SetRetryPolicy(p RetryPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p == nil {
		p = DefaultRetryPolicy{}
	}
	s.retry = p
}

// State returns the current connection state.
func (s *Service) State() models.ConnectionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Session returns a clone of the current session, or nil.
func (s *Service) Session() *models.VPNSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSession(s.session)
}

// ConnectionState returns the current state and a session clone (nil when idle).
func (s *Service) ConnectionState() (models.ConnectionState, *models.VPNSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, cloneSession(s.session)
}

// RunningTo reports whether a live session targets serverID.
func (s *Service) RunningTo(serverID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session != nil && s.session.Running() && s.session.ServerID == serverID
}

// Connect starts a connection to serverID in the given mode (VPN when mode is
// invalid/empty). Returns ErrBusy when already connecting/connected. The state
// is set to Connecting synchronously; the result arrives via stateChanged
// events and getConnectionState.
func (s *Service) Connect(serverID string, mode models.ConnectionMode) error {
	p, err := s.profiles.Resolve(serverID)
	if err != nil {
		return err
	}
	if !mode.Valid() {
		mode = models.ModeVPN
	}

	s.mu.Lock()
	if s.state == models.StateConnecting || s.state == models.StateConnected || s.state == models.StateReconnecting {
		s.mu.Unlock()
		return ErrBusy
	}
	if s.active != nil {
		s.active.cancel() // reap a loop that has not finished tearing down yet
	}
	session := &models.VPNSession{
		ID:        models.NewID(),
		ServerID:  p.ID,
		Mode:      mode,
		StartedAt: time.Now().UTC(),
	}
	session.AddState(models.StateConnecting)
	s.state = models.StateConnecting
	s.session = session

	ctx, cancel := context.WithCancel(context.Background())
	rs := &runState{ctx: ctx, cancel: cancel, cmds: make(chan command, 1), done: make(chan struct{}), session: session}
	s.active = rs
	s.mu.Unlock()

	s.emitState(models.StateConnecting)
	s.logger.Infof("vpn", "connecting to %q (%s)", p.Name, mode)
	go s.runLoop(rs, p, mode)
	return nil
}

// Disconnect ends the current session and waits briefly for the loop to finish.
func (s *Service) Disconnect() error {
	s.mu.Lock()
	rs := s.active
	state := s.state
	s.mu.Unlock()

	if rs == nil {
		if state == models.StateDisconnected {
			return nil
		}
		// Stale Error state with no live loop: reset to disconnected.
		s.mu.Lock()
		s.state = models.StateDisconnected
		s.mu.Unlock()
		s.emitState(models.StateDisconnected)
		return nil
	}
	s.sendCmd(rs, cmdDisconnect)
	select {
	case <-rs.done:
		return nil
	case <-time.After(5 * time.Second):
		s.forceDisconnect()
		return nil
	}
}

// Reconnect re-establishes the current session. Allowed while connected,
// reconnecting, or after a failed connect (Error).
func (s *Service) Reconnect() error {
	s.mu.Lock()
	rs := s.active
	state := s.state
	sess := cloneSession(s.session)
	s.mu.Unlock()

	switch state {
	case models.StateConnected, models.StateReconnecting:
	case models.StateError:
		if rs == nil && sess != nil {
			return s.Connect(sess.ServerID, sess.Mode)
		}
	default:
		return ErrBusy
	}
	if rs == nil {
		return ErrBusy
	}
	s.sendCmd(rs, cmdReconnect)
	return nil
}

// runLoop drives the tunnel: connect, then wait for commands/context until the
// session ends. Auto-reconnect retries failed starts (and reconnects) with
// backoff up to the retry policy's limit.
func (s *Service) runLoop(rs *runState, p *models.ServerProfile, mode models.ConnectionMode) {
	defer s.finish(rs)

	attempt := 0
	for {
		err := s.tunnel.Start(p, mode)
		if err == nil {
			attempt = 0
			s.transition(rs, models.StateConnected, nil)
			s.logger.Infof("vpn", "connected to %q", p.Name)
		} else {
			if !s.shouldRetry(attempt) {
				s.transitionError(rs, err)
				return
			}
			delay := s.backoff(attempt)
			attempt++
			s.transition(rs, models.StateReconnecting, err)
			if !s.wait(rs, delay) {
				return
			}
			continue
		}

		select {
		case cmd := <-rs.cmds:
			switch cmd {
			case cmdDisconnect:
				s.teardown(rs, models.StateDisconnected, nil)
				return
			case cmdReconnect:
				s.transition(rs, models.StateReconnecting, nil)
				_ = s.tunnel.Stop()
				attempt = 0
				continue
			}
		case <-rs.ctx.Done():
			s.teardown(rs, models.StateDisconnected, nil)
			return
		}
	}
}

// shouldRetry reports whether a failed attempt should be retried.
func (s *Service) shouldRetry(attempt int) bool {
	s.mu.Lock()
	auto := s.auto
	max := 0
	if s.retry != nil {
		max = s.retry.MaxAttempts()
	}
	s.mu.Unlock()
	return auto && (max <= 0 || attempt < max)
}

// backoff returns the delay for a failed attempt.
func (s *Service) backoff(attempt int) time.Duration {
	s.mu.Lock()
	p := s.retry
	s.mu.Unlock()
	if p == nil {
		p = DefaultRetryPolicy{}
	}
	return p.Backoff(attempt)
}

// wait blocks during a retry backoff. Returns false when the session ended.
func (s *Service) wait(rs *runState, delay time.Duration) bool {
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case cmd := <-rs.cmds:
		if cmd == cmdDisconnect {
			s.teardown(rs, models.StateDisconnected, nil)
			return false
		}
		return true // reconnect: retry now
	case <-t.C:
		return true
	case <-rs.ctx.Done():
		s.teardown(rs, models.StateDisconnected, nil)
		return false
	}
}

// transition updates state, session history, and emits a stateChanged event.
func (s *Service) transition(rs *runState, state models.ConnectionState, err error) {
	s.mu.Lock()
	s.state = state
	if sess := rs.session; sess != nil {
		sess.AddState(state)
		if state == models.StateError && err != nil {
			sess.Error = &models.SessionError{Code: classify(err), Message: err.Error()}
		}
	}
	s.mu.Unlock()
	s.emitState(state)
}

func (s *Service) transitionError(rs *runState, err error) {
	s.transition(rs, models.StateError, err)
	s.logger.Errorf("vpn", "tunnel failed: %v", err)
}

// teardown stops the tunnel and finalizes the session. Idempotent.
func (s *Service) teardown(rs *runState, state models.ConnectionState, sessErr *models.SessionError) {
	_ = s.tunnel.Stop()
	s.mu.Lock()
	sess := rs.session
	if sess != nil && sess.EndedAt == nil {
		now := time.Now().UTC()
		sess.EndedAt = &now
		if sessErr != nil {
			sess.Error = sessErr
		}
		sess.AddState(state)
	}
	s.state = state
	s.mu.Unlock()
	s.emitState(state)
	s.logger.Infof("vpn", "session ended: %s", state)
}

// finish runs when the loop exits: reap the active run and finalize the
// session's end time if teardown did not.
func (s *Service) finish(rs *runState) {
	s.mu.Lock()
	if s.active == rs {
		s.active = nil
	}
	if sess := rs.session; sess != nil && sess.EndedAt == nil {
		now := time.Now().UTC()
		sess.EndedAt = &now
	}
	s.mu.Unlock()
	close(rs.done)
}

// forceDisconnect is the safety net for a loop that ignores commands.
func (s *Service) forceDisconnect() {
	s.mu.Lock()
	rs := s.active
	s.mu.Unlock()
	if rs == nil {
		return
	}
	rs.cancel()
	_ = s.tunnel.Stop()
	s.teardown(rs, models.StateDisconnected, nil)
}

func (s *Service) sendCmd(rs *runState, c command) {
	select {
	case rs.cmds <- c:
	default:
	}
}

func (s *Service) emitState(state models.ConnectionState) {
	if s.events == nil {
		return
	}
	s.mu.Lock()
	sess := cloneSession(s.session)
	s.mu.Unlock()
	s.events.Publish(api.Event{
		Type: api.EventStateChanged,
		Data: api.StateResponse{State: state, Session: sess},
	})
}

func cloneSession(sess *models.VPNSession) *models.VPNSession {
	if sess == nil {
		return nil
	}
	cp := *sess
	cp.StatusHistory = append([]models.StatusChange(nil), sess.StatusHistory...)
	if sess.Error != nil {
		e := *sess.Error
		cp.Error = &e
	}
	if sess.EndedAt != nil {
		t := *sess.EndedAt
		cp.EndedAt = &t
	}
	return &cp
}

// classify maps a tunnel error onto a contract error code.
func classify(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "permission denied"),
		strings.Contains(msg, "operation not permitted"),
		strings.Contains(msg, "not permitted"),
		strings.Contains(msg, "privileged helper"):
		return api.ErrCodeUnauthorized
	case strings.Contains(msg, "already running"):
		return api.ErrCodeBusy
	default:
		return api.ErrCodeEngine
	}
}
