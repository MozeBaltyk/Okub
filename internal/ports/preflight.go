package ports

import (
	"context"
	"net/netip"
)

// DNSChecker sends queries to one explicitly selected resolver.
type DNSChecker interface {
	LookupHost(context.Context, string, string) ([]netip.Addr, error)
	LookupAddr(context.Context, string, netip.Addr) ([]string, error)
}

// PreflightEnvironment exposes read-only host observations.
type PreflightEnvironment interface {
	ReadCredential(string) ([]byte, error)
	DirectoryWritable(string) error
	ToolVersion(context.Context, string) (string, error)
	AddressInUse(context.Context, netip.Addr, []int) (bool, error)
	TCPReachable(context.Context, netip.Addr, int) error
	RouteAvailable(context.Context, netip.Addr) error
	EndpointReachable(context.Context, string) error
}
