package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBrowserProcess struct {
	pages  atomic.Int32
	closes atomic.Int32
}

type panicCloseBrowserProcess struct {
	fakeBrowserProcess
}

func (b *panicCloseBrowserProcess) Close() {
	panic("browser already closed")
}

func (b *fakeBrowserProcess) NewPage() *rod.Page {
	b.pages.Add(1)
	return nil
}

func (b *fakeBrowserProcess) Close() {
	b.closes.Add(1)
}

func TestBrowserRuntimeReusesProcess(t *testing.T) {
	process := &fakeBrowserProcess{}
	var launches atomic.Int32
	runtime := newBrowserRuntime(func() browserProcess {
		launches.Add(1)
		return process
	})

	for range 2 {
		require.NoError(t, runtime.Run(context.Background(), func(*rod.Page) error {
			return nil
		}))
	}

	assert.Equal(t, int32(1), launches.Load())
	assert.Equal(t, int32(2), process.pages.Load())
	assert.Equal(t, "ready", runtime.Snapshot().State)

	require.NoError(t, runtime.Close(context.Background()))
	assert.Equal(t, int32(1), process.closes.Load())
}

func TestBrowserRuntimeBoundsStartup(t *testing.T) {
	releaseStartup := make(chan struct{})
	process := &fakeBrowserProcess{}
	runtime := newBrowserRuntime(func() browserProcess {
		<-releaseStartup
		return process
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	started := time.Now()
	err := runtime.Run(ctx, func(*rod.Page) error {
		return nil
	})
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), 200*time.Millisecond)
	assert.Equal(t, "degraded", runtime.Snapshot().State)

	started = time.Now()
	err = runtime.Run(context.Background(), func(*rod.Page) error {
		return nil
	})
	assert.ErrorAs(t, err, new(*browserRuntimeUnavailableError))
	assert.Less(t, time.Since(started), 50*time.Millisecond)

	close(releaseStartup)
	require.Eventually(t, func() bool {
		return runtime.Snapshot().State == "cold"
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(1), process.closes.Load())
}

func TestBrowserRuntimeCancelsRunningPageOperation(t *testing.T) {
	process := &fakeBrowserProcess{}
	var launches atomic.Int32
	runtime := newBrowserRuntime(func() browserProcess {
		launches.Add(1)
		return process
	})

	releaseOperation := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	started := time.Now()
	err := runtime.Run(ctx, func(*rod.Page) error {
		<-releaseOperation
		return nil
	})
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), 200*time.Millisecond)

	close(releaseOperation)
	require.NoError(t, runtime.Run(context.Background(), func(*rod.Page) error {
		return nil
	}))
	assert.Equal(t, int32(1), launches.Load())
	assert.Equal(t, int32(0), process.closes.Load())
}

func TestBrowserRuntimeRecoversAfterCleanupPanic(t *testing.T) {
	var launches atomic.Int32
	runtime := newBrowserRuntime(func() browserProcess {
		if launches.Add(1) == 1 {
			return &panicCloseBrowserProcess{}
		}
		return &fakeBrowserProcess{}
	})

	err := runtime.Run(context.Background(), func(*rod.Page) error {
		return assert.AnError
	})
	require.ErrorIs(t, err, assert.AnError)
	runtime.Reset("force cleanup")

	require.Eventually(t, func() bool {
		return runtime.Snapshot().State == "cold"
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, runtime.Run(context.Background(), func(*rod.Page) error {
		return nil
	}))
	assert.Equal(t, int32(2), launches.Load())
}

func TestServiceRefreshesBrowserAfterCookiesChange(t *testing.T) {
	var launches atomic.Int32
	first := &fakeBrowserProcess{}
	second := &fakeBrowserProcess{}
	service := NewXiaohongshuService()
	service.readBrowser = newBrowserRuntime(func() browserProcess {
		if launches.Add(1) == 1 {
			return first
		}
		return second
	})

	require.NoError(t, service.readBrowser.Run(
		context.Background(),
		func(*rod.Page) error { return nil },
	))
	service.readBrowserStale.Store(true)

	_, err := withServiceReadPage(
		service,
		context.Background(),
		func(*rod.Page) (struct{}, error) { return struct{}{}, nil },
	)
	require.NoError(t, err)
	assert.Equal(t, int32(2), launches.Load())
	assert.Equal(t, int32(1), first.closes.Load())
}
