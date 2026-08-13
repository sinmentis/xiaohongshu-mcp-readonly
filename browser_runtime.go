package main

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/sirupsen/logrus"
)

const browserCloseWarning = 5 * time.Second

type browserProcess interface {
	NewPage() *rod.Page
	Close()
}

type browserProcessFactory func() browserProcess

type browserRuntimeSnapshot struct {
	State       string
	Launches    uint64
	LastFailure string
}

type browserRuntimeUnavailableError struct {
	State string
}

func (e *browserRuntimeUnavailableError) Error() string {
	return fmt.Sprintf(
		"browser runtime is %s; inspect /health before retrying or restart the service",
		e.State,
	)
}

type browserStagePanicError struct {
	Stage string
}

func (e *browserStagePanicError) Error() string {
	return fmt.Sprintf("browser %s failed internally; check server logs", e.Stage)
}

// browserRuntime reuses one browser process while opening a fresh page per operation.
type browserRuntime struct {
	mu          sync.Mutex
	factory     browserProcessFactory
	current     browserProcess
	state       string
	launches    uint64
	lastFailure string
	closed      bool
}

func newBrowserRuntime(factory browserProcessFactory) *browserRuntime {
	return &browserRuntime{
		factory: factory,
		state:   "cold",
	}
}

func (r *browserRuntime) Run(ctx context.Context, fn func(*rod.Page) error) error {
	process, err := r.get(ctx)
	if err != nil {
		return err
	}

	page, err := r.newPage(ctx, process)
	if err != nil {
		return err
	}

	result := make(chan error, 1)
	go func() {
		result <- runBrowserStage("page operation", func() error {
			return fn(page)
		})
	}()

	select {
	case err := <-result:
		if closeErr := closeRodPage(page); closeErr != nil {
			r.resetAsync(process, "page cleanup failed")
			if err == nil {
				return fmt.Errorf("close browser page: %w", closeErr)
			}
		}
		if ctx.Err() == nil && shouldResetBrowser(err) {
			r.resetAsync(process, "browser operation failed")
		}
		return err
	case <-ctx.Done():
		if err := closeRodPage(page); err != nil {
			r.resetAsync(process, "timed-out page cleanup failed")
		}
		go func() { <-result }()
		return ctx.Err()
	}
}

func (r *browserRuntime) get(ctx context.Context) (browserProcess, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, &browserRuntimeUnavailableError{State: "closed"}
	}
	if r.current != nil {
		process := r.current
		r.mu.Unlock()
		return process, nil
	}
	if r.state != "cold" {
		state := r.state
		r.mu.Unlock()
		return nil, &browserRuntimeUnavailableError{State: state}
	}
	r.state = "starting"
	r.launches++
	r.mu.Unlock()

	reportProgress(ctx, "Starting the reusable browser process.")
	result := make(chan browserStartResult, 1)
	go func() {
		process, err := startBrowserProcess(r.factory)
		result <- browserStartResult{process: process, err: err}
	}()

	select {
	case started := <-result:
		if started.err != nil {
			r.setCold("browser startup failed")
			return nil, started.err
		}
		if err := ctx.Err(); err != nil {
			r.setDegraded("browser startup completed after cancellation")
			go r.closeAbandonedProcess(started.process)
			return nil, err
		}

		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			go closeBrowserProcess(started.process)
			return nil, &browserRuntimeUnavailableError{State: "closed"}
		}
		r.current = started.process
		r.state = "ready"
		r.lastFailure = ""
		r.mu.Unlock()
		reportProgress(ctx, "Reusable browser process is ready.")
		return started.process, nil
	case <-ctx.Done():
		r.setDegraded("browser startup exceeded its deadline")
		go func() {
			started := <-result
			if started.process != nil {
				r.closeAbandonedProcess(started.process)
				return
			}
			r.setCold("browser startup failed after cancellation")
		}()
		return nil, ctx.Err()
	}
}

func (r *browserRuntime) newPage(
	ctx context.Context,
	process browserProcess,
) (*rod.Page, error) {
	result := make(chan browserPageResult, 1)
	go func() {
		page, err := newBrowserPage(process)
		result <- browserPageResult{page: page, err: err}
	}()

	select {
	case created := <-result:
		if created.err != nil {
			r.resetAsync(process, "browser page creation failed")
			return nil, created.err
		}
		return created.page, nil
	case <-ctx.Done():
		r.resetAsync(process, "browser page creation exceeded its deadline")
		go func() {
			created := <-result
			if created.page != nil {
				_ = closeRodPage(created.page)
			}
		}()
		return nil, ctx.Err()
	}
}

func (r *browserRuntime) Reset(reason string) {
	r.mu.Lock()
	process := r.current
	r.mu.Unlock()
	if process != nil {
		r.resetAsync(process, reason)
	}
}

func (r *browserRuntime) ResetAndWait(ctx context.Context, reason string) error {
	r.mu.Lock()
	process := r.current
	if process == nil {
		state := r.state
		r.mu.Unlock()
		if state == "cold" {
			return nil
		}
		return &browserRuntimeUnavailableError{State: state}
	}
	r.current = nil
	r.state = "resetting"
	r.lastFailure = reason
	r.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		done <- closeBrowserProcess(process)
	}()

	select {
	case err := <-done:
		if err != nil {
			r.setCold("browser cleanup failed")
			return nil
		}
		r.setCold(reason)
		return nil
	case <-ctx.Done():
		r.setDegraded("browser cleanup is stuck")
		go func() {
			<-done
			r.setCold(reason)
		}()
		return ctx.Err()
	}
}

