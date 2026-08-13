package humanize

import (
	"context"
	"time"
)

func Delay(ctx context.Context, action Action) {
	dist, ok := defaultTiming[action]
	if !ok {
		dist = defaultTiming[AfterClick]
	}

	t := time.NewTimer(dist.Sample())
	defer t.Stop()

	select {
	case <-t.C:
	case <-ctx.Done():
	}
}
