package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccessGateSerializesOperations(t *testing.T) {
	gate := newAccessGate(0, 0)

	var active atomic.Int32
	var maxActive atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, 2)

	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			errs <- gate.Run(context.Background(), "test", func() error {
				current := active.Add(1)
				defer active.Add(-1)

				for {
					previous := maxActive.Load()
					if current <= previous || maxActive.CompareAndSwap(previous, current) {
						break
					}
				}

				time.Sleep(20 * time.Millisecond)
				return nil
			})
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), maxActive.Load())
}

func TestAccessGateEnforcesCooldown(t *testing.T) {
	const cooldown = 40 * time.Millisecond
	gate := newAccessGate(cooldown, 0)

	require.NoError(t, gate.Run(context.Background(), "first", func() error {
		return nil
	}))

	started := time.Now()
	require.NoError(t, gate.Run(context.Background(), "second", func() error {
		return nil
	}))

	assert.GreaterOrEqual(t, time.Since(started), 30*time.Millisecond)
}

func TestAccessGateHonorsCancellation(t *testing.T) {
	gate := newAccessGate(200*time.Millisecond, 0)

	require.NoError(t, gate.Run(context.Background(), "first", func() error {
		return nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	called := false
	err := gate.Run(ctx, "second", func() error {
		called = true
		return nil
	})

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, called)
}

func TestAccessGateHonorsCancellationWhileQueued(t *testing.T) {
	gate := newAccessGate(0, 0)

	holding := make(chan struct{})
	releaseHolder := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- gate.Run(context.Background(), "holder", func() error {
			close(holding)
			<-releaseHolder
			return nil
		})
	}()
	<-holding

	ctx, cancel := context.WithCancel(context.Background())
	var called atomic.Bool
	queuedDone := make(chan error, 1)
	queuedStarted := make(chan struct{})
	go func() {
		close(queuedStarted)
		queuedDone <- gate.Run(ctx, "queued", func() error {
			called.Store(true)
			return nil
		})
	}()

	// The queued request must return promptly when cancelled.
	<-queuedStarted
	cancel()

	select {
	case err := <-queuedDone:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("queued request did not return promptly after cancellation")
	}
	assert.False(t, called.Load())

	close(releaseHolder)
	require.NoError(t, <-holderDone)

	// Releasing the holder must not run the cancelled request.
	time.Sleep(20 * time.Millisecond)
	assert.False(t, called.Load())
}

func TestAccessPolicyValidation(t *testing.T) {
	policy := DefaultAccessPolicy()
	require.NoError(t, policy.Validate())

	policy.MaxComments = 0
	assert.EqualError(t, policy.Validate(), "maximum comments must be greater than zero")
}

func TestCommentPolicyCapsLargeRequests(t *testing.T) {
	policy := DefaultAccessPolicy()
	policy.MaxComments = 25
	policy.MaxReplies = 5
	service := NewXiaohongshuService(policy)

	t.Run("caps comment count", func(t *testing.T) {
		config := service.enforceCommentPolicy(commentConfig(26, 5, false, "slow"))
		assert.Equal(t, 25, config.MaxCommentItems)
	})

	t.Run("caps reply expansion", func(t *testing.T) {
		config := service.enforceCommentPolicy(commentConfig(20, 6, true, "slow"))
		assert.Equal(t, 5, config.MaxRepliesThreshold)
	})

	t.Run("forces slow scrolling", func(t *testing.T) {
		config := service.enforceCommentPolicy(commentConfig(20, 5, true, "fast"))
		assert.Equal(t, "slow", config.ScrollSpeed)
	})
}

func commentConfig(maxComments, maxReplies int, clickReplies bool, scrollSpeed string) xiaohongshu.CommentLoadConfig {
	return xiaohongshu.CommentLoadConfig{
		MaxCommentItems:     maxComments,
		MaxRepliesThreshold: maxReplies,
		ClickMoreReplies:    clickReplies,
		ScrollSpeed:         scrollSpeed,
	}
}
