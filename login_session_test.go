package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/stretchr/testify/assert"
)

func testLoginSession(closed *int) *loginSession {
	return newLoginSession(
		time.Now().Add(time.Minute),
		func() {},
		func(context.Context) (xiaohongshu.LoginState, error) {
			return xiaohongshu.LoginState{Stage: xiaohongshu.LoginStageQRCode}, nil
		},
		func() error { return nil },
		func() { *closed++ },
	)
}

func TestLoginSessions(t *testing.T) {
	t.Run("reuses active session", func(t *testing.T) {
		var sessions loginSessions
		closed := 0
		current := testLoginSession(&closed)

		sessions.start(current)

		assert.Same(t, current, sessions.active())
		assert.Same(t, current, sessions.active())
		assert.Equal(t, 0, closed)
	})

	t.Run("new session closes previous session", func(t *testing.T) {
		var sessions loginSessions
		firstClosed := 0
		secondClosed := 0

		sessions.start(testLoginSession(&firstClosed))
		second := testLoginSession(&secondClosed)
		sessions.start(second)

		assert.Equal(t, 1, firstClosed)
		assert.Equal(t, 0, secondClosed)
		assert.Same(t, second, sessions.active())
	})

	t.Run("finishing clears current session", func(t *testing.T) {
		var sessions loginSessions
		closed := 0
		seq := sessions.start(testLoginSession(&closed))

		sessions.finish(seq)

		assert.Nil(t, sessions.active())
		assert.Equal(t, 1, closed)
	})

	t.Run("recent success survives browser close", func(t *testing.T) {
		var sessions loginSessions
		closed := 0
		seq := sessions.start(testLoginSession(&closed))
		loggedIn := xiaohongshu.LoginState{Stage: xiaohongshu.LoginStageLoggedIn}

		sessions.remember(loggedIn)
		sessions.finish(seq)

		assert.Nil(t, sessions.active())
		assert.Equal(t, 1, closed)
		got, ok := sessions.recentState(time.Minute)
		assert.True(t, ok)
		assert.Equal(t, loggedIn, got)
	})

	t.Run("finishing an old session preserves the new session", func(t *testing.T) {
		var sessions loginSessions
		oldClosed := 0
		newClosed := 0
		oldSeq := sessions.start(testLoginSession(&oldClosed))
		current := testLoginSession(&newClosed)
		sessions.start(current)

		sessions.finish(oldSeq)

		assert.Same(t, current, sessions.active())
		assert.Equal(t, 1, oldClosed)
		assert.Equal(t, 0, newClosed)
	})

	t.Run("concurrent sessions receive unique sequence numbers", func(t *testing.T) {
		var sessions loginSessions
		const count = 50

		var mu sync.Mutex
		seen := make(map[uint64]bool, count)

		var wg sync.WaitGroup
		for range count {
			wg.Add(1)
			go func() {
				defer wg.Done()
				closed := 0
				seq := sessions.start(testLoginSession(&closed))
				mu.Lock()
				seen[seq] = true
				mu.Unlock()
			}()
		}
		wg.Wait()

		assert.Len(t, seen, count)
	})
}

func TestConsumeRecentPersistenceFailure(t *testing.T) {
	var sessions loginSessions
	failed := xiaohongshu.LoginState{
		Stage: xiaohongshu.LoginStagePersistenceFailed,
	}
	sessions.remember(failed)

	got, ok := sessions.consumeRecentState(
		time.Minute,
		xiaohongshu.LoginStagePersistenceFailed,
	)
	assert.True(t, ok)
	assert.Equal(t, failed, got)

	_, ok = sessions.recentState(time.Minute)
	assert.False(t, ok)
}
