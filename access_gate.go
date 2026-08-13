package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"runtime/debug"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	defaultMaxQueueWait       = time.Minute
	progressHeartbeatInterval = 15 * time.Second
	defaultCancellationGrace  = 5 * time.Second
)

// AccessPolicy controls single-account pacing and per-request comment limits.
type AccessPolicy struct {
	MinInterval  time.Duration
	MaxJitter    time.Duration
	MaxQueueWait time.Duration
	MaxComments  int
	MaxReplies   int
}

func DefaultAccessPolicy() AccessPolicy {
	return AccessPolicy{
		MinInterval:  30 * time.Second,
		MaxJitter:    15 * time.Second,
		MaxQueueWait: defaultMaxQueueWait,
		MaxComments:  50,
		MaxReplies:   10,
	}
}

func (p AccessPolicy) Validate() error {
	if p.MinInterval < 0 {
		return fmt.Errorf("minimum access interval cannot be negative")
	}
	if p.MaxJitter < 0 {
		return fmt.Errorf("maximum jitter cannot be negative")
	}
	if p.MaxQueueWait <= 0 {
		return fmt.Errorf("maximum queue wait must be greater than zero")
	}
	if p.MaxComments <= 0 {
		return fmt.Errorf("maximum comments must be greater than zero")
	}
	if p.MaxReplies <= 0 {
		return fmt.Errorf("maximum replies must be greater than zero")
	}
	return nil
}

type accessOperation struct {
	ID           uint64
	Name         string
	Phase        string
	AcquiredAt   time.Time
	StartedAt    time.Time
	Deadline     time.Time
	CancelReason string
}

// AccessGateSnapshot is a secret-free operational view for health reporting.
type AccessGateSnapshot struct {
	State           string
	OperationID     uint64
	Operation       string
	Phase           string
	AcquiredAt      time.Time
	StartedAt       time.Time
	Deadline        time.Time
	Queued          int
	LastFinished    time.Time
	LastOperationID uint64
	LastOperation   string
	LastDuration    time.Duration
	LastOutcome     string
}

type operationTimeoutError struct {
	Operation   string
	OperationID uint64
	Timeout     time.Duration
}

func (e *operationTimeoutError) Error() string {
	return fmt.Sprintf(
		"%s timed out after %s (operation %d); inspect /health before retrying",
		e.Operation,
		e.Timeout.Round(time.Second),
		e.OperationID,
	)
}

func (e *operationTimeoutError) Unwrap() error {
	return context.DeadlineExceeded
}

type operationQueueTimeoutError struct {
	Operation string
	Wait      time.Duration
	Active    string
}

func (e *operationQueueTimeoutError) Error() string {
	if e.Active == "" {
		return fmt.Sprintf(
			"%s could not start within %s; inspect /health before retrying",
			e.Operation,
			e.Wait.Round(time.Second),
		)
	}
	return fmt.Sprintf(
		"%s could not start within %s because %s is still using the browser; inspect /health before retrying",
		e.Operation,
		e.Wait.Round(time.Second),
		e.Active,
	)
}

func (e *operationQueueTimeoutError) Unwrap() error {
	return context.DeadlineExceeded
}

type accessGateUnavailableError struct {
	Operation   string
	OperationID uint64
}

func (e *accessGateUnavailableError) Error() string {
	if e.Operation == "" || e.OperationID == 0 {
		return "browser access is temporarily unavailable because the previous operation is still stopping after cancellation; inspect /health or restart the service"
	}
	return fmt.Sprintf(
		"browser access is temporarily unavailable because %s (operation %d) is still stopping after cancellation; inspect /health or restart the service",
		e.Operation,
		e.OperationID,
	)
}

// accessGate serializes operations, applies pacing, and exposes stuck work.
type accessGate struct {
	slot              chan struct{}
	minInterval       time.Duration
	maxJitter         time.Duration
	maxQueueWait      time.Duration
	cancellationGrace time.Duration

	mu              sync.Mutex
	nextID          uint64
	queued          int
	active          *accessOperation
	unavailable     chan struct{}
	lastFinished    time.Time
	lastOperationID uint64
	lastOperation   string
	lastDuration    time.Duration
	lastOutcome     string
}

