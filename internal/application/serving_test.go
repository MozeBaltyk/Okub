package application

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
)

const servingConfig = `apiVersion: okub.io/v1alpha1
kind: Cluster
metadata: {name: lab}
cluster: {baseDomain: example.test, version: "4.20", architecture: x86_64, topology: sno}
platform: {type: none}
network: {machineCIDR: 192.168.1.0/24, clusterCIDR: 10.128.0.0/14, hostPrefix: 23, serviceCIDR: 172.30.0.0/16, dnsServers: [192.168.1.1], gateway: 192.168.1.1}
hosts:
  - {name: master-0, role: master, interfaces: [{name: eno1, macAddress: "52:54:00:00:00:10"}], address: 192.168.1.20/24, installationDisk: /dev/vda}
boot: {method: pxe, artifactsBaseURL: "http://192.168.1.10:8080/artifacts/pxe"}
credentials: {pullSecretFile: pull-secret.json, sshPublicKeyFile: id.pub}
`

func TestArtifactServerRejectsInvalidRequestValues(t *testing.T) {
	base := ArtifactServeRequest{Mode: "http", BindAddress: "192.168.1.10", AdvertisedAddress: "192.168.1.10", Port: 8080}
	tests := []struct {
		name   string
		change func(*ArtifactServeRequest)
	}{
		{"mode", func(r *ArtifactServeRequest) { r.Mode = "ftp" }},
		{"low port", func(r *ArtifactServeRequest) { r.Port = 0 }},
		{"high port", func(r *ArtifactServeRequest) { r.Port = 65536 }},
		{"bind", func(r *ArtifactServeRequest) { r.BindAddress = "anywhere" }},
		{"advertise", func(r *ArtifactServeRequest) { r.AdvertisedAddress = "127.0.0.1" }},
		{"address mismatch", func(r *ArtifactServeRequest) { r.AdvertisedAddress = "192.168.1.11" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.change(&request)
			_, _, err := (ArtifactServer{}).Serve(context.Background(), request)
			var compatibility *CompatibilityError
			if !errors.As(err, &compatibility) {
				t.Fatalf("error=%T %v, want CompatibilityError", err, err)
			}
		})
	}
}

func TestArtifactServerValidatesAdvertisedRangeAndLocalAddress(t *testing.T) {
	for _, test := range []struct {
		name, address, finding string
		local                  bool
	}{
		{name: "outside machine network", address: "192.168.2.10", local: true, finding: "serve.addressOutsideMachineCIDR"},
		{name: "not local", address: "192.168.1.10", local: false, finding: "serve.addressNotLocal"},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &servingTransport{}
			server := testArtifactServer(test.local, transport)
			request := ArtifactServeRequest{ConfigPath: "ignored", Mode: "http", BindAddress: test.address, AdvertisedAddress: test.address, Port: 8080}
			_, report, err := server.Serve(context.Background(), request)
			if err != nil || report.Valid || len(report.Findings) == 0 || report.Findings[len(report.Findings)-1].ID != test.finding || transport.called {
				t.Fatalf("error=%v report=%#v transport.called=%t", err, report, transport.called)
			}
		})
	}
}

func TestArtifactServerValidRequestReachesTransport(t *testing.T) {
	transport := &servingTransport{}
	server := testArtifactServer(true, transport)
	ready := false
	request := ArtifactServeRequest{
		ConfigPath: "ignored", ManifestPath: "run.json", Mode: "http", BindAddress: "192.168.1.10", AdvertisedAddress: "192.168.1.10", Port: 8080,
		Ready: func(result ArtifactServeResult) error { ready = result.State == "ready"; return nil },
	}
	result, report, err := server.Serve(context.Background(), request)
	if err != nil || !report.Valid || !transport.called || !ready || transport.address != netip.MustParseAddrPort("192.168.1.10:8080") || result.State != "stopped" {
		t.Fatalf("result=%#v report=%#v error=%v transport=%#v ready=%t", result, report, err, transport, ready)
	}
}

func TestArtifactServerPXERequiresAuthoritativeIPXEScript(t *testing.T) {
	server := testArtifactServer(true, &servingTransport{})
	request := ArtifactServeRequest{
		ConfigPath: "ignored", ManifestPath: "run.json", Mode: "pxe", BindAddress: "192.168.1.10", AdvertisedAddress: "192.168.1.10", Port: 8080,
		PXEPorts: ports.PXEPorts{DHCP: 1067, TFTP: 1069, PXE: 14011, HTTP: 8081},
	}
	_, report, err := server.Serve(context.Background(), request)
	if err != nil || report.Valid || report.Findings[len(report.Findings)-1].ID != "serve.missingIPXEScript" {
		t.Fatalf("report=%#v error=%v", report, err)
	}
}

