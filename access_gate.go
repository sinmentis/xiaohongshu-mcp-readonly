package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/sirupsen/logrus"
)

// AccessPolicy controls single-account pacing and per-request comment limits.
type AccessPolicy struct {
	MinInterval time.Duration
	MaxJitter   time.Duration
	MaxComments int
	MaxReplies  int
}

func DefaultAccessPolicy() AccessPolicy {
	return AccessPolicy{
		MinInterval: 30 * time.Second,
		MaxJitter:   15 * time.Second,
		MaxComments: 50,
		MaxReplies:  10,
	}
}

func (p AccessPolicy) Validate() error {
	if p.MinInterval < 0 {
		return fmt.Errorf("minimum access interval cannot be negative")
	}
	if p.MaxJitter < 0 {
		return fmt.Errorf("maximum jitter cannot be negative")
	}
	if p.MaxComments <= 0 {
		return fmt.Errorf("maximum comments must be greater than zero")
	}
	if p.MaxReplies <= 0 {
		return fmt.Errorf("maximum replies must be greater than zero")
	}
	return nil
}

// accessGate serializes operations and adds a cooldown between completions.
type accessGate struct {
	// A channel semaphore keeps queued callers responsive to cancellation.
	slot         chan struct{}
	minInterval  time.Duration
	maxJitter    time.Duration
	lastFinished time.Time
}

func newAccessGate(minInterval, maxJitter time.Duration) *accessGate {
	return &accessGate{
		slot:        make(chan struct{}, 1),
		minInterval: minInterval,
		maxJitter:   maxJitter,
	}
}

func (g *accessGate) Run(ctx context.Context, operation string, fn func() error) error {
	select {
	case g.slot <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-g.slot }()

	// A request cancelled while queued must not run after acquiring the slot.
	if err := ctx.Err(); err != nil {
		return err
	}

	if !g.lastFinished.IsZero() {
		jitter := time.Duration(0)
		if g.maxJitter > 0 {
			jitter = time.Duration(rand.Int63n(int64(g.maxJitter)))
		}

		wait := time.Until(g.lastFinished.Add(g.minInterval + jitter))
		if wait > 0 {
			logrus.Infof("Access gate: %s will wait %s", operation, wait.Round(time.Second))
			timer := time.NewTimer(wait)
			defer timer.Stop()

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}
	}

	defer func() {
		g.lastFinished = time.Now()
	}()

	return fn()
}

func withReadAccess[T any](
	service *XiaohongshuService,
	ctx context.Context,
	operation string,
	fn func() (T, error),
) (T, error) {
	var result T

	err := service.accessGate.Run(ctx, operation, func() error {
		var err error
		result, err = fn()
		return err
	})

	return result, err
}