func newAccessGate(minInterval, maxJitter, maxQueueWait time.Duration) *accessGate {
	return &accessGate{
		slot:              make(chan struct{}, 1),
		minInterval:       minInterval,
		maxJitter:         maxJitter,
		maxQueueWait:      maxQueueWait,
		cancellationGrace: defaultCancellationGrace,
		unavailable:       make(chan struct{}),
	}
}

func (g *accessGate) Run(
	ctx context.Context,
	operation string,
	timeout time.Duration,
	fn func(context.Context) error,
) error {
	return g.run(ctx, operation, g.maxQueueWait, timeout, fn)
}

func (g *accessGate) RunWithQueueWait(
	ctx context.Context,
	operation string,
	queueWait time.Duration,
	timeout time.Duration,
	fn func(context.Context) error,
) error {
	return g.run(ctx, operation, queueWait, timeout, fn)
}

func (g *accessGate) run(
	ctx context.Context,
	operation string,
	queueWait time.Duration,
	timeout time.Duration,
	fn func(context.Context) error,
) error {
	if timeout <= 0 {
		return fmt.Errorf("%s operation timeout must be greater than zero", operation)
	}
	if queueWait <= 0 {
		return fmt.Errorf("%s queue timeout must be greater than zero", operation)
	}

	if snapshot := g.Snapshot(); snapshot.Phase == "cancelling" {
		return &accessGateUnavailableError{
			Operation:   snapshot.Operation,
			OperationID: snapshot.OperationID,
		}
	}

	g.changeQueued(1)
	if snapshot := g.Snapshot(); snapshot.Operation != "" {
		reportProgress(ctx, fmt.Sprintf(
			"%s is queued behind %s (operation %d).",
			operation,
			snapshot.Operation,
			snapshot.OperationID,
		))
	}

	queueCtx, cancelQueue := context.WithTimeout(ctx, queueWait)
	defer cancelQueue()

	for {
		unavailable := g.unavailableSignal()
		select {
		case g.slot <- struct{}{}:
			g.changeQueued(-1)
			if err := ctx.Err(); err != nil {
				<-g.slot
				return err
			}
			goto acquired
		case <-unavailable:
			snapshot := g.Snapshot()
			if snapshot.Phase != "cancelling" {
				continue
			}
			g.changeQueued(-1)
			return unavailableError(snapshot)
		case <-queueCtx.Done():
			g.changeQueued(-1)
			if err := ctx.Err(); err != nil {
				return err
			}
			snapshot := g.Snapshot()
			return &operationQueueTimeoutError{
				Operation: operation,
				Wait:      queueWait,
				Active:    snapshot.Operation,
			}
		}
	}

acquired:
	operationID, cooldown := g.begin(operation)
	if cooldown > 0 {
		logrus.WithFields(logrus.Fields{
			"operation":    operation,
			"operation_id": operationID,
			"phase":        "cooldown",
			"wait":         cooldown.Round(time.Second),
		}).Info("Browser access delayed")
		reportProgress(ctx, fmt.Sprintf(
			"%s is waiting %s for the access cooldown.",
			operation,
			cooldown.Round(time.Second),
		))

		timer := time.NewTimer(cooldown)
		select {
		case <-ctx.Done():
			timer.Stop()
			g.cancelBeforeRun(operationID)
			<-g.slot
			return ctx.Err()
		case <-timer.C:
		}
	}

	operationCtx, cancelOperation := context.WithTimeout(ctx, timeout)
	defer cancelOperation()

	deadline, _ := operationCtx.Deadline()
	effectiveTimeout := time.Until(deadline)
	if effectiveTimeout < 0 {
		effectiveTimeout = 0
	}
	startedAt := time.Now()
	g.start(operationID, startedAt, deadline)
	logrus.WithFields(logrus.Fields{
		"operation":    operation,
		"operation_id": operationID,
		"phase":        "running",
		"timeout":      effectiveTimeout,
	}).Info("Browser operation started")
	reportProgress(operationCtx, fmt.Sprintf(
		"%s started with a %s effective deadline.",
		operation,
		effectiveTimeout.Round(time.Second),
	))

	result := make(chan error, 1)
	go func() {
		err := runAccessOperation(operation, operationID, operationCtx, fn)
		g.finish(operation, operationID, startedAt, err)
		<-g.slot
		result <- err
	}()

	heartbeat := time.NewTicker(progressHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case err := <-result:
			return normalizeOperationResult(
				operation,
				operationID,
				effectiveTimeout,
				operationCtx,
				err,
			)
		case <-operationCtx.Done():
			select {
			case err := <-result:
				return normalizeOperationResult(
					operation,
					operationID,
					effectiveTimeout,
					operationCtx,
					err,
				)
			default:
			}

			if errors.Is(operationCtx.Err(), context.Canceled) {
				g.escalateCancellation(operationID)
				return context.Canceled
			}
			g.markCancelling(operationID, "timed_out")
			reportProgress(operationCtx, fmt.Sprintf(
				"%s reached its deadline and is being cancelled.",
				operation,
			))
			return &operationTimeoutError{
				Operation:   operation,
				OperationID: operationID,
				Timeout:     effectiveTimeout,
			}
		case <-heartbeat.C:
			elapsed := time.Since(startedAt).Round(time.Second)
			remaining := time.Until(deadline).Round(time.Second)
			if remaining < 0 {
				remaining = 0
			}
			reportProgress(operationCtx, fmt.Sprintf(
				"%s is still running (%s elapsed, %s remaining).",
				operation,
				elapsed,
				remaining,
			))
		}
	}
}

