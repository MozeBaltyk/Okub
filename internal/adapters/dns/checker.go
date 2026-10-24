package dns

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

// Checker queries the resolver configured by the cluster definition.
type Checker struct {
	Timeout time.Duration
}

func (c Checker) resolver(server string) *net.Resolver {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: timeout}).DialContext(ctx, network, net.JoinHostPort(server, "53"))
	}}
}

func (c Checker) LookupHost(ctx context.Context, server, name string) ([]netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	values, err := c.resolver(server).LookupHost(ctx, name)
	if err != nil {
		return nil, err
	}
	addresses := make([]netip.Addr, 0, len(values))
	for _, value := range values {
		address, err := netip.ParseAddr(value)
		if err != nil {
			return nil, fmt.Errorf("parse DNS answer: %w", err)
		}
		addresses = append(addresses, address.Unmap())
	}
	return addresses, nil
}

func (c Checker) LookupAddr(ctx context.Context, server string, address netip.Addr) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	values, err := c.resolver(server).LookupAddr(ctx, address.String())
	if err != nil {
		return nil, err
	}
	for i := range values {
		values[i] = strings.TrimSuffix(strings.ToLower(values[i]), ".")
	}
	return values, nil
}

func (c Checker) timeout() time.Duration {
	if c.Timeout != 0 {
		return c.Timeout
	}
	return 3 * time.Second
}
