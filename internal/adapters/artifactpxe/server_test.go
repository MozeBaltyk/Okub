package artifactpxe

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
	"go.universe.tf/netboot/pixiecore"
)

func TestBooterAdmissionArchitectureAndIdempotence(t *testing.T) {
	set := scriptSet(t, "#!ipxe\nkernel authoritative\n")
	boot, err := newBooter(set, []string{"52-54-00-AA-BB-CC"})
	if err != nil {
		t.Fatal(err)
	}
	allowed := pixiecore.Machine{MAC: mustMAC(t, "52:54:00:aa:bb:cc"), Arch: pixiecore.ArchX64}
	for i := 0; i < 2; i++ {
		spec, err := boot.BootSpec(allowed)
		if err != nil || spec == nil || spec.IpxeScript != "#!ipxe\nkernel authoritative\n" || spec.Cmdline != "" {
			t.Fatalf("request %d: spec=%#v error=%v", i, spec, err)
		}
	}
	for _, machine := range []pixiecore.Machine{
		{MAC: mustMAC(t, "52:54:00:00:00:01"), Arch: pixiecore.ArchX64},
		{MAC: allowed.MAC, Arch: pixiecore.ArchIA32},
	} {
		spec, err := boot.BootSpec(machine)
		if err != nil || spec != nil {
			t.Fatalf("rejected machine got spec=%#v error=%v", spec, err)
		}
	}
}

func TestBooterRejectsChangedAuthoritativeScript(t *testing.T) {
	set := scriptSet(t, "#!ipxe\n")
	boot, err := newBooter(set, []string{"52:54:00:aa:bb:cc"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(set.Artifacts[0].FilePath, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if spec, err := boot.BootSpec(pixiecore.Machine{MAC: mustMAC(t, "52:54:00:aa:bb:cc"), Arch: pixiecore.ArchX64}); err == nil || spec != nil {
		t.Fatalf("spec=%#v error=%v", spec, err)
	}
}

func TestPixiecoreEmbedsOnlyX8664UEFIBootloader(t *testing.T) {
	server := newPixiecore(&booter{}, netip.MustParseAddr("192.0.2.10"), ports.PXEPorts{DHCP: 1067, TFTP: 1069, PXE: 14011, HTTP: 18081}, func() {}).(*pixiecore.Server)
	if len(server.Ipxe) != 2 || len(server.Ipxe[pixiecore.FirmwareEFI64]) == 0 || len(server.Ipxe[pixiecore.FirmwareEFIBC]) == 0 {
		t.Fatalf("firmware bootloaders=%v", server.Ipxe)
	}
	if server.Ipxe[pixiecore.FirmwareX86PC] != nil || server.Ipxe[pixiecore.FirmwareEFI32] != nil || server.Ipxe[pixiecore.FirmwareX86Ipxe] != nil {
		t.Fatalf("unsupported firmware has a bootloader: %v", server.Ipxe)
	}
}

func TestServerWaitsForBothListenersAndStopsAsOneUnit(t *testing.T) {
	for _, first := range []string{"http", "pxe"} {
		t.Run(first, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			http := &fakeHTTP{ready: make(chan struct{})}
			pxe := &fakePXE{ready: make(chan struct{}), stopped: make(chan struct{})}
			var factoryAddress netip.Addr
			server := Server{HTTP: http, ShutdownTimeout: time.Second, newPXE: func(_ *booter, address netip.Addr, _ ports.PXEPorts, ready func()) pxeServer {
				factoryAddress = address
				pxe.onReady = ready
				return pxe
			}}
			ready := make(chan struct{}, 1)
			done := make(chan error, 1)
			set := scriptSet(t, "#!ipxe\n")
			go func() {
				done <- server.Serve(ctx, set, netip.MustParseAddrPort("127.0.0.1:18080"), netip.MustParseAddr("192.0.2.10"), ports.PXEPorts{DHCP: 1067, TFTP: 1069, PXE: 14011, HTTP: 18081}, []string{"52:54:00:aa:bb:cc"}, func(ports.PXEInfo) error { ready <- struct{}{}; return nil })
			}()
			if first == "http" {
				close(http.ready)
				assertNotReady(t, ready)
				close(pxe.ready)
			} else {
				close(pxe.ready)
				assertNotReady(t, ready)
				close(http.ready)
			}
			select {
			case <-ready:
			case <-time.After(time.Second):
				t.Fatal("combined server did not become ready")
			}
			if factoryAddress != netip.MustParseAddr("192.0.2.10") {
				t.Fatalf("Pixiecore address=%s, want advertised address", factoryAddress)
			}
			cancel()
			if err := <-done; err != context.Canceled {
				t.Fatalf("error=%v", err)
			}
			select {
			case <-pxe.stopped:
			default:
				t.Fatal("PXE server was not stopped")
			}
		})
	}
}

func assertNotReady(t *testing.T, ready <-chan struct{}) {
	t.Helper()
	select {
	case <-ready:
		t.Fatal("reported ready before all listeners")
	case <-time.After(20 * time.Millisecond):
	}
}

type fakeHTTP struct{ ready chan struct{} }

func (f *fakeHTTP) Serve(ctx context.Context, _ ports.PublishedArtifactSet, _ netip.AddrPort, ready func() error) error {
	<-f.ready
	if err := ready(); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

type fakePXE struct {
	ready   chan struct{}
	stopped chan struct{}
	onReady func()
	once    sync.Once
}

func (f *fakePXE) Serve() error {
	<-f.ready
	f.onReady()
	<-f.stopped
	return nil
}

func (f *fakePXE) Shutdown() { f.once.Do(func() { close(f.stopped) }) }

func scriptSet(t *testing.T, script string) ports.PublishedArtifactSet {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.ipxe")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(script))
	return ports.PublishedArtifactSet{Artifacts: []ports.PublishedArtifact{{Artifact: domain.Artifact{
		Path: "artifacts/pxe/agent.x86_64.ipxe", Size: int64(len(script)), SHA256: fmt.Sprintf("sha256:%x", digest),
	}, FilePath: path}}}
}

func mustMAC(t *testing.T, value string) net.HardwareAddr {
	t.Helper()
	mac, err := net.ParseMAC(value)
	if err != nil {
		t.Fatal(err)
	}
	return mac
}