func runAccessOperation(
	operation string,
	operationID uint64,
	ctx context.Context,
	fn func(context.Context) error,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logrus.WithFields(logrus.Fields{
				"operation":    operation,
				"operation_id": operationID,
				"panic_type":   fmt.Sprintf("%T", recovered),
			}).Error("Browser operation panicked")
			logrus.Errorf("Stack trace:\n%s", debug.Stack())
			err = fmt.Errorf("%s failed internally; check server logs", operation)
		}
	}()
	return fn(ctx)
}

func (g *accessGate) begin(operation string) (uint64, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.nextID++
	operationID := g.nextID

	jitter := time.Duration(0)
	if g.maxJitter > 0 {
		jitter = time.Duration(rand.Int63n(int64(g.maxJitter)))
	}

	cooldown := time.Duration(0)
	if !g.lastFinished.IsZero() {
		cooldown = time.Until(g.lastFinished.Add(g.minInterval + jitter))
		if cooldown < 0 {
			cooldown = 0
		}
	}

	acquiredAt := time.Now()
	g.active = &accessOperation{
		ID:         operationID,
		Name:       operation,
		Phase:      "cooldown",
		AcquiredAt: acquiredAt,
		StartedAt:  acquiredAt,
		Deadline:   acquiredAt.Add(cooldown),
	}
	return operationID, cooldown
}

func (g *accessGate) start(operationID uint64, startedAt, deadline time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active == nil || g.active.ID != operationID {
		return
	}
	g.active.Phase = "running"
	g.active.StartedAt = startedAt
	g.active.Deadline = deadline
}

func (g *accessGate) markCancelling(operationID uint64, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active == nil || g.active.ID != operationID {
		return
	}
	if g.active.Phase == "cancelling" {
		return
	}
	g.active.Phase = "cancelling"
	g.active.CancelReason = reason
	close(g.unavailable)
}

func (g *accessGate) escalateCancellation(operationID uint64) {
	go func() {
		timer := time.NewTimer(g.cancellationGrace)
		defer timer.Stop()
		<-timer.C
		g.markCancelling(operationID, "canceled")
	}()
}

