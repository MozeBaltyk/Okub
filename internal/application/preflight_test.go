package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"testing"

	"github.com/mozebaltyk/Okub/internal/domain"
)

type preflightFake struct {
	missingDNS, badPTR, conflict, badCredential, badTool, cached bool
	endpointCalls, toolCalls, credentialCalls                    int
	listenerFailures                                             map[int]bool
	listenerCalls                                                []int
	addressChecks                                                []netip.Addr
	routeChecks                                                  []netip.Addr
}

func (f *preflightFake) LookupHost(_ context.Context, _ string, name string) ([]netip.Addr, error) {
	if f.missingDNS && strings.HasPrefix(name, "api.") {
		return nil, errors.New("not found")
	}
	address := "192.168.1.20"
	if strings.HasPrefix(name, "api.") || strings.HasPrefix(name, "api-int.") {
		address = "192.168.1.10"
	} else if strings.Contains(name, ".apps.") {
		address = "192.168.1.11"
	}
	return []netip.Addr{netip.MustParseAddr(address)}, nil
}

func (f *preflightFake) LookupAddr(context.Context, string, netip.Addr) ([]string, error) {
	if f.badPTR {
		return []string{"other.example.test"}, nil
	}
	return []string{"master-0.lab.example.test"}, nil
}

func (f *preflightFake) ReadCredential(path string) ([]byte, error) {
	f.credentialCalls++
	if f.badCredential && strings.Contains(path, "pull") {
		return nil, errors.New("permission denied SECRET-MARKER")
	}
	if strings.Contains(path, "pull") {
		return []byte(`{"auths":{"quay.io":{"auth":"U0VDUkVULU1BUktFUg=="}}}`), nil
	}
	return []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AA== test"), nil
}
func (f *preflightFake) DirectoryWritable(string) error { return nil }
func (f *preflightFake) ToolVersion(context.Context, string) (string, error) {
	f.toolCalls++
	if f.badTool {
		return "openshift-install 4.19.9", nil
	}
	return "openshift-install 4.20.1", nil
}
func (f *preflightFake) AddressInUse(_ context.Context, address netip.Addr, _ []int) (bool, error) {
	f.addressChecks = append(f.addressChecks, address)
	return f.conflict, nil
}
func (f *preflightFake) TCPReachable(_ context.Context, _ netip.Addr, port int) error {
	f.listenerCalls = append(f.listenerCalls, port)
	if f.listenerFailures[port] {
		return errors.New("connection refused")
	}
	return nil
}
func (f *preflightFake) RouteAvailable(_ context.Context, address netip.Addr) error {
	f.routeChecks = append(f.routeChecks, address)
	return nil
}
func (f *preflightFake) EndpointReachable(context.Context, string) error {
	f.endpointCalls++
	return nil
}
func (f *preflightFake) Lookup(string, string, string) (domain.Release, bool, error) {
	return domain.Release{}, f.cached, nil
}
func (*preflightFake) OpenBlob(string, domain.ReleaseInput) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (*preflightFake) HasBlob(string, domain.ReleaseInput) (bool, error)      { return false, nil }
func (*preflightFake) StoreBlob(string, domain.ReleaseInput, io.Reader) error { return nil }
func (*preflightFake) Publish(string, domain.Release) error                   { return nil }

func TestPreflightSuccessAndDeterministicReport(t *testing.T) {
	fake := &preflightFake{}
	report, err := testPreflight(fake).Run(context.Background(), "ignored", "", false)
	if err != nil || !report.Valid || report.Offline || len(report.Checks) < 20 {
		t.Fatalf("Run() error = %v, report = %#v", err, report)
	}
	for i := 1; i < len(report.Checks); i++ {
		previous, current := report.Checks[i-1], report.Checks[i]
		if previous.ID > current.ID || previous.ID == current.ID && previous.Path > current.Path {
			t.Fatalf("checks are not sorted: %#v before %#v", previous, current)
		}
	}
	if fake.endpointCalls != 3 || fake.credentialCalls != 2 || fake.toolCalls != 2 {
		t.Fatalf("calls: endpoints=%d credentials=%d tools=%d", fake.endpointCalls, fake.credentialCalls, fake.toolCalls)
	}
}