func TestArtifactServerPXENormalizesAllowlistAndReportsProvenance(t *testing.T) {
	pxe := &servingPXETransport{}
	server := ArtifactServer{
		Validator: ConfigurationValidator{Source: sourceStub{data: []byte(servingConfig)}, CacheDirectory: "/cache", HomeDirectory: "/home/test"},
		Source:    servingPXESource{}, Network: servingNetwork{local: true}, PXE: pxe,
	}
	request := ArtifactServeRequest{
		ConfigPath: "ignored", ManifestPath: "run.json", Mode: "pxe", BindAddress: "192.168.1.10", AdvertisedAddress: "192.168.1.10", Port: 8080,
		PXEPorts: ports.PXEPorts{DHCP: 1067, TFTP: 1069, PXE: 14011, HTTP: 8081},
	}
	result, report, err := server.Serve(context.Background(), request)
	if err != nil || !report.Valid || !pxe.called || len(pxe.macs) != 1 || pxe.macs[0] != "52:54:00:00:00:10" || result.PixiecoreVersion != "pinned" || result.IPXESHA256 != "sha256:test" {
		t.Fatalf("result=%#v report=%#v error=%v pxe=%#v", result, report, err, pxe)
	}
}

func TestArtifactServerPXERejectsArtifactBaseURLMismatch(t *testing.T) {
	pxe := &servingPXETransport{}
	server := ArtifactServer{
		Validator: ConfigurationValidator{Source: sourceStub{data: []byte(servingConfig)}, CacheDirectory: "/cache", HomeDirectory: "/home/test"},
		Source:    servingPXESource{}, Network: servingNetwork{local: true}, PXE: pxe,
	}
	request := ArtifactServeRequest{
		ConfigPath: "ignored", ManifestPath: "run.json", Mode: "pxe", BindAddress: "192.168.1.10", AdvertisedAddress: "192.168.1.10", Port: 8082,
		PXEPorts: ports.PXEPorts{DHCP: 1067, TFTP: 1069, PXE: 14011, HTTP: 8081},
	}
	_, report, err := server.Serve(context.Background(), request)
	if err != nil || report.Valid || pxe.called || report.Findings[len(report.Findings)-1].ID != "serve.artifactsBaseURLMismatch" {
		t.Fatalf("report=%#v error=%v pxe.called=%t", report, err, pxe.called)
	}
}

func testArtifactServer(local bool, transport *servingTransport) ArtifactServer {
	return ArtifactServer{
		Validator: ConfigurationValidator{Source: sourceStub{data: []byte(servingConfig)}, CacheDirectory: "/cache", HomeDirectory: "/home/test"},
		Source:    servingSource{}, Network: servingNetwork{local: local}, Transport: transport,
	}
}

type servingSource struct{}

func (servingSource) LoadAndVerify(string) (ports.PublishedArtifactSet, error) {
	return ports.PublishedArtifactSet{Artifacts: []ports.PublishedArtifact{{Artifact: domain.Artifact{Path: "artifacts/agent.x86_64.iso"}}}}, nil
}

type servingPXESource struct{}

func (servingPXESource) LoadAndVerify(string) (ports.PublishedArtifactSet, error) {
	return ports.PublishedArtifactSet{Artifacts: []ports.PublishedArtifact{{Artifact: domain.Artifact{Path: "artifacts/pxe/agent.x86_64.ipxe"}}}}, nil
}

type servingNetwork struct{ local bool }

func (n servingNetwork) HasLocalAddress(netip.Addr) (bool, error) { return n.local, nil }

type servingTransport struct {
	called  bool
	address netip.AddrPort
}

func (t *servingTransport) Serve(_ context.Context, _ ports.PublishedArtifactSet, address netip.AddrPort, ready func() error) error {
	t.called = true
	t.address = address
	return ready()
}

type servingPXETransport struct {
	called bool
	macs   []string
}

func (t *servingPXETransport) Serve(_ context.Context, _ ports.PublishedArtifactSet, _ netip.AddrPort, _ netip.Addr, _ ports.PXEPorts, macs []string, ready func(ports.PXEInfo) error) error {
	t.called = true
	t.macs = macs
	return ready(ports.PXEInfo{PixiecoreVersion: "pinned", IPXESHA256: "sha256:test"})
}
