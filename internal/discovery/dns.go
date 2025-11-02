package discovery

import (
	"context"
	"net"
)

func lookupHost(ctx context.Context, name string) ([]string, error) {
	resolver := net.Resolver{}
	return resolver.LookupHost(ctx, name)
}
