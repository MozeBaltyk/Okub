package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mozebaltyk/Okub/internal/domain"
)

type sourceStub struct {
	data []byte
	err  error
}

func (s sourceStub) Read(string) ([]byte, error) { return s.data, s.err }

func TestValidateStrictYAMLAndDefaults(t *testing.T) {
	data := []byte(`apiVersion: okub.io/v1alpha1
kind: Cluster
metadata: {name: lab}
cluster: {baseDomain: example.test, version: "4.20", architecture: x86_64, topology: sno}
platform: {type: none}
network: {machineCIDR: 192.168.1.0/24, clusterCIDR: 10.128.0.0/14, hostPrefix: 23, serviceCIDR: 172.30.0.0/16, dnsServers: [192.168.1.1], gateway: 192.168.1.1}
hosts:
  - {name: master-0, role: master, interfaces: [{name: eno1, macAddress: "52:54:00:00:00:10"}], address: 192.168.1.20/24, installationDisk: /dev/vda}
boot: {method: iso}
credentials: {pullSecretFile: pull-secret.json, sshPublicKeyFile: id.pub}
`)
	result, err := (ConfigurationValidator{Source: sourceStub{data: data}, CacheDirectory: "/cache/okub", HomeDirectory: "/home/tester"}).Validate("ignored")
	if err != nil || !result.Report.Valid {
		t.Fatalf("Validate() error = %v, report = %#v", err, result.Report)
	}
	if result.Definition.Artifacts.OutputDirectory != filepath.Join(".local", "build", "lab") || result.Definition.Artifacts.CacheDirectory != "/cache/okub" || result.Definition.Artifacts.ChecksumPolicy != "required" {
		t.Fatalf("defaults not applied: %#v", result.Definition.Artifacts)
	}

	result, err = (ConfigurationValidator{Source: sourceStub{data: append(data, []byte("unknown: true\n")...)}, CacheDirectory: "/cache/okub"}).Validate("ignored")
	if err != nil || result.Report.Valid || result.Report.Findings[0].ID != "config.schema" {
		t.Fatalf("unknown field was not rejected: error = %v, report = %#v", err, result.Report)
	}
}

func TestValidateSchemaBeforeTypedDecode(t *testing.T) {
	base := `apiVersion: okub.io/v1alpha1
kind: Cluster
metadata: {name: lab}
cluster: {baseDomain: example.test, version: "4.20", architecture: x86_64, topology: sno}
platform: {type: none}
network: {machineCIDR: 192.168.1.0/24, clusterCIDR: 10.128.0.0/14, hostPrefix: 23, serviceCIDR: 172.30.0.0/16, dnsServers: [192.168.1.1], rendezvousIP: 192.168.1.20}
hosts: [{name: master-0, role: master, interfaces: [{name: eno1, macAddress: "52:54:00:00:00:10"}], installationDisk: /dev/vda}]
boot: {method: iso}
credentials: {pullSecretFile: pull-secret.json, sshPublicKeyFile: id.pub}
`
	tests := []struct {
		name, config, secret string
		defaultsApplied      bool
	}{
		{"boolean version", strings.Replace(base, `version: "4.20"`, "version: true", 1), "true", true},
		{"numeric version", strings.Replace(base, `version: "4.20"`, "version: 420", 1), "420", true},
		{"object version", strings.Replace(base, `version: "4.20"`, "version: {nested: do-not-report}", 1), "do-not-report", false},
		{"null artifacts", base + "artifacts: null\n", "null", true},
		{"empty DNS item", strings.Replace(base, "dnsServers: [192.168.1.1]", `dnsServers: [""]`, 1), `""`, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := (ConfigurationValidator{Source: sourceStub{data: []byte(test.config)}, CacheDirectory: "/cache"}).Validate("ignored")
			if err != nil || result.Report.Valid || len(result.Report.Findings) == 0 || result.Report.Findings[0].ID != "config.schema" {
				t.Fatalf("error = %v, report = %#v", err, result.Report)
			}
			if applied := result.Definition.Artifacts.ChecksumPolicy == "required"; applied != test.defaultsApplied {
				t.Fatalf("defaults applied = %t, want %t", applied, test.defaultsApplied)
			}
			for _, finding := range result.Report.Findings {
				if strings.Contains(finding.Message, test.secret) {
					t.Fatalf("finding leaked input value %q: %#v", test.secret, finding)
				}
			}
		})
	}
}

func TestValidateDispatchesAPIVersionBeforeSchema(t *testing.T) {
	unsupported := []byte(`apiVersion: other.io/v9
kind: Anything
secret: do-not-report
`)
	result, err := (ConfigurationValidator{Source: sourceStub{data: unsupported}}).Validate("ignored")
	if err != nil || len(result.Report.Findings) != 1 {
		t.Fatalf("error = %v, report = %#v", err, result.Report)
	}
	want := domain.Finding{ID: "apiVersion.unsupported", Severity: domain.SeverityError, Path: "apiVersion", Message: "API version is not supported"}
	if result.Report.Findings[0] != want {
		t.Fatalf("finding = %#v, want %#v", result.Report.Findings[0], want)
	}

	for _, data := range [][]byte{
		[]byte("kind: Cluster\n"),
		[]byte("apiVersion: 7\nkind: Cluster\n"),
	} {
		result, err := (ConfigurationValidator{Source: sourceStub{data: data}}).Validate("ignored")
		if err != nil || result.Report.Valid {
			t.Fatalf("error = %v, report = %#v", err, result.Report)
		}
		for _, finding := range result.Report.Findings {
			if finding.ID == "apiVersion.unsupported" {
				t.Fatalf("missing/non-string version reported as unsupported: %#v", result.Report)
			}
		}
	}
}

