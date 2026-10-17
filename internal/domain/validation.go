package domain

import (
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	SeverityError = "error"
)

// Finding is one stable, machine-readable configuration problem.
type Finding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

// ValidationReport is ordered deterministically and contains no configuration values.
type ValidationReport struct {
	Valid    bool      `json:"valid"`
	Findings []Finding `json:"findings"`
}

var dnsLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$`)

// Validate performs pure structural, network, uniqueness, and topology checks.
func Validate(c ClusterDefinition) ValidationReport {
	findings := []Finding{}
	add := func(id, path, message string) {
		findings = append(findings, Finding{ID: id, Severity: SeverityError, Path: path, Message: message})
	}
	required := func(value, path string) {
		if value == "" {
			add("field.required", path, "field is required")
		}
	}
	allowed := func(value, path string, values ...string) {
		if value == "" {
			required(value, path)
			return
		}
		for _, candidate := range values {
			if value == candidate {
				return
			}
		}
		add("field.value", path, "value is not supported")
	}

	if c.APIVersion == "" {
		required(c.APIVersion, "apiVersion")
	} else if c.APIVersion != "okub.io/v1alpha1" {
		add("apiVersion.unsupported", "apiVersion", "API version is not supported")
	}
	if c.Kind == "" {
		required(c.Kind, "kind")
	} else if c.Kind != "Cluster" {
		add("kind.unsupported", "kind", "kind must be Cluster")
	}
	validateDNSLabel(c.Metadata.Name, "metadata.name", true, add)
	validateDomain(c.Cluster.BaseDomain, "cluster.baseDomain", add)
	required(c.Cluster.Version, "cluster.version")
	allowed(c.Cluster.Architecture, "cluster.architecture", "x86_64")
	allowed(c.Cluster.Topology, "cluster.topology", "sno", "compact", "ha")
	allowed(c.Platform.Type, "platform.type", "none", "baremetal")
	allowed(c.Boot.Method, "boot.method", "iso", "pxe")
	if c.Artifacts.ChecksumPolicy != "required" {
		add("artifacts.checksumPolicy", "artifacts.checksumPolicy", "checksum policy must be required")
	}
	validatePath(c.Artifacts.OutputDirectory, "artifacts.outputDirectory", add)
	validatePath(c.Artifacts.CacheDirectory, "artifacts.cacheDirectory", add)
	validatePath(c.Credentials.PullSecretFile, "credentials.pullSecretFile", add)
	validatePath(c.Credentials.SSHPublicKeyFile, "credentials.sshPublicKeyFile", add)

	machine, machineOK := parseCIDR(c.Network.MachineCIDR, "network.machineCIDR", add)
	cluster, clusterOK := parseCIDR(c.Network.ClusterCIDR, "network.clusterCIDR", add)
	service, serviceOK := parseCIDR(c.Network.ServiceCIDR, "network.serviceCIDR", add)
	if machineOK && clusterOK && machine.Overlaps(cluster) {
		add("network.cidrOverlap", "network.clusterCIDR", "cluster CIDR overlaps machine CIDR")
	}
	if machineOK && serviceOK && machine.Overlaps(service) {
		add("network.cidrOverlap", "network.serviceCIDR", "service CIDR overlaps machine CIDR")
	}
	if clusterOK && serviceOK && cluster.Overlaps(service) {
		add("network.cidrOverlap", "network.serviceCIDR", "service CIDR overlaps cluster CIDR")
	}
	if c.Network.HostPrefix == 0 {
		add("field.required", "network.hostPrefix", "field is required")
	} else if c.Network.HostPrefix < 0 || c.Network.HostPrefix > 32 {
		add("network.hostPrefix", "network.hostPrefix", "host prefix must be between 1 and 32")
	} else if clusterOK {
		bits := cluster.Bits()
		if c.Network.HostPrefix <= bits {
			add("network.hostPrefix", "network.hostPrefix", fmt.Sprintf("host prefix must be greater than %d and no greater than 32", bits))
		} else if hostSubnetCount(c.Network.HostPrefix-bits) < len(c.Hosts) {
			add("network.hostPrefixCapacity", "network.hostPrefix", "host prefix cannot provide a node subnet for every host")
		}
	}

	if len(c.Network.DNSServers) == 0 {
		add("field.required", "network.dnsServers", "at least one DNS server is required")
	}
	type addressUse struct{ path, category string }
	addresses := map[netip.Addr]addressUse{}
	checkAddress := func(value, path, category string, withPrefix bool, mustBeInMachine bool) {
		if value == "" {
			return
		}
		var address netip.Addr
		var err error
		if withPrefix {
			var prefix netip.Prefix
			prefix, err = netip.ParsePrefix(value)
			address = prefix.Addr()
		} else {
			address, err = netip.ParseAddr(value)
		}
		if err != nil || (!address.Is4() && category != "dns") {
			message := "must be a valid IPv4 address"
			if category == "dns" {
				message = "must be a valid IP address"
			}
			add("network.address", path, message)
			return
		}
		if mustBeInMachine && machineOK && !machine.Contains(address) {
			add("network.outsideMachineCIDR", path, "address is outside machine CIDR")
		}
		if previous, found := addresses[address]; found && !((category == "dns" && previous.category == "gateway") || (category == "gateway" && previous.category == "dns")) {
			add("network.addressConflict", path, "address conflicts with "+previous.path)
		} else {
			addresses[address] = addressUse{path: path, category: category}
		}
	}

	checkAddress(c.Network.Gateway, "network.gateway", "gateway", false, true)
	checkAddress(c.Network.APIVIP, "network.apiVIP", "vip", false, true)
	checkAddress(c.Network.IngressVIP, "network.ingressVIP", "vip", false, true)
	for i, server := range c.Network.DNSServers {
		checkAddress(server, fmt.Sprintf("network.dnsServers[%d]", i), "dns", false, false)
	}
	if (c.Platform.Type == "baremetal" || c.Cluster.Topology == "compact" || c.Cluster.Topology == "ha") && c.Network.APIVIP == "" {
		add("field.required", "network.apiVIP", "API VIP is required for this platform or topology")
	}
	if (c.Platform.Type == "baremetal" || c.Cluster.Topology == "compact" || c.Cluster.Topology == "ha") && c.Network.IngressVIP == "" {
		add("field.required", "network.ingressVIP", "ingress VIP is required for this platform or topology")
	}

	hostNames := map[string]string{}
	macs := map[string]string{}
	static := false
	masters, workers := 0, 0
	if len(c.Hosts) == 0 {
		add("field.required", "hosts", "at least one host is required")
	}
	for i, host := range c.Hosts {
		base := fmt.Sprintf("hosts[%d]", i)
		validateDNSLabel(host.Name, base+".name", true, add)
		if previous, found := hostNames[host.Name]; host.Name != "" && found {
			add("host.nameDuplicate", base+".name", "host name duplicates "+previous)
		} else if host.Name != "" {
			hostNames[host.Name] = base + ".name"
		}
		if host.Role == "master" {
			masters++
		} else if host.Role == "worker" {
			workers++
		} else {
			allowed(host.Role, base+".role", "master", "worker")
		}
		if len(host.Interfaces) == 0 {
			add("field.required", base+".interfaces", "at least one interface is required")
		}
		interfaceNames := map[string]string{}
		for j, iface := range host.Interfaces {
			path := fmt.Sprintf("%s.interfaces[%d]", base, j)
			required(iface.Name, path+".name")
			if previous, found := interfaceNames[iface.Name]; iface.Name != "" && found {
				add("interface.nameDuplicate", path+".name", "interface name duplicates "+previous)
			} else if iface.Name != "" {
				interfaceNames[iface.Name] = path + ".name"
			}
			mac, err := net.ParseMAC(iface.MACAddress)
			if err != nil || len(mac) != 6 || mac[0]&1 != 0 || allZero(mac) {
				add("interface.macAddress", path+".macAddress", "must be a valid unicast MAC address")
				continue
			}
			normalized := mac.String()
			if previous, found := macs[normalized]; found {
				add("interface.macDuplicate", path+".macAddress", "MAC address duplicates "+previous)
			} else {
				macs[normalized] = path + ".macAddress"
			}
		}
		if host.Address != "" {
			static = true
			checkAddress(host.Address, base+".address", "host", true, true)
		}
		validatePath(host.InstallationDisk, base+".installationDisk", add)
	}
	if static && c.Network.Gateway == "" {
		add("network.staticGateway", "network.gateway", "gateway is required when hosts use static addresses")
	}
	if static && len(c.Network.DNSServers) == 0 {
		add("network.staticDNS", "network.dnsServers", "a DNS server is required when hosts use static addresses")
	}
	validateTopology(c.Cluster.Topology, masters, workers, add)

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		if findings[i].ID != findings[j].ID {
			return findings[i].ID < findings[j].ID
		}
		return findings[i].Message < findings[j].Message
	})
	return ValidationReport{Valid: len(findings) == 0, Findings: findings}
}

func allZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}

func parseCIDR(value, path string, add func(string, string, string)) (netip.Prefix, bool) {
	if value == "" {
		add("field.required", path, "field is required")
		return netip.Prefix{}, false
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil || !prefix.Addr().Is4() || prefix != prefix.Masked() {
		add("network.cidr", path, "must be a canonical IPv4 CIDR")
		return netip.Prefix{}, false
	}
	return prefix, true
}

func validateDNSLabel(value, path string, required bool, add func(string, string, string)) {
	if value == "" {
		if required {
			add("field.required", path, "field is required")
		}
		return
	}
	if len(value) > 63 || !dnsLabelPattern.MatchString(value) {
		add("dns.label", path, "must be a lowercase DNS label")
	}
}

func validateDomain(value, path string, add func(string, string, string)) {
	if value == "" {
		add("field.required", path, "field is required")
		return
	}
	if len(value) > 253 || strings.HasSuffix(value, ".") || strings.HasPrefix(value, "*.") {
		add("dns.domain", path, "must be a DNS domain without a wildcard or trailing dot")
		return
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) > 63 || !dnsLabelPattern.MatchString(label) {
			add("dns.domain", path, "must be a lowercase DNS domain")
			return
		}
	}
}

func validatePath(value, path string, add func(string, string, string)) {
	if value == "" {
		add("field.required", path, "field is required")
	} else if strings.ContainsRune(value, 0) || !filepath.IsLocal(strings.TrimPrefix(value, "~/")) && !filepath.IsAbs(value) {
		add("path.invalid", path, "must be a valid local or absolute path")
	}
}

func hostSubnetCount(difference int) int {
	if difference >= 31 {
		return int(^uint(0) >> 1)
	}
	return 1 << difference
}

func validateTopology(topology string, masters, workers int, add func(string, string, string)) {
	switch topology {
	case "sno":
		if masters != 1 || workers != 0 {
			add("topology.sno", "hosts", "SNO requires exactly one master and no workers")
		}
	case "compact":
		if masters != 3 {
			add("topology.compact", "hosts", "compact topology requires exactly three masters")
		}
	case "ha":
		if masters != 3 || workers < 2 {
			add("topology.ha", "hosts", "HA topology requires exactly three masters and at least two workers")
		}
	}
}
