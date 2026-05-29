package cli

import (
	"context"

	"github.com/hackebrot/opportunities/internal/service"
)

// clockKey carries a service.Clock through the command context. main
// seeds a real clock (it is forbidigo-exempt and may call time.Now());
// the service layer must not call time.Now() directly.
type clockKey struct{}

// WithClock returns a context carrying c, read by openServiceFromConfig
// when constructing a service.Service.
func WithClock(ctx context.Context, c service.Clock) context.Context {
	return context.WithValue(ctx, clockKey{}, c)
}

// clockFrom returns the clock carried by ctx, or nil if none was set.
func clockFrom(ctx context.Context) service.Clock {
	c, _ := ctx.Value(clockKey{}).(service.Clock)
	return c
}
