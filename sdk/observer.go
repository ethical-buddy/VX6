package sdk

import (
	"context"
	"time"

	"github.com/vx6/vx6/internal/config"
	"github.com/vx6/vx6/internal/runtimectl"
)

type StatusObserverOptions struct {
	Interval time.Duration
	OnStatus func(runtimectl.Status)
	OnError  func(error)
}

// ObserveStatus polls local VX6 runtime status over the control channel until ctx is cancelled.
func (c *Client) ObserveStatus(ctx context.Context, opts StatusObserverOptions) error {
	if opts.Interval <= 0 {
		opts.Interval = 2 * time.Second
	}
	store, err := config.NewStore(c.store.Path())
	if err != nil {
		return err
	}
	controlPath, err := config.RuntimeControlPath(store.Path())
	if err != nil {
		return err
	}

	t := time.NewTicker(opts.Interval)
	defer t.Stop()

	poll := func() {
		status, err := runtimectl.RequestStatus(ctx, controlPath)
		if err != nil {
			if opts.OnError != nil {
				opts.OnError(err)
			}
			return
		}
		if opts.OnStatus != nil {
			opts.OnStatus(status)
		}
	}

	poll()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			poll()
		}
	}
}