func (r *browserRuntime) resetAsync(process browserProcess, reason string) {
	r.mu.Lock()
	if r.current != process {
		r.mu.Unlock()
		return
	}
	r.current = nil
	if r.closed {
		r.state = "closed"
	} else {
		r.state = "resetting"
	}
	r.lastFailure = reason
	r.mu.Unlock()

	go func() {
		done := make(chan error, 1)
		go func() {
			done <- closeBrowserProcess(process)
		}()

		timer := time.NewTimer(browserCloseWarning)
		defer timer.Stop()
		select {
		case err := <-done:
			if err != nil {
				r.setCold("browser cleanup failed")
				return
			}
			r.setCold(reason)
		case <-timer.C:
			r.setDegraded("browser cleanup is stuck")
			<-done
			r.setCold(reason)
		}
	}()
}

func (r *browserRuntime) closeAbandonedProcess(process browserProcess) {
	if err := closeBrowserProcess(process); err != nil {
		r.setCold("abandoned browser cleanup failed")
		return
	}
	r.setCold("abandoned browser startup cleaned up")
}

func (r *browserRuntime) Close(ctx context.Context) error {
	r.mu.Lock()
	r.closed = true
	process := r.current
	r.current = nil
	r.state = "closed"
	r.mu.Unlock()

	if process == nil {
		return nil
	}

	done := make(chan error, 1)
	go func() {
		done <- closeBrowserProcess(process)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *browserRuntime) Snapshot() browserRuntimeSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return browserRuntimeSnapshot{
		State:       r.state,
		Launches:    r.launches,
		LastFailure: r.lastFailure,
	}
}

func (r *browserRuntime) setCold(lastFailure string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		r.state = "closed"
		return
	}
	r.state = "cold"
	r.lastFailure = lastFailure
}

func (r *browserRuntime) setDegraded(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		r.state = "closed"
		return
	}
	r.state = "degraded"
	r.lastFailure = reason
}

type browserStartResult struct {
	process browserProcess
	err     error
}

type browserPageResult struct {
	page *rod.Page
	err  error
}

func startBrowserProcess(factory browserProcessFactory) (process browserProcess, err error) {
	err = runBrowserStage("startup", func() error {
		process = factory()
		if process == nil {
			return fmt.Errorf("browser factory returned nil")
		}
		return nil
	})
	return process, err
}

func newBrowserPage(process browserProcess) (page *rod.Page, err error) {
	err = runBrowserStage("page creation", func() error {
		page = process.NewPage()
		if page == nil {
			return nil
		}
		return nil
	})
	return page, err
}

func closeBrowserProcess(process browserProcess) error {
	if process == nil {
		return nil
	}
	return runBrowserStage("cleanup", func() error {
		process.Close()
		return nil
	})
}

func closeRodPage(page *rod.Page) error {
	if page == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), browserCloseWarning)
	defer cancel()
	return page.Context(ctx).Close()
}

func runBrowserStage(stage string, fn func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logrus.WithFields(logrus.Fields{
				"stage":      stage,
				"panic_type": fmt.Sprintf("%T", recovered),
			}).Error("Browser stage panicked")
			logrus.Errorf("Stack trace:\n%s", debug.Stack())
			err = &browserStagePanicError{Stage: stage}
		}
	}()
	return fn()
}

func shouldResetBrowser(err error) bool {
	if err == nil {
		return false
	}
	var panicErr *browserStagePanicError
	if errors.As(err, &panicErr) {
		return true
	}

	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"browser has been closed",
		"connection closed",
		"connection reset",
		"execution context was destroyed",
		"page crashed",
		"session closed",
		"target closed",
		"websocket",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func withRuntimePage[T any](
	runtime *browserRuntime,
	ctx context.Context,
	fn func(*rod.Page) (T, error),
) (T, error) {
	var result T
	err := runtime.Run(ctx, func(page *rod.Page) error {
		var err error
		result, err = fn(page)
		return err
	})
	return result, err
}

func openEphemeralBrowserPage(
	ctx context.Context,
	factory browserProcessFactory,
) (browserProcess, *rod.Page, error) {
	started := make(chan browserStartResult, 1)
	go func() {
		process, err := startBrowserProcess(factory)
		started <- browserStartResult{process: process, err: err}
	}()

	var process browserProcess
	select {
	case result := <-started:
		if result.err != nil {
			return nil, nil, result.err
		}
		process = result.process
	case <-ctx.Done():
		go func() {
			result := <-started
			if result.process != nil {
				_ = closeBrowserProcess(result.process)
			}
		}()
		return nil, nil, ctx.Err()
	}

	created := make(chan browserPageResult, 1)
	go func() {
		page, err := newBrowserPage(process)
		created <- browserPageResult{page: page, err: err}
	}()

	select {
	case result := <-created:
		if result.err != nil {
			_ = closeBrowserProcess(process)
			return nil, nil, result.err
		}
		return process, result.page, nil
	case <-ctx.Done():
		go func() {
			result := <-created
			if result.page != nil {
				_ = closeRodPage(result.page)
			}
			_ = closeBrowserProcess(process)
		}()
		return nil, nil, ctx.Err()
	}
}

func closeEphemeralBrowserPage(process browserProcess, page *rod.Page) {
	if err := closeRodPage(page); err != nil {
		logrus.WithField("error_type", fmt.Sprintf("%T", err)).
			Warn("Failed to close browser page")
	}

	done := make(chan error, 1)
	go func() {
		done <- closeBrowserProcess(process)
	}()

	timer := time.NewTimer(browserCloseWarning)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			logrus.WithField("error_type", fmt.Sprintf("%T", err)).
				Warn("Failed to close browser process")
		}
	case <-timer.C:
		logrus.Warn("Browser process cleanup is still running")
	}
}
