package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
)

var errLoginSessionClosed = errors.New("login session is closed")

const loginStateInspectionLimit = 5 * time.Second

type loginSession struct {
	mu        sync.Mutex
	expiresAt time.Time
	cancel    context.CancelFunc
	inspect   func(context.Context) (xiaohongshu.LoginState, error)
	persist   func() error
	close     func()
	closed    bool
}

func newLoginSession(
	expiresAt time.Time,
	cancel context.CancelFunc,
	inspect func(context.Context) (xiaohongshu.LoginState, error),
	persist func() error,
	closeSession func(),
) *loginSession {
	return &loginSession{
		expiresAt: expiresAt,
		cancel:    cancel,
		inspect:   inspect,
		persist:   persist,
		close:     closeSession,
	}
}

func (s *loginSession) currentState(ctx context.Context) (xiaohongshu.LoginState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return xiaohongshu.LoginState{}, errLoginSessionClosed
	}
	inspectCtx, cancel := context.WithTimeout(ctx, loginStateInspectionLimit)
	defer cancel()
	return s.inspect(inspectCtx)
}

func (s *loginSession) saveCookies() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errLoginSessionClosed
	}
	return s.persist()
}

func (s *loginSession) remaining() time.Duration {
	remaining := time.Until(s.expiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *loginSession) stop() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	cancel := s.cancel
	closeSession := s.close
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if closeSession != nil {
		closeSession()
	}
}

// loginSessions keeps one active QR session and reuses its browser page.
type loginSessions struct {
	mu       sync.Mutex
	seq      uint64
	current  *loginSession
	recent   xiaohongshu.LoginState
	recentAt time.Time
}

func (l *loginSessions) active() *loginSession {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.current
}

// start records a new session after reuse is no longer possible.
func (l *loginSessions) start(session *loginSession) uint64 {
	l.mu.Lock()
	previous := l.current
	l.seq++
	seq := l.seq
	l.current = session
	l.recent = xiaohongshu.LoginState{}
	l.recentAt = time.Time{}
	l.mu.Unlock()

	if previous != nil {
		previous.stop()
	}
	return seq
}

func (l *loginSessions) remember(state xiaohongshu.LoginState) {
	l.mu.Lock()
	l.recent = state
	l.recentAt = time.Now()
	l.mu.Unlock()
}

func (l *loginSessions) recentState(maxAge time.Duration) (xiaohongshu.LoginState, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.recentAt.IsZero() || time.Since(l.recentAt) > maxAge {
		return xiaohongshu.LoginState{}, false
	}
	return l.recent, true
}

func (l *loginSessions) consumeRecentState(
	maxAge time.Duration,
	stage xiaohongshu.LoginStage,
) (xiaohongshu.LoginState, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.recentAt.IsZero() ||
		time.Since(l.recentAt) > maxAge ||
		l.recent.Stage != stage {
		return xiaohongshu.LoginState{}, false
	}

	state := l.recent
	l.recent = xiaohongshu.LoginState{}
	l.recentAt = time.Time{}
	return state, true
}

func (l *loginSessions) finish(seq uint64) {
	l.mu.Lock()
	var session *loginSession
	if l.seq == seq {
		session = l.current
		l.current = nil
	}
	l.mu.Unlock()

	if session != nil {
		session.stop()
	}
}

func (l *loginSessions) stopCurrent() {
	l.mu.Lock()
	session := l.current
	l.current = nil
	l.seq++
	l.mu.Unlock()

	if session != nil {
		session.stop()
	}
}
