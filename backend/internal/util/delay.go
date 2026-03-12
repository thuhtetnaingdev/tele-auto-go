package util

import (
	"context"
	"math/rand"
	"time"
)

func WaitRandomDelay(minMS, maxMS int) time.Duration {
	if maxMS < minMS {
		maxMS = minMS
	}
	n := rand.Intn(maxMS-minMS+1) + minMS
	d := time.Duration(n) * time.Millisecond
	time.Sleep(d)
	return d
}

func WaitRandomDelayContext(ctx context.Context, minMS, maxMS int) (time.Duration, error) {
	if maxMS < minMS {
		maxMS = minMS
	}
	n := rand.Intn(maxMS-minMS+1) + minMS
	d := time.Duration(n) * time.Millisecond
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-t.C:
		return d, nil
	}
}