func TestValidateMergesSchemaAndIndependentSemanticFindings(t *testing.T) {
	data, err := os.ReadFile("../../tests/fixtures/invalid-config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	result, err := (ConfigurationValidator{Source: sourceStub{data: data}, CacheDirectory: "/cache"}).Validate("ignored")
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"config.schema": false, "network.cidrOverlap": false, "topology.sno": false}
	for _, finding := range result.Report.Findings {
		if _, found := wanted[finding.ID]; found {
			wanted[finding.ID] = true
		}
	}
	for id, found := range wanted {
		if !found {
			t.Errorf("missing %s in %#v", id, result.Report.Findings)
		}
	}
}

func TestValidateCompactAndHANegativeFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		ids     []string
	}{
		{"compact-invalid-host-count.yaml", []string{"topology.compact"}},
		{"ha-invalid-host-count.yaml", []string{"topology.ha"}},
		{"ha-invalid-network.yaml", []string{"config.schema", "field.required", "network.staticDNS"}},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", test.fixture))
			if err != nil {
				t.Fatal(err)
			}
			result, err := (ConfigurationValidator{Source: sourceStub{data: data}, CacheDirectory: "/cache"}).Validate("ignored")
			if err != nil || result.Report.Valid {
				t.Fatalf("error = %v, report = %#v", err, result.Report)
			}
			for _, id := range test.ids {
				if !hasValidationFinding(result.Report, id) {
					t.Errorf("missing %s in %#v", id, result.Report.Findings)
				}
			}
		})
	}
}

func hasValidationFinding(report domain.ValidationReport, id string) bool {
	for _, finding := range report.Findings {
		if finding.ID == id {
			return true
		}
	}
	return false
}

func TestValidateUnknownFieldDoesNotStopSemanticFindings(t *testing.T) {
	data := []byte(`apiVersion: okub.io/v1alpha1
kind: Cluster
unknown: ignored
metadata: {name: lab}
cluster: {baseDomain: example.test, version: "4.20", architecture: x86_64, topology: sno}
platform: {type: none}
network: {machineCIDR: 192.168.1.0/24, clusterCIDR: 192.168.1.0/25, hostPrefix: 26, serviceCIDR: 172.30.0.0/16, dnsServers: [192.168.1.1]}
hosts: []
boot: {method: iso}
credentials: {pullSecretFile: pull-secret.json, sshPublicKeyFile: id.pub}
`)
	result, err := (ConfigurationValidator{Source: sourceStub{data: data}, CacheDirectory: "/cache"}).Validate("ignored")
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"config.schema": false, "network.cidrOverlap": false, "topology.sno": false}
	for _, finding := range result.Report.Findings {
		if _, found := wanted[finding.ID]; found {
			wanted[finding.ID] = true
		}
	}
	for id, found := range wanted {
		if !found {
			t.Errorf("missing %s in %#v", id, result.Report.Findings)
		}
	}
}

func TestValidateResolvesDocumentedHomePaths(t *testing.T) {
	data := []byte(`apiVersion: okub.io/v1alpha1
kind: Cluster
metadata: {name: lab}
cluster: {baseDomain: example.test, version: "4.20", architecture: x86_64, topology: sno}
platform: {type: none}
network: {machineCIDR: 192.168.1.0/24, clusterCIDR: 10.128.0.0/14, hostPrefix: 23, serviceCIDR: 172.30.0.0/16, dnsServers: [192.168.1.1], rendezvousIP: 192.168.1.20}
hosts: [{name: master-0, role: master, interfaces: [{name: eno1, macAddress: "52:54:00:00:00:10"}], installationDisk: /dev/vda}]
boot: {method: iso}
artifacts: {outputDirectory: ~/output, cacheDirectory: ~/cache}
credentials: {pullSecretFile: ~/.config/pull-secret.json, sshPublicKeyFile: ~/.ssh/id.pub}
`)
	result, err := (ConfigurationValidator{Source: sourceStub{data: data}, CacheDirectory: "/cache", HomeDirectory: "/home/tester"}).Validate("ignored")
	if err != nil || !result.Report.Valid {
		t.Fatalf("error = %v, report = %#v", err, result.Report)
	}
	wanted := []string{"/home/tester/output", "/home/tester/cache", "/home/tester/.config/pull-secret.json", "/home/tester/.ssh/id.pub"}
	got := []string{result.Definition.Artifacts.OutputDirectory, result.Definition.Artifacts.CacheDirectory, result.Definition.Credentials.PullSecretFile, result.Definition.Credentials.SSHPublicKeyFile}
	for i := range wanted {
		if got[i] != wanted[i] {
			t.Errorf("path %d = %q, want %q", i, got[i], wanted[i])
		}
	}
}

func TestValidateReadError(t *testing.T) {
	want := errors.New("no file")
	_, err := (ConfigurationValidator{Source: sourceStub{err: want}}).Validate("missing.yaml")
	var readErr *ReadError
	if !errors.As(err, &readErr) || !errors.Is(err, want) {
		t.Fatalf("error = %v, want ReadError wrapping source error", err)
	}
}
