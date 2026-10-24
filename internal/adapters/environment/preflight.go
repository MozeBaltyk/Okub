package environment

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mozebaltyk/Okub/internal/adapters/filesystem"
	"golang.org/x/sys/unix"
)

// Local performs bounded, read-only observations of the local environment.
type Local struct {
	Timeout time.Duration
}

func (l Local) timeout() time.Duration {
	if l.Timeout != 0 {
		return l.Timeout
	}
	return 4 * time.Second
}

func (l Local) ReadCredential(path string) ([]byte, error) {
	return (filesystem.ConfigurationSource{}).ReadCredential(path)
}

func (l Local) DirectoryWritable(path string) error {
	current := filepath.Clean(path)
	for {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return errors.New("path or its existing parent is not a directory")
			}
			if unix.Access(current, unix.W_OK) != nil {
				return errors.New("existing directory is not writable")
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return errors.New("no existing parent directory")
		}
		current = parent
	}
}

func (l Local) ToolVersion(ctx context.Context, name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, l.timeout())
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("version command failed: %w", err)
	}
	if len(output) > 4096 {
		output = output[:4096]
	}
	return string(output), nil
}

func (l Local) AddressInUse(ctx context.Context, address netip.Addr, ports []int) (bool, error) {
	interfaces, err := net.InterfaceAddrs()
	if err != nil {
		return false, err
	}
	for _, item := range interfaces {
		prefix, err := netip.ParsePrefix(item.String())
		if err == nil && prefix.Addr().Unmap() == address.Unmap() {
			return true, nil
		}
	}
	for _, port := range ports {
		probe, cancel := context.WithTimeout(ctx, l.timeout())
		connection, err := (&net.Dialer{}).DialContext(probe, "tcp", net.JoinHostPort(address.String(), fmt.Sprint(port)))
		cancel()
		if err == nil {
			connection.Close()
			return true, nil
		}
	}
	return false, nil
}

func (l Local) TCPReachable(ctx context.Context, address netip.Addr, port int) error {
	probe, cancel := context.WithTimeout(ctx, l.timeout())
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(probe, "tcp", net.JoinHostPort(address.String(), fmt.Sprint(port)))
	if err != nil {
		return err
	}
	_ = connection.Close()
	return nil
}

func (l Local) RouteAvailable(ctx context.Context, address netip.Addr) error {
	probe, cancel := context.WithTimeout(ctx, l.timeout())
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(probe, "udp", net.JoinHostPort(address.String(), "9"))
	if err == nil {
		connection.Close()
	}
	return err
}

func (l Local) EndpointReachable(ctx context.Context, endpoint string) error {
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		probe, cancel := context.WithTimeout(ctx, l.timeout())
		request, err := http.NewRequestWithContext(probe, method, endpoint, nil)
		if err != nil {
			cancel()
			return err
		}
		if method == http.MethodGet {
			request.Header.Set("Range", "bytes=0-0")
		}
		response, err := (&http.Client{Timeout: l.timeout()}).Do(request)
		cancel()
		if err != nil {
			return err
		}
		response.Body.Close()
		if response.StatusCode < 400 || response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			return nil
		}
		if method == http.MethodGet {
			return fmt.Errorf("endpoint returned HTTP %d", response.StatusCode)
		}
	}
	return errors.New("endpoint is unreachable")
}