func TestPreflightGateFailuresContinueIndependentlyAndRedactSecrets(t *testing.T) {
	fake := &preflightFake{missingDNS: true, badPTR: true, conflict: true, badCredential: true, badTool: true}
	report, err := testPreflight(fake).Run(context.Background(), "ignored", "", false)
	if err != nil || report.Valid {
		t.Fatalf("Run() error = %v, report = %#v", err, report)
	}
	wanted := map[string]bool{
		"dns.api.forward": false, "dns.host.reverse": false, "network.addressAvailable": false,
		"credential.pullSecret": false, "tool.openshift-install": false,
	}
	for _, check := range report.Checks {
		if check.Status == "fail" {
			if _, ok := wanted[check.ID]; ok {
				wanted[check.ID] = true
			}
		}
		if strings.Contains(check.Message, "SECRET-MARKER") {
			t.Fatalf("secret leaked in %#v", check)
		}
	}
	for id, found := range wanted {
		if !found {
			t.Errorf("missing failed check %s", id)
		}
	}
	if fake.endpointCalls != 3 || fake.toolCalls != 2 || fake.credentialCalls != 2 {
		t.Fatal("an independent check stopped after a failure")
	}
}

func TestPreflightOfflineNeverCallsEndpointsAndRequiresVerifiedCache(t *testing.T) {
	for _, cached := range []bool{false, true} {
		fake := &preflightFake{cached: cached}
		report, err := testPreflight(fake).Run(context.Background(), "ignored", "", true)
		if err != nil || report.Valid != cached || !report.Offline || fake.endpointCalls != 0 {
			t.Fatalf("cached=%t error=%v report=%#v endpoint calls=%d", cached, err, report, fake.endpointCalls)
		}
		if countStatus(report, "connectivity.", "skip") != 3 {
			t.Fatalf("connectivity was not skipped: %#v", report.Checks)
		}
	}
}

func TestPreflightVerifiedCacheDoesNotRequirePathToolsOrDownloadEndpoints(t *testing.T) {
	fake := &preflightFake{cached: true, badTool: true}
	report, err := testPreflight(fake).Run(context.Background(), "ignored", "", false)
	if err != nil || !report.Valid || fake.toolCalls != 0 || fake.endpointCalls != 1 {
		t.Fatalf("error=%v report=%#v tools=%d endpoints=%d", err, report, fake.toolCalls, fake.endpointCalls)
	}
	for _, id := range []string{"tool.openshift-install", "tool.oc"} {
		if failed(report, id) {
			t.Fatalf("cached tool failed: %#v", report.Checks)
		}
	}
}

func TestPreflightFedoraHelperHAProxyRequiresAllListeners(t *testing.T) {
	tests := []struct {
		name       string
		failPort   int
		wantValid  bool
		wantFailID string
	}{
		{name: "all reachable", wantValid: true},
		{name: "API unreachable", failPort: 6443, wantFailID: "network.loadBalancer.api"},
		{name: "MCS unreachable", failPort: 22623, wantFailID: "network.loadBalancer.mcs"},
		{name: "HTTP unreachable", failPort: 80, wantFailID: "network.loadBalancer.http"},
		{name: "HTTPS unreachable", failPort: 443, wantFailID: "network.loadBalancer.https"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &preflightFake{listenerFailures: map[int]bool{test.failPort: test.failPort != 0}}
			workflow := testPreflight(fake)
			workflow.Validator.Source = sourceStub{data: []byte(helperTestConfig())}
			report, err := workflow.Run(context.Background(), "ignored", "", false)
			if err != nil || report.Valid != test.wantValid {
				t.Fatalf("error=%v report=%#v", err, report)
			}
			if fmt.Sprint(fake.listenerCalls) != "[6443 22623 80 443]" {
				t.Fatalf("listener calls=%v", fake.listenerCalls)
			}
			for _, id := range []string{"network.loadBalancer.api", "network.loadBalancer.mcs", "network.loadBalancer.http", "network.loadBalancer.https"} {
				wantStatus := domain.CheckPass
				if id == test.wantFailID {
					wantStatus = domain.CheckFail
				}
				if !hasPreflightStatus(report, id, wantStatus) {
					t.Errorf("unexpected status for %s: %#v", id, report.Checks)
				}
			}
			for _, address := range fake.addressChecks {
				if address == netip.MustParseAddr("192.168.1.10") || address == netip.MustParseAddr("192.168.1.11") {
					t.Fatalf("VIP used address-availability semantics: %v", fake.addressChecks)
				}
			}
			for _, wanted := range []netip.Addr{netip.MustParseAddr("192.168.1.10"), netip.MustParseAddr("192.168.1.11")} {
				if !containsAddress(fake.routeChecks, wanted) {
					t.Errorf("VIP route check missing for %s: %v", wanted, fake.routeChecks)
				}
			}
		})
	}
}

