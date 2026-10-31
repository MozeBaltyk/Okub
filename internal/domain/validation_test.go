package domain

import (
	"fmt"
	"reflect"
	"testing"
)

func TestValidateValidSNO(t *testing.T) {
	report := Validate(validDefinition())
	if !report.Valid || len(report.Findings) != 0 {
		t.Fatalf("report = %#v, want valid", report)
	}
}

func TestValidateCompactAndHAHostCounts(t *testing.T) {
	tests := []struct {
		name     string
		topology string
		workers  int
	}{
		{"compact without workers", "compact", 0},
		{"compact with workers", "compact", 2},
		{"HA minimum workers", "ha", 2},
		{"HA additional workers", "ha", 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := validDefinition()
			definition.Cluster.Topology = test.topology
			definition.Hosts = nil
			for i := 0; i < 3+test.workers; i++ {
				role, roleIndex := "master", i
				if i >= 3 {
					role, roleIndex = "worker", i-3
				}
				definition.Hosts = append(definition.Hosts, Host{
					Name: fmt.Sprintf("%s-%d", role, roleIndex), Role: role,
					Address: fmt.Sprintf("192.168.100.%d/24", 20+i), InstallationDisk: fmt.Sprintf("/dev/sd%c", 'a'+i),
					Interfaces: []Interface{{Name: "eno1", MACAddress: fmt.Sprintf("52:54:00:00:10:%02x", 20+i)}},
				})
			}
			if report := Validate(definition); !report.Valid {
				t.Fatalf("report = %#v, want valid", report)
			}
		})
	}
}

func TestValidatePXEArtifactBaseURL(t *testing.T) {
	for _, test := range []struct {
		name, value string
		valid       bool
	}{
		{name: "valid", value: "http://192.168.100.10:8080/artifacts/pxe", valid: true},
		{name: "missing"},
		{name: "https", value: "https://192.168.100.10/artifacts/pxe"},
		{name: "credentials", value: "http://user:pass@192.168.100.10/artifacts/pxe"},
		{name: "missing host", value: "http://:8080/artifacts/pxe"},
		{name: "zero port", value: "http://192.168.100.10:0/artifacts/pxe"},
		{name: "large port", value: "http://192.168.100.10:65536/artifacts/pxe"},
		{name: "wrong path", value: "http://192.168.100.10/boot"},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := validDefinition()
			definition.Boot = Boot{Method: "pxe", ArtifactsBaseURL: test.value}
			report := Validate(definition)
			if report.Valid != test.valid {
				t.Fatalf("report=%#v, valid=%t", report, test.valid)
			}
		})
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

func TestValidateRendezvousIP(t *testing.T) {
	tests := []struct {
		name       string
		rendezvous string
		wantID     string
	}{
		{name: "DHCP with explicit rendezvous", rendezvous: "192.168.100.30"},
		{name: "DHCP without rendezvous", wantID: "field.required"},
		{name: "invalid rendezvous", rendezvous: "not-an-ip", wantID: "network.address"},
		{name: "outside machine network", rendezvous: "192.168.101.30", wantID: "network.outsideMachineCIDR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := validDefinition()
			definition.Hosts[0].Address = ""
			definition.Network.RendezvousIP = test.rendezvous
			report := Validate(definition)
			found := false
			for _, finding := range report.Findings {
				if finding.Path == "network.rendezvousIP" && finding.ID == test.wantID {
					found = true
				}
			}
			if test.wantID == "" && !report.Valid {
				t.Fatalf("report = %#v, want valid", report)
			}
			if test.wantID != "" && !found {
				t.Fatalf("report = %#v, want %s", report, test.wantID)
			}
		})
	}
}

