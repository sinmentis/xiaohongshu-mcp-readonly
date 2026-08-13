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
	gate := newAccessGate(0, 0, time.Second)

	var active atomic.Int32
	var maxActive atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, 2)

	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			errs <- gate.Run(context.Background(), "test", time.Second, func(context.Context) error {
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
	gate := newAccessGate(cooldown, 0, time.Second)

	require.NoError(t, gate.Run(context.Background(), "first", time.Second, func(context.Context) error {
		return nil
	}))

	started := time.Now()
	require.NoError(t, gate.Run(context.Background(), "second", time.Second, func(context.Context) error {
		return nil
	}))

	assert.GreaterOrEqual(t, time.Since(started), 30*time.Millisecond)
}

func TestAccessGateReportsCooldownAndStart(t *testing.T) {
	gate := newAccessGate(40*time.Millisecond, 0, time.Second)
	require.NoError(t, gate.Run(
		context.Background(),
		"first",
		time.Second,
		func(context.Context) error { return nil },
	))

	messages := make(chan string, 4)
	ctx := withProgressReporter(context.Background(), func(message string) {
		messages <- message
	})
	require.NoError(t, gate.Run(
		ctx,
		"second",
		time.Second,
		func(context.Context) error { return nil },
	))
	close(messages)

	var joined string
	for message := range messages {
		joined += message + "\n"
	}
	assert.Contains(t, joined, "access cooldown")
	assert.Contains(t, joined, "effective deadline")
}

func TestAccessGateHonorsCancellation(t *testing.T) {
	gate := newAccessGate(200*time.Millisecond, 0, time.Second)

	require.NoError(t, gate.Run(context.Background(), "first", time.Second, func(context.Context) error {
		return nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	called := false
	err := gate.Run(ctx, "second", time.Second, func(context.Context) error {
		called = true
		return nil
	})

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, called)
}

func TestAccessGateDoesNotRunAlreadyCancelledRequest(t *testing.T) {
	for range 100 {
		gate := newAccessGate(0, 0, time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		called := false
		err := gate.Run(ctx, "cancelled", time.Second, func(context.Context) error {
			called = true
			return nil
		})

		assert.ErrorIs(t, err, context.Canceled)
		assert.False(t, called)
	}
}

func TestAccessGateHonorsCancellationWhileQueued(t *testing.T) {
	gate := newAccessGate(0, 0, time.Second)

	holding := make(chan struct{})
	releaseHolder := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- gate.Run(context.Background(), "holder", time.Second, func(context.Context) error {
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
		queuedDone <- gate.Run(ctx, "queued", time.Second, func(context.Context) error {
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

func TestAccessGateAllowsCooperativeCancellationToCleanUp(t *testing.T) {
	gate := newAccessGate(0, 0, time.Second)
	gate.cancellationGrace = 20 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- gate.Run(ctx, "cancelled", time.Second, func(operationCtx context.Context) error {
			close(started)
			<-operationCtx.Done()
			return operationCtx.Err()
		})
	}()
	<-started
	cancel()

	require.ErrorIs(t, <-done, context.Canceled)
	require.Eventually(t, func() bool {
		return gate.Snapshot().State == "idle"
	}, time.Second, 10*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	assert.NotEqual(t, "degraded", gate.Snapshot().State)
}

func TestAccessGateEscalatesIgnoredCancellation(t *testing.T) {
	gate := newAccessGate(0, 0, time.Second)
	gate.cancellationGrace = 20 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- gate.Run(ctx, "cancelled", time.Second, func(context.Context) error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started
	cancel()

	require.ErrorIs(t, <-done, context.Canceled)
	require.Eventually(t, func() bool {
		return gate.Snapshot().State == "degraded"
	}, time.Second, 10*time.Millisecond)

	close(release)
	require.Eventually(t, func() bool {
		return gate.Snapshot().State == "idle"
	}, time.Second, 10*time.Millisecond)
}

func TestAccessGateReturnsWhenRunningOperationContextExpires(t *testing.T) {
	gate := newAccessGate(0, 0, time.Second)

	releaseOperation := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- gate.Run(context.Background(), "blocked", 20*time.Millisecond, func(context.Context) error {
			<-releaseOperation
			return nil
		})
	}()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(200 * time.Millisecond):
		close(releaseOperation)
		<-done
		t.Fatal("running operation did not return after its context deadline")
	}

	snapshot := gate.Snapshot()
	assert.Equal(t, "degraded", snapshot.State)
	assert.Equal(t, "cancelling", snapshot.Phase)

	started := time.Now()
	err := gate.Run(context.Background(), "next", time.Second, func(context.Context) error {
		return nil
	})
	assert.ErrorAs(t, err, new(*accessGateUnavailableError))
	assert.Less(t, time.Since(started), 50*time.Millisecond)

	close(releaseOperation)
	require.Eventually(t, func() bool {
		return gate.Snapshot().State == "idle"
	}, time.Second, 10*time.Millisecond)
}

func TestAccessGateReleasesQueuedCallerWhenActiveOperationTimesOut(t *testing.T) {
	gate := newAccessGate(0, 0, time.Second)

	releaseOperation := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- gate.Run(
			context.Background(),
			"holder",
			40*time.Millisecond,
			func(context.Context) error {
				<-releaseOperation
				return nil
			},
		)
	}()
	require.Eventually(t, func() bool {
		return gate.Snapshot().Phase == "running"
	}, time.Second, 10*time.Millisecond)

	queuedDone := make(chan error, 1)
	go func() {
		queuedDone <- gate.Run(
			context.Background(),
			"queued",
			time.Second,
			func(context.Context) error {
				return nil
			},
		)
	}()
	require.Eventually(t, func() bool {
		return gate.Snapshot().Queued == 1
	}, time.Second, 10*time.Millisecond)

	require.ErrorIs(t, <-holderDone, context.DeadlineExceeded)
	select {
	case err := <-queuedDone:
		assert.ErrorAs(t, err, new(*accessGateUnavailableError))
	case <-time.After(200 * time.Millisecond):
		t.Fatal("queued request kept waiting after the active operation timed out")
	}

	close(releaseOperation)
	require.Eventually(t, func() bool {
		return gate.Snapshot().State == "idle"
	}, time.Second, 10*time.Millisecond)
}

func TestAccessPolicyValidation(t *testing.T) {
	policy := DefaultAccessPolicy()
	require.NoError(t, policy.Validate())

	policy.MaxQueueWait = 0
	assert.EqualError(t, policy.Validate(), "maximum queue wait must be greater than zero")

	policy = DefaultAccessPolicy()
	policy.MaxComments = 0
	assert.EqualError(t, policy.Validate(), "maximum comments must be greater than zero")
}

func TestServiceDefaultsMissingQueueWait(t *testing.T) {
	service := NewXiaohongshuService(AccessPolicy{
		MaxComments: 10,
		MaxReplies:  5,
	})

	assert.Equal(t, defaultMaxQueueWait, service.accessGate.maxQueueWait)
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