func TestPreflightWithoutHelperKeepsVIPAvailabilityChecks(t *testing.T) {
	fake := &preflightFake{}
	report, err := testPreflight(fake).Run(context.Background(), "ignored", "", false)
	if err != nil || !report.Valid || len(fake.listenerCalls) != 0 {
		t.Fatalf("error=%v report=%#v listenerCalls=%v", err, report, fake.listenerCalls)
	}
	for _, wanted := range []netip.Addr{netip.MustParseAddr("192.168.1.10"), netip.MustParseAddr("192.168.1.11")} {
		if !containsAddress(fake.addressChecks, wanted) {
			t.Errorf("legacy availability check missing for %s: %v", wanted, fake.addressChecks)
		}
	}
}

func TestPreflightOfflineSkipsHelperListeners(t *testing.T) {
	fake := &preflightFake{cached: true}
	workflow := testPreflight(fake)
	workflow.Validator.Source = sourceStub{data: []byte(helperTestConfig())}
	report, err := workflow.Run(context.Background(), "ignored", "", true)
	if err != nil || !report.Valid || len(fake.listenerCalls) != 0 {
		t.Fatalf("error=%v report=%#v listenerCalls=%v", err, report, fake.listenerCalls)
	}
	for _, id := range []string{"network.loadBalancer.api", "network.loadBalancer.mcs", "network.loadBalancer.http", "network.loadBalancer.https"} {
		if !hasPreflightStatus(report, id, domain.CheckSkip) {
			t.Errorf("listener %s was not skipped: %#v", id, report.Checks)
		}
	}
}

func TestPreflightFailsClosedForUnknownReleaseResourceRules(t *testing.T) {
	fake := &preflightFake{}
	workflow := testPreflight(fake)
	workflow.Validator.Source = sourceStub{data: []byte(strings.Replace(preflightConfig, `version: "4.20"`, `version: "4.19"`, 1))}
	report, err := workflow.Run(context.Background(), "ignored", "", false)
	if err != nil || !failed(report, "host.resources") {
		t.Fatalf("error=%v report=%#v", err, report)
	}
}

func testPreflight(fake *preflightFake) Preflight {
	return Preflight{
		Validator: ConfigurationValidator{Source: sourceStub{data: []byte(preflightConfig)}, CacheDirectory: "/cache"},
		DNS:       fake, Environment: fake, Cache: fake,
	}
}

func countStatus(report domain.PreflightReport, prefix, status string) int {
	count := 0
	for _, check := range report.Checks {
		if strings.HasPrefix(check.ID, prefix) && check.Status == status {
			count++
		}
	}
	return count
}

func failed(report domain.PreflightReport, id string) bool {
	for _, check := range report.Checks {
		if check.ID == id && check.Status == domain.CheckFail {
			return true
		}
	}
	return false
}

func hasPreflightStatus(report domain.PreflightReport, id, status string) bool {
	for _, check := range report.Checks {
		if check.ID == id && check.Status == status {
			return true
		}
	}
	return false
}

func helperTestConfig() string {
	return preflightConfig + `helper:
  fedoraRelease: 44
  vmName: okub-helper
  storagePool: images
  wanCIDR: 172.31.255.0/24
  wanMAC: "02:00:00:00:00:01"
  lanMAC: "02:00:00:00:00:02"
`
}

const preflightConfig = `apiVersion: okub.io/v1alpha1
kind: Cluster
metadata: {name: lab}
cluster: {baseDomain: example.test, version: "4.20", architecture: x86_64, topology: sno}
platform: {type: none}
network:
  machineCIDR: 192.168.1.0/24
  clusterCIDR: 10.128.0.0/14
  hostPrefix: 23
  serviceCIDR: 172.30.0.0/16
  dnsServers: [192.168.1.1]
  gateway: 192.168.1.1
  apiVIP: 192.168.1.10
  ingressVIP: 192.168.1.11
  rendezvousIP: 192.168.1.20
hosts:
  - name: master-0
    role: master
    interfaces: [{name: eno1, macAddress: "52:54:00:00:00:10"}]
    address: 192.168.1.20/24
    installationDisk: /dev/vda
boot: {method: iso}
credentials: {pullSecretFile: pull-secret.json, sshPublicKeyFile: id.pub}
`
