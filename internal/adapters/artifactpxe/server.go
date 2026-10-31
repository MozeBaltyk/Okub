package artifactpxe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mozebaltyk/Okub/internal/ports"
	"go.universe.tf/netboot/out/ipxe"
	"go.universe.tf/netboot/pixiecore"
)

const Version = "go.universe.tf/netboot@v0.0.0-20260201190555-f5d248c4db46"

const efiAsset = "third_party/ipxe/src/bin-x86_64-efi/ipxe.efi"

type Server struct {
	HTTP            ports.ArtifactTransport
	ShutdownTimeout time.Duration
	newPXE          func(*booter, netip.Addr, ports.PXEPorts, func()) pxeServer
}

type pxeServer interface {
	Serve() error
	Shutdown()
}

func (s Server) Serve(ctx context.Context, set ports.PublishedArtifactSet, httpAddress netip.AddrPort, advertised netip.Addr, networkPorts ports.PXEPorts, macs []string, ready func(ports.PXEInfo) error) error {
	if s.HTTP == nil {
		return errors.New("HTTP artifact transport is not configured")
	}
	boot, err := newBooter(set, macs)
	if err != nil {
		return err
	}
	efi := ipxe.MustAsset(efiAsset)
	info := ports.PXEInfo{PixiecoreVersion: Version, IPXESHA256: "sha256:" + hashBytes(efi)}

	child, cancel := context.WithCancel(ctx)
	defer cancel()
	httpReady, pxeReady := make(chan struct{}, 1), make(chan struct{}, 1)
	httpDone, pxeDone := make(chan error, 1), make(chan error, 1)
	go func() {
		httpDone <- s.HTTP.Serve(child, set, httpAddress, func() error {
			httpReady <- struct{}{}
			return nil
		})
	}()
	factory := s.newPXE
	if factory == nil {
		factory = newPixiecore
	}
	pxe := factory(boot, advertised, networkPorts, func() { pxeReady <- struct{}{} })
	go func() { pxeDone <- pxe.Serve() }()

	var httpIsReady, pxeIsReady bool
	for !httpIsReady || !pxeIsReady {
		select {
		case <-httpReady:
			httpIsReady = true
		case <-pxeReady:
			pxeIsReady = true
		case err := <-httpDone:
			return s.stop(pxe, cancel, pxeDone, httpDone, false, true, fmt.Errorf("HTTP startup failed: %w", err))
		case err := <-pxeDone:
			return s.stop(pxe, cancel, pxeDone, httpDone, true, false, fmt.Errorf("Pixiecore startup failed: %w", err))
		case <-ctx.Done():
			return s.stop(pxe, cancel, pxeDone, httpDone, false, false, ctx.Err())
		}
	}
	if err := ready(info); err != nil {
		return s.stop(pxe, cancel, pxeDone, httpDone, false, false, err)
	}
	select {
	case err := <-httpDone:
		return s.stop(pxe, cancel, pxeDone, httpDone, false, true, fmt.Errorf("HTTP listener failed: %w", err))
	case err := <-pxeDone:
		return s.stop(pxe, cancel, pxeDone, httpDone, true, false, fmt.Errorf("Pixiecore listener failed: %w", err))
	case <-ctx.Done():
		return s.stop(pxe, cancel, pxeDone, httpDone, false, false, ctx.Err())
	}
}

func (s Server) stop(pxe pxeServer, cancel context.CancelFunc, pxeDone, httpDone <-chan error, pxeStopped, httpStopped bool, result error) error {
	cancel()
	pxe.Shutdown()
	timeout := s.ShutdownTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	deadline := time.NewTimer(timeout)
	retry := time.NewTicker(10 * time.Millisecond)
	defer deadline.Stop()
	defer retry.Stop()
	for !pxeStopped || !httpStopped {
		select {
		case <-pxeDone:
			pxeStopped = true
		case <-httpDone:
			httpStopped = true
		case <-retry.C:
			pxe.Shutdown()
		case <-deadline.C:
			return errors.Join(result, errors.New("PXE serving shutdown timed out"))
		}
	}
	return result
}

func newPixiecore(boot *booter, address netip.Addr, networkPorts ports.PXEPorts, ready func()) pxeServer {
	var once sync.Once
	return &pixiecore.Server{
		Booter: boot, Address: address.String(), HTTPPort: networkPorts.HTTP,
		DHCPPort: networkPorts.DHCP, TFTPPort: networkPorts.TFTP, PXEPort: networkPorts.PXE,
		DHCPNoBind: true,
		Ipxe: map[pixiecore.Firmware][]byte{
			pixiecore.FirmwareEFI64: ipxe.MustAsset(efiAsset),
			pixiecore.FirmwareEFIBC: ipxe.MustAsset(efiAsset),
		},
		Debug: func(subsystem, message string) {
			// The pinned Pixiecore emits this only after all four listeners bind.
			if subsystem == "Init" && message == "Starting Pixiecore goroutines" {
				once.Do(ready)
			}
		},
	}
}

type booter struct {
	allowed map[string]struct{}
	script  ports.PublishedArtifact
}

func newBooter(set ports.PublishedArtifactSet, macs []string) (*booter, error) {
	b := &booter{allowed: make(map[string]struct{}, len(macs))}
	for _, raw := range macs {
		mac, err := net.ParseMAC(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid admitted MAC address %q: %w", raw, err)
		}
		b.allowed[mac.String()] = struct{}{}
	}
	for _, artifact := range set.Artifacts {
		if pxeScriptPath(artifact.Path) {
			b.script = artifact
			return b, nil
		}
	}
	return nil, errors.New("PXE artifact set is missing an architecture-specific iPXE script")
}

func pxeScriptPath(path string) bool {
	const prefix, suffix = "artifacts/pxe/agent.", ".ipxe"
	return strings.HasPrefix(path, prefix) && strings.HasSuffix(path, suffix) && len(path) > len(prefix)+len(suffix)
}

func (b *booter) BootSpec(machine pixiecore.Machine) (*pixiecore.Spec, error) {
	if machine.Arch != pixiecore.ArchX64 {
		return nil, nil
	}
	mac, err := net.ParseMAC(machine.MAC.String())
	if err != nil {
		return nil, nil
	}
	if _, ok := b.allowed[mac.String()]; !ok {
		return nil, nil
	}
	script, err := readVerified(b.script)
	if err != nil {
		return nil, fmt.Errorf("read authoritative iPXE script: %w", err)
	}
	return &pixiecore.Spec{IpxeScript: string(script)}, nil
}

func (*booter) ReadBootFile(pixiecore.ID) (io.ReadCloser, int64, error) {
	return nil, 0, errors.New("boot files are served by the verified HTTP artifact transport")
}

func (*booter) WriteBootFile(pixiecore.ID, io.Reader) error {
	return errors.New("boot artifacts are read-only")
}

func readVerified(artifact ports.PublishedArtifact) ([]byte, error) {
	info, err := os.Lstat(artifact.FilePath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != artifact.Size {
		return nil, errors.New("artifact changed")
	}
	data, err := os.ReadFile(artifact.FilePath)
	if err != nil || "sha256:"+hashBytes(data) != artifact.SHA256 {
		return nil, errors.New("artifact changed")
	}
	return data, nil
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