func (g *accessGate) cancelBeforeRun(operationID uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active != nil && g.active.ID == operationID {
		g.active = nil
	}
}

func (g *accessGate) finish(
	operation string,
	operationID uint64,
	startedAt time.Time,
	err error,
) {
	finishedAt := time.Now()
	outcome := "success"
	if err != nil {
		outcome = "error"
	}

	g.mu.Lock()
	wasCancelling := false
	if g.active != nil && g.active.ID == operationID {
		wasCancelling = g.active.Phase == "cancelling"
		if g.active.CancelReason != "" {
			outcome = g.active.CancelReason
		}
		g.active = nil
	}
	if wasCancelling {
		g.unavailable = make(chan struct{})
	}
	g.lastFinished = finishedAt
	g.lastOperationID = operationID
	g.lastOperation = operation
	g.lastDuration = finishedAt.Sub(startedAt)
	g.lastOutcome = outcome
	g.mu.Unlock()

	fields := logrus.Fields{
		"operation":    operation,
		"operation_id": operationID,
		"duration":     finishedAt.Sub(startedAt),
		"outcome":      outcome,
	}
	if err != nil {
		fields["error_type"] = fmt.Sprintf("%T", err)
		logrus.WithFields(fields).Warn("Browser operation finished")
		return
	}
	logrus.WithFields(fields).Info("Browser operation finished")
}

func (g *accessGate) changeQueued(delta int) {
	g.mu.Lock()
	g.queued += delta
	g.mu.Unlock()
}

func (g *accessGate) unavailableSignal() <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.unavailable
}

func (g *accessGate) Snapshot() AccessGateSnapshot {
	g.mu.Lock()
	defer g.mu.Unlock()

	snapshot := AccessGateSnapshot{
		State:           "idle",
		Queued:          g.queued,
		LastFinished:    g.lastFinished,
		LastOperationID: g.lastOperationID,
		LastOperation:   g.lastOperation,
		LastDuration:    g.lastDuration,
		LastOutcome:     g.lastOutcome,
	}
	if g.active == nil {
		return snapshot
	}

	snapshot.State = "busy"
	if g.active.Phase == "cancelling" {
		snapshot.State = "degraded"
	}
	snapshot.OperationID = g.active.ID
	snapshot.Operation = g.active.Name
	snapshot.Phase = g.active.Phase
	snapshot.AcquiredAt = g.active.AcquiredAt
	snapshot.StartedAt = g.active.StartedAt
	snapshot.Deadline = g.active.Deadline
	return snapshot
}

func unavailableError(snapshot AccessGateSnapshot) error {
	return &accessGateUnavailableError{
		Operation:   snapshot.Operation,
		OperationID: snapshot.OperationID,
	}
}

func normalizeOperationResult(
	operation string,
	operationID uint64,
	timeout time.Duration,
	operationCtx context.Context,
	err error,
) error {
	if err == nil {
		return nil
	}
	if errors.Is(operationCtx.Err(), context.DeadlineExceeded) &&
		(errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, context.Canceled)) {
		return &operationTimeoutError{
			Operation:   operation,
			OperationID: operationID,
			Timeout:     timeout,
		}
	}
	return err
}

func withReadAccess[T any](
	service *XiaohongshuService,
	ctx context.Context,
	operation string,
	timeout time.Duration,
	fn func(context.Context) (T, error),
) (T, error) {
	var result T

	err := service.accessGate.Run(ctx, operation, timeout, func(operationCtx context.Context) error {
		var err error
		result, err = fn(operationCtx)
		return err
	})

	return result, err
}

func withReadAccessQueue[T any](
	service *XiaohongshuService,
	ctx context.Context,
	operation string,
	queueWait time.Duration,
	timeout time.Duration,
	fn func(context.Context) (T, error),
) (T, error) {
	var result T

	err := service.accessGate.RunWithQueueWait(
		ctx,
		operation,
		queueWait,
		timeout,
		func(operationCtx context.Context) error {
			var err error
			result, err = fn(operationCtx)
			return err
		},
	)

	return result, err
}
