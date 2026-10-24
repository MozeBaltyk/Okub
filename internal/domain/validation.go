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

var (
	dnsLabelPattern   = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$`)
	unicastMACPattern = regexp.MustCompile(`^[0-9A-Fa-f][02468AaCcEe](?::[0-9A-Fa-f]{2}){5}$`)
	localMACPattern   = regexp.MustCompile(`^[0-9A-Fa-f][26AaEe](?::[0-9A-Fa-f]{2}){5}$`)
)

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
	if c.Cluster.Topology == "sno" && c.Platform.Type == "baremetal" {
		add("platform.sno", "platform.type", "SNO requires platform type none")
	}
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
	if c.Network.RendezvousIP != "" {
		rendezvous, err := netip.ParseAddr(c.Network.RendezvousIP)
		if err != nil || !rendezvous.Is4() {
			add("network.address", "network.rendezvousIP", "must be a valid IPv4 address")
		} else if machineOK && !machine.Contains(rendezvous) {
			add("network.outsideMachineCIDR", "network.rendezvousIP", "address is outside machine CIDR")
		}
	}
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
	staticMaster := false
	gateway, gatewayErr := netip.ParseAddr(c.Network.Gateway)
	gatewayOK := gatewayErr == nil && gateway.Is4()
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
			if err != nil || !unicastMACPattern.MatchString(iface.MACAddress) || allZero(mac) {
				add("interface.macAddress", path+".macAddress", "must be a canonical colon-separated unicast MAC address")
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
			prefix, err := netip.ParsePrefix(host.Address)
			if err == nil && prefix.Addr().Is4() {
				if host.Role == "master" {
					staticMaster = true
				}
				if gatewayOK && !prefix.Contains(gateway) {
					add("network.gatewayOutsideHostPrefix", base+".address", "gateway is outside the host address prefix")
				}
			}
			if len(host.Interfaces) > 1 {
				add("host.staticInterfaces", base+".interfaces", "a statically addressed host must have exactly one interface until a primary interface can be selected")
			}
		}
		validateInstallationDisk(host.InstallationDisk, base+".installationDisk", add)
	}
	if !staticMaster && c.Network.RendezvousIP == "" {
		add("field.required", "network.rendezvousIP", "rendezvous IP is required when no master has a static address")
	}
	if static && c.Network.Gateway == "" {
		add("network.staticGateway", "network.gateway", "gateway is required when hosts use static addresses")
	}
	if static && len(c.Network.DNSServers) == 0 {
		add("network.staticDNS", "network.dnsServers", "a DNS server is required when hosts use static addresses")
	}
	validateTopology(c.Cluster.Topology, masters, workers, add)
	if c.Helper != nil {
		validateHelper(*c.Helper, c, add)
	}

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

func validateHelper(h Helper, c ClusterDefinition, add func(string, string, string)) {
	if h.FedoraRelease != 44 {
		add("helper.fedoraRelease", "helper.fedoraRelease", "only Fedora Cloud release 44 is supported")
	}
	for i, host := range c.Hosts {
		if host.Address == "" {
			add("helper.hostAddress", fmt.Sprintf("hosts[%d].address", i), "Fedora helper DNS, DHCP, and HAProxy require a static host address")
		}
	}
	for _, item := range []struct{ value, path string }{{h.VMName, "helper.vmName"}, {h.StoragePool, "helper.storagePool"}, {h.WANCIDR, "helper.wanCIDR"}, {h.WANMAC, "helper.wanMAC"}, {h.LANMAC, "helper.lanMAC"}} {
		if item.value == "" {
			add("field.required", item.path, "field is required")
		}
	}
	machine, machineErr := netip.ParsePrefix(c.Network.MachineCIDR)
	machineOK := machineErr == nil && machine.Addr().Is4() && machine == machine.Masked() && machine.Bits() >= 24 && machine.Bits() <= 29
	if !machineOK {
		add("helper.lanCIDR", "network.machineCIDR", "Fedora helper LAN must be a canonical IPv4 CIDR from /24 through /29")
	} else {
		helperIP, hostIP := machine.Addr().Next(), lastAddress(machine).Prev()
		if c.Network.Gateway != helperIP.String() {
			add("helper.gateway", "network.gateway", "must be the first usable machineCIDR address used by the Fedora helper")
		}
		if !containsValue(c.Network.DNSServers, helperIP.String()) {
			add("helper.dns", "network.dnsServers", "must include the Fedora helper first usable address")
		}
		uses := []struct{ value, path string }{{c.Network.APIVIP, "network.apiVIP"}, {c.Network.IngressVIP, "network.ingressVIP"}, {c.Network.RendezvousIP, "network.rendezvousIP"}}
		for i, host := range c.Hosts {
			if p, err := netip.ParsePrefix(host.Address); err == nil {
				uses = append(uses, struct{ value, path string }{p.Addr().String(), fmt.Sprintf("hosts[%d].address", i)})
			}
		}
		for _, use := range uses {
			if a, err := netip.ParseAddr(use.value); err == nil && (a == helperIP || a == hostIP) {
				add("helper.lanAddressConflict", use.path, "address is reserved for the Fedora helper or managed LAN host bridge")
			}
		}
		reserved := map[netip.Addr]bool{machine.Addr(): true, lastAddress(machine): true, helperIP: true, hostIP: true}
		for _, value := range append([]string{c.Network.APIVIP, c.Network.IngressVIP, c.Network.RendezvousIP}, c.Network.DNSServers...) {
			if a, err := netip.ParseAddr(value); err == nil && machine.Contains(a) {
				reserved[a] = true
			}
		}
		for _, host := range c.Hosts {
			if p, err := netip.ParsePrefix(host.Address); err == nil && machine.Contains(p.Addr()) {
				reserved[p.Addr()] = true
			}
		}
		if len(reserved) >= 1<<(32-machine.Bits()) {
			add("helper.lanCapacity", "network.machineCIDR", "LAN has no address left for a DHCP range")
		}
	}
	wan, err := netip.ParsePrefix(h.WANCIDR)
	if err != nil || !wan.Addr().Is4() || wan != wan.Masked() || wan.Bits() < 24 || wan.Bits() > 29 || !prefixInsideRFC1918(wan) {
		add("helper.wanCIDR", "helper.wanCIDR", "must be a canonical RFC1918 IPv4 CIDR from /24 through /29")
	} else {
		for _, value := range []string{c.Network.MachineCIDR, c.Network.ClusterCIDR, c.Network.ServiceCIDR} {
			if p, e := netip.ParsePrefix(value); e == nil && p.Addr().Is4() && wan.Overlaps(p) {
				add("helper.wanCIDROverlap", "helper.wanCIDR", "must not overlap cluster network CIDRs")
				break
			}
		}
	}
	// Reserve four characters for the derived -wan/-lan libvirt network names.
	name := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,58}$`)
	if h.VMName != "" && !name.MatchString(h.VMName) {
		add("helper.vmName", "helper.vmName", "must be a valid libvirt name of at most 59 characters")
	}
	if h.StoragePool != "" && strings.TrimSpace(h.StoragePool) != h.StoragePool {
		add("helper.storagePool", "helper.storagePool", "must be a non-whitespace storage pool name")
	}
	seen := map[string]string{}
	for _, item := range []struct{ value, path string }{{h.WANMAC, "helper.wanMAC"}, {h.LANMAC, "helper.lanMAC"}} {
		if item.value == "" {
			continue
		}
		mac, e := net.ParseMAC(item.value)
		if e != nil || !localMACPattern.MatchString(item.value) {
			add("helper.macAddress", item.path, "must be a canonical colon-separated locally administered unicast MAC address")
			continue
		}
		n := mac.String()
		if prev, ok := seen[n]; ok {
			add("helper.macDuplicate", item.path, "MAC address duplicates "+prev)
		} else {
			seen[n] = item.path
		}
	}
	if h.LibvirtURI != "qemu:///system" {
		add("helper.libvirtURI", "helper.libvirtURI", "must be qemu:///system for local managed networking")
	}
	if h.VCPU < 1 || h.VCPU > 64 {
		add("helper.vcpu", "helper.vcpu", "must be between 1 and 64")
	}
	if h.MemoryMiB < 1024 {
		add("helper.memoryMiB", "helper.memoryMiB", "must be at least 1024")
	}
	if h.RootDiskSizeGiB < 8 {
		add("helper.rootDiskSizeGiB", "helper.rootDiskSizeGiB", "must be at least 8")
	}
	if c.Network.APIVIP == "" || c.Network.IngressVIP == "" {
		add("helper.vip", "network.apiVIP", "API and ingress VIPs are required for the Fedora helper")
	}
}

func containsValue(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func prefixInsideRFC1918(prefix netip.Prefix) bool {
	if !prefix.IsValid() || !prefix.Addr().Is4() {
		return false
	}
	for _, value := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		private := netip.MustParsePrefix(value)
		if private.Contains(prefix.Masked().Addr()) && private.Contains(lastAddress(prefix)) {
			return true
		}
	}
	return false
}

func lastAddress(prefix netip.Prefix) netip.Addr {
	b := prefix.Masked().Addr().As4()
	hostBits := 32 - prefix.Bits()
	value := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	value |= uint32((uint64(1) << hostBits) - 1)
	return netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)})
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

func validateInstallationDisk(value, path string, add func(string, string, string)) {
	if value == "" {
		add("field.required", path, "field is required")
		return
	}
	if strings.ContainsRune(value, 0) || !filepath.IsAbs(value) || !strings.HasPrefix(value, "/dev/") || filepath.Clean(value) != value || value == "/dev/" {
		add("host.installationDisk", path, "must be an absolute installer-visible device path under /dev")
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