func TestValidateInstallerCompatibilityRules(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ClusterDefinition)
		wantID string
	}{
		{
			name: "SNO baremetal", wantID: "platform.sno",
			mutate: func(c *ClusterDefinition) { c.Platform.Type = "baremetal" },
		},
		{
			name: "relative installation disk", wantID: "host.installationDisk",
			mutate: func(c *ClusterDefinition) { c.Hosts[0].InstallationDisk = "vda" },
		},
		{
			name: "path outside dev", wantID: "host.installationDisk",
			mutate: func(c *ClusterDefinition) { c.Hosts[0].InstallationDisk = "/tmp/vda" },
		},
		{
			name: "device path traversal", wantID: "host.installationDisk",
			mutate: func(c *ClusterDefinition) { c.Hosts[0].InstallationDisk = "/dev/../tmp/vda" },
		},
		{
			name: "multiple static interfaces", wantID: "host.staticInterfaces",
			mutate: func(c *ClusterDefinition) {
				c.Hosts[0].Interfaces = append(c.Hosts[0].Interfaces, Interface{Name: "eno2", MACAddress: "52:54:00:00:00:11"})
			},
		},
		{
			name: "gateway outside host prefix", wantID: "network.gatewayOutsideHostPrefix",
			mutate: func(c *ClusterDefinition) {
				c.Network.MachineCIDR = "192.168.0.0/16"
				c.Network.Gateway = "192.168.101.1"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := validDefinition()
			test.mutate(&definition)
			report := Validate(definition)
			for _, finding := range report.Findings {
				if finding.ID == test.wantID {
					return
				}
			}
			t.Fatalf("report = %#v, want finding %s", report, test.wantID)
		})
	}
}

func TestValidateAllowsStableInstallationDiskPath(t *testing.T) {
	definition := validDefinition()
	definition.Hosts[0].InstallationDisk = "/dev/disk/by-id/wwn-test"
	if report := Validate(definition); !report.Valid {
		t.Fatalf("stable installation disk rejected: %#v", report)
	}
}

func TestValidateHelperAndDefaults(t *testing.T) {
	definition := validDefinition()
	definition.Helper = &Helper{FedoraRelease: 44, VMName: "okub-helper", StoragePool: "images", WANMAC: "02:00:00:00:00:01", LANMAC: "02:00:00:00:00:02"}
	ApplyDefaults(&definition, ".local/build", ".local/cache", "/home/test")
	if report := Validate(definition); !report.Valid {
		t.Fatalf("valid helper rejected: %#v", report)
	}
	if definition.Helper.WANCIDR != "172.31.255.0/24" || definition.Helper.RootDiskSizeGiB != 20 {
		t.Fatalf("defaults=%+v", definition.Helper)
	}
	definition.Helper.LibvirtURI = "qemu+ssh://host/system"
	if report := Validate(definition); !hasFinding(report, "helper.libvirtURI") {
		t.Fatalf("remote libvirt accepted: %#v", report)
	}
}

func TestMACsNormalizeCaseAndRejectHyphens(t *testing.T) {
	definition := validDefinition()
	definition.Hosts[0].Interfaces[0].MACAddress = "52:54:00:AA:BB:CC"
	definition.Helper = &Helper{FedoraRelease: 44, VMName: "okub-helper", StoragePool: "images", WANMAC: "02:00:00:AA:BB:01", LANMAC: "02:00:00:AA:BB:02"}
	ApplyDefaults(&definition, ".local/build", ".local/cache", "/home/test")
	if definition.Hosts[0].Interfaces[0].MACAddress != "52:54:00:aa:bb:cc" || definition.Helper.WANMAC != "02:00:00:aa:bb:01" {
		t.Fatalf("MACs were not normalized: %+v %+v", definition.Hosts[0].Interfaces[0], definition.Helper)
	}
	definition.Hosts[0].Interfaces[0].MACAddress = "52-54-00-aa-bb-cc"
	definition.Helper.WANMAC = "02-00-00-aa-bb-01"
	report := Validate(definition)
	if !hasFinding(report, "interface.macAddress") || !hasFinding(report, "helper.macAddress") {
		t.Fatalf("hyphen MACs accepted: %#v", report)
	}
}

func TestValidateHelperReservesAddressesAndWAN(t *testing.T) {
	definition := validDefinition()
	definition.Helper = &Helper{FedoraRelease: 44, VMName: "helper", StoragePool: "images", WANCIDR: "192.168.100.0/24", WANMAC: "02:00:00:00:00:01", LANMAC: "02:00:00:00:00:02", LibvirtURI: "qemu:///system", VCPU: 2, MemoryMiB: 2048, RootDiskSizeGiB: 20}
	if report := Validate(definition); !hasFinding(report, "helper.wanCIDROverlap") {
		t.Fatalf("overlap accepted: %#v", report)
	}
	definition.Helper.WANCIDR = "172.31.255.0/24"
	definition.Hosts[0].Address = "192.168.100.254/24"
	if report := Validate(definition); !hasFinding(report, "helper.lanAddressConflict") {
		t.Fatalf("host bridge address accepted: %#v", report)
	}
}

func hasFinding(report ValidationReport, id string) bool {
	for _, finding := range report.Findings {
		if finding.ID == id {
			return true
		}
	}
	return false
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
