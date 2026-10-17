package domain

import (
	"reflect"
	"testing"
)

func TestValidateValidSNO(t *testing.T) {
	report := Validate(validDefinition())
	if !report.Valid || len(report.Findings) != 0 {
		t.Fatalf("report = %#v, want valid", report)
	}
}

func TestValidateAggregatesAndOrdersIndependentFindings(t *testing.T) {
	definition := validDefinition()
	definition.Metadata.Name = "Bad_Name"
	definition.Network.ClusterCIDR = "192.168.100.0/25"
	definition.Network.HostPrefix = 25
	definition.Hosts = append(definition.Hosts, Host{
		Name: "master-0", Role: "worker", Address: "192.168.100.20/24", InstallationDisk: "/dev/vdb",
		Interfaces: []Interface{{Name: "eno1", MACAddress: "52:54:00:00:00:10"}},
	})

	first := Validate(definition)
	second := Validate(definition)
	if first.Valid || len(first.Findings) < 6 {
		t.Fatalf("findings = %#v, want aggregated failures", first.Findings)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reports are not deterministic:\n%#v\n%#v", first, second)
	}
	wanted := map[string]bool{"dns.label": false, "network.cidrOverlap": false, "host.nameDuplicate": false, "network.addressConflict": false, "interface.macDuplicate": false, "topology.sno": false}
	for _, finding := range first.Findings {
		if _, ok := wanted[finding.ID]; ok {
			wanted[finding.ID] = true
		}
	}
	for id, found := range wanted {
		if !found {
			t.Errorf("missing finding %q in %#v", id, first.Findings)
		}
	}
}

func TestValidateHostPrefixAndMalformedValues(t *testing.T) {
	definition := validDefinition()
	definition.Cluster.BaseDomain = "*.example.test."
	definition.Network.HostPrefix = 14
	definition.Hosts[0].Address = "not-an-address"
	definition.Hosts[0].Interfaces[0].MACAddress = "ff:ff:ff:ff:ff:ff"

	report := Validate(definition)
	wanted := map[string]bool{"dns.domain": false, "network.hostPrefix": false, "network.address": false, "interface.macAddress": false}
	for _, finding := range report.Findings {
		if _, ok := wanted[finding.ID]; ok {
			wanted[finding.ID] = true
		}
	}
	for id, found := range wanted {
		if !found {
			t.Errorf("missing finding %q in %#v", id, report.Findings)
		}
	}
}

func TestValidateRejectsAllZeroMAC(t *testing.T) {
	definition := validDefinition()
	definition.Hosts[0].Interfaces[0].MACAddress = "00:00:00:00:00:00"
	report := Validate(definition)
	for _, finding := range report.Findings {
		if finding.ID == "interface.macAddress" {
			return
		}
	}
	t.Fatalf("all-zero MAC was accepted: %#v", report)
}

func validDefinition() ClusterDefinition {
	return ClusterDefinition{
		APIVersion: "okub.io/v1alpha1", Kind: "Cluster", Metadata: Metadata{Name: "lab"},
		Cluster:  Cluster{BaseDomain: "example.test", Version: "4.20", Architecture: "x86_64", Topology: "sno"},
		Platform: Platform{Type: "none"},
		Network: Network{
			MachineCIDR: "192.168.100.0/24", ClusterCIDR: "10.128.0.0/14", HostPrefix: 23,
			ServiceCIDR: "172.30.0.0/16", DNSServers: []string{"192.168.100.1"}, Gateway: "192.168.100.1",
			APIVIP: "192.168.100.10", IngressVIP: "192.168.100.11",
		},
		Hosts: []Host{{Name: "master-0", Role: "master", Interfaces: []Interface{{Name: "eno1", MACAddress: "52:54:00:00:00:10"}}, Address: "192.168.100.20/24", InstallationDisk: "/dev/vda"}},
		Boot:  Boot{Method: "iso"}, Artifacts: Artifacts{OutputDirectory: "./build/lab", CacheDirectory: "/tmp/okub", ChecksumPolicy: "required"},
		Credentials: Credentials{PullSecretFile: "~/.config/okub/pull-secret.json", SSHPublicKeyFile: "~/.ssh/id_ed25519.pub"},
	}
}
