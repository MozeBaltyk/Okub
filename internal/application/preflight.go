package application

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
)

// Preflight runs independent, read-only checks and aggregates every result.
type Preflight struct {
	Validator   ConfigurationValidator
	DNS         ports.DNSChecker
	Environment ports.PreflightEnvironment
	Cache       ports.ReleaseCache
}

func (p Preflight) Run(ctx context.Context, configPath, outputOverride string, offline bool) (domain.PreflightReport, error) {
	validator := p.Validator
	validator.OutputDirectoryOverride = outputOverride
	validated, err := validator.Validate(configPath)
	if err != nil {
		return domain.PreflightReport{}, err
	}
	checks := make([]domain.PreflightCheck, 0, len(validated.Report.Findings)+24)
	for _, finding := range validated.Report.Findings {
		checks = append(checks, domain.PreflightCheck{ID: finding.ID, Status: domain.CheckFail, Path: finding.Path, Message: finding.Message, Hint: finding.Hint})
	}
	if validated.Report.Valid {
		p.runEnvironment(ctx, validated.Definition, offline, &checks)
	}
	sort.SliceStable(checks, func(i, j int) bool {
		if checks[i].ID != checks[j].ID {
			return checks[i].ID < checks[j].ID
		}
		if checks[i].Path != checks[j].Path {
			return checks[i].Path < checks[j].Path
		}
		return checks[i].Message < checks[j].Message
	})
	valid := true
	for _, check := range checks {
		if check.Status == domain.CheckFail {
			valid = false
			break
		}
	}
	return domain.PreflightReport{Valid: valid, Offline: offline, Checks: checks}, nil
}

func (p Preflight) runEnvironment(ctx context.Context, c domain.ClusterDefinition, offline bool, checks *[]domain.PreflightCheck) {
	add := func(id, status, path, message string) {
		*checks = append(*checks, domain.PreflightCheck{ID: id, Status: status, Path: path, Message: message})
	}
	p.credentials(c, add)
	for _, item := range []struct{ path, value string }{{"artifacts.outputDirectory", c.Artifacts.OutputDirectory}, {"artifacts.cacheDirectory", c.Artifacts.CacheDirectory}} {
		if err := p.Environment.DirectoryWritable(item.value); err != nil {
			add("filesystem.writable", domain.CheckFail, item.path, "directory does not exist with a writable parent")
		} else {
			add("filesystem.writable", domain.CheckPass, item.path, "directory exists or can be created")
		}
	}
	_, cached, cacheErr := p.Cache.Lookup(c.Artifacts.CacheDirectory, c.Cluster.Version, c.Cluster.Architecture)
	p.tools(ctx, c, cached && cacheErr == nil, add)
	p.resources(c, add)
	if offline {
		networkIDs := []string{"network.route"}
		if usesFedoraHelper(c) {
			networkIDs = append(networkIDs, "network.loadBalancer.api", "network.loadBalancer.mcs", "network.loadBalancer.http", "network.loadBalancer.https")
		} else {
			networkIDs = append(networkIDs, "network.addressAvailable")
		}
		for _, id := range append([]string{
			"connectivity.releaseMetadata", "connectivity.installer", "connectivity.imageRegistry",
			"dns.api.forward", "dns.api-int.forward", "dns.apps.forward", "dns.host.forward", "dns.host.reverse",
		}, networkIDs...) {
			add(id, domain.CheckSkip, "cluster.version", "network check skipped in offline mode")
		}
		if c.Helper != nil {
			add("helper.dhcp", domain.CheckSkip, "helper", "DHCP readiness cannot be safely observed in offline mode")
			if c.Boot.Method == "pxe" {
				add("helper.pxeTransport", domain.CheckSkip, "boot.method", "built-in PXE transport is started separately and cannot be observed in preflight")
			}
		}
		if cacheErr != nil || !cached {
			add("offline.verifiedCache", domain.CheckFail, "artifacts.cacheDirectory", "verified HTTP release inputs and immutable image metadata are not cached")
		} else {
			add("offline.verifiedCache", domain.CheckPass, "artifacts.cacheDirectory", "verified HTTP release inputs and immutable image metadata are cached")
		}
		return
	}
	p.dns(ctx, c, add)
	p.network(ctx, c, add)
	if c.Helper != nil {
		add("helper.dhcp", domain.CheckSkip, "helper", "DHCP lease service cannot be safely observed from the host preflight")
		if c.Boot.Method == "pxe" {
			add("helper.pxeTransport", domain.CheckSkip, "boot.method", "built-in PXE transport is started separately and cannot be observed in preflight")
		}
	}
	endpoints := []struct{ id, endpoint string }{{"connectivity.imageRegistry", "https://quay.io/v2/"}}
	if cached && cacheErr == nil {
		add("connectivity.releaseMetadata", domain.CheckSkip, "artifacts.cacheDirectory", "verified release metadata is cached")
		add("connectivity.installer", domain.CheckSkip, "artifacts.cacheDirectory", "verified installer and client archives are cached")
	} else {
		endpoints = append(endpoints,
			struct{ id, endpoint string }{"connectivity.releaseMetadata", "https://amd64.origin.releases.ci.openshift.org/api/v1/releasestream/4-scos-stable/tags"},
			struct{ id, endpoint string }{"connectivity.installer", "https://api.github.com/repos/okd-project/okd/releases"})
	}
	for _, endpoint := range endpoints {
		if err := p.Environment.EndpointReachable(ctx, endpoint.endpoint); err != nil {
			add(endpoint.id, domain.CheckFail, "cluster.version", "required endpoint is not reachable")
		} else {
			add(endpoint.id, domain.CheckPass, "cluster.version", "required endpoint is reachable")
		}
	}
}

func (p Preflight) credentials(c domain.ClusterDefinition, add func(string, string, string, string)) {
	items := []struct {
		id, path, value string
		valid           func([]byte) bool
	}{
		{"credential.pullSecret", "credentials.pullSecretFile", c.Credentials.PullSecretFile, validPullSecret},
		{"credential.sshPublicKey", "credentials.sshPublicKeyFile", c.Credentials.SSHPublicKeyFile, validPublicKey},
	}
	for _, item := range items {
		content, err := p.Environment.ReadCredential(item.value)
		if err != nil {
			add(item.id, domain.CheckFail, item.path, "credential is not a readable regular file")
		} else if !item.valid(content) {
			add(item.id, domain.CheckFail, item.path, "credential content is invalid")
		} else {
			add(item.id, domain.CheckPass, item.path, "credential is readable and valid")
		}
	}
}

func validPullSecret(content []byte) bool {
	var value struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if json.Unmarshal(content, &value) != nil || len(value.Auths) == 0 {
		return false
	}
	for registry, credential := range value.Auths {
		decoded, err := base64.StdEncoding.DecodeString(credential.Auth)
		if registry == "" || err != nil || len(decoded) == 0 {
			return false
		}
	}
	return true
}

func validPublicKey(content []byte) bool {
	fields := strings.Fields(strings.TrimSpace(string(content)))
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "ssh-") && !strings.HasPrefix(fields[0], "ecdsa-") || len(fields[1]) > 1<<20 {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil || len(decoded) < 4 {
		return false
	}
	nameLength := int(binary.BigEndian.Uint32(decoded[:4]))
	return nameLength > 0 && nameLength <= len(decoded)-4 && string(decoded[4:4+nameLength]) == fields[0]
}

var installerVersionPattern = regexp.MustCompile(`(?m)(?:openshift-install\s+)?(\d+\.\d+)(?:\.\d+)?`)

func (p Preflight) tools(ctx context.Context, c domain.ClusterDefinition, cached bool, add func(string, string, string, string)) {
	for _, name := range []string{"openshift-install", "oc"} {
		id := "tool." + name
		if cached {
			add(id, domain.CheckPass, "artifacts.cacheDirectory", "verified cached "+name+" is compatible")
			continue
		}
		version, err := p.Environment.ToolVersion(ctx, name)
		if err != nil {
			add(id, domain.CheckFail, "artifacts.cacheDirectory", name+" is unavailable; run `okub images sync`")
			continue
		}
		match := installerVersionPattern.FindStringSubmatch(version)
		if len(match) != 2 || match[1] != releaseSeries(c.Cluster.Version) {
			add(id, domain.CheckFail, "cluster.version", name+" version is incompatible with the requested release")
			continue
		}
		add(id, domain.CheckPass, "cluster.version", name+" version is compatible")
	}
}

func releaseSeries(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

func (p Preflight) resources(c domain.ClusterDefinition, add func(string, string, string, string)) {
	type requirement struct{ cpu, memoryGiB, diskGiB int }
	// Keep explicit release rules: changed upstream requirements must add a rule.
	requirements := map[string]requirement{"4.20": {cpu: 8, memoryGiB: 32, diskGiB: 120}}
	req, ok := requirements[releaseSeries(c.Cluster.Version)]
	if !ok {
		add("host.resources", domain.CheckFail, "cluster.version", "host resource requirements are unknown for the requested release")
		return
	}
	for i := range c.Hosts {
		add("host.resources", domain.CheckPass, fmt.Sprintf("hosts[%d]", i), fmt.Sprintf("release requires at least %d CPUs, %d GiB memory, and %d GiB disk", req.cpu, req.memoryGiB, req.diskGiB))
	}
}

func (p Preflight) dns(ctx context.Context, c domain.ClusterDefinition, add func(string, string, string, string)) {
	expectedAPI := expectedAPIAddress(c)
	expectedIngress := expectedIngressAddress(c)
	zone := c.Metadata.Name + "." + c.Cluster.BaseDomain
	queries := []struct {
		id, name string
		expected netip.Addr
	}{
		{"dns.api.forward", "api." + zone, expectedAPI},
		{"dns.api-int.forward", "api-int." + zone, expectedAPI},
		{"dns.apps.forward", "okub-preflight.apps." + zone, expectedIngress},
	}
	for i, resolver := range c.Network.DNSServers {
		path := fmt.Sprintf("network.dnsServers[%d]", i)
		for _, query := range queries {
			p.forward(ctx, resolver, path, query.id, query.name, query.expected, add)
		}
		for hostIndex, host := range c.Hosts {
			hostPath := fmt.Sprintf("hosts[%d].address", hostIndex)
			if host.Address == "" {
				add("dns.host.forward", domain.CheckSkip, hostPath, "host address uses DHCP")
				add("dns.host.reverse", domain.CheckSkip, hostPath, "host address uses DHCP")
				continue
			}
			prefix, _ := netip.ParsePrefix(host.Address)
			name := host.Name + "." + zone
			p.forward(ctx, resolver, hostPath, "dns.host.forward", name, prefix.Addr(), add)
			names, err := p.DNS.LookupAddr(ctx, resolver, prefix.Addr())
			if err != nil || !containsString(names, name) {
				add("dns.host.reverse", domain.CheckFail, hostPath, "host PTR record is missing or does not match its forward name")
			} else {
				add("dns.host.reverse", domain.CheckPass, hostPath, "host PTR record matches its forward name")
			}
		}
	}
}

func (p Preflight) forward(ctx context.Context, resolver, path, id, name string, expected netip.Addr, add func(string, string, string, string)) {
	addresses, err := p.DNS.LookupHost(ctx, resolver, name)
	if err != nil || !containsAddress(addresses, expected) {
		add(id, domain.CheckFail, path, "DNS record is missing or does not contain the expected address")
	} else {
		add(id, domain.CheckPass, path, "DNS record contains the expected address")
	}
}

func expectedAPIAddress(c domain.ClusterDefinition) netip.Addr {
	if address, err := netip.ParseAddr(c.Network.APIVIP); err == nil {
		return address
	}
	return rendezvousAddress(c)
}

func expectedIngressAddress(c domain.ClusterDefinition) netip.Addr {
	if address, err := netip.ParseAddr(c.Network.IngressVIP); err == nil {
		return address
	}
	return rendezvousAddress(c)
}

func rendezvousAddress(c domain.ClusterDefinition) netip.Addr {
	if address, err := netip.ParseAddr(c.Network.RendezvousIP); err == nil {
		return address
	}
	for _, host := range c.Hosts {
		if host.Role == "master" && host.Address != "" {
			prefix, _ := netip.ParsePrefix(host.Address)
			return prefix.Addr()
		}
	}
	return netip.Addr{}
}

func (p Preflight) network(ctx context.Context, c domain.ClusterDefinition, add func(string, string, string, string)) {
	type target struct {
		path  string
		addr  netip.Addr
		ports []int
	}
	targets := []target{}
	appendAddress := func(path, value string, ports ...int) {
		if address, err := netip.ParseAddr(value); err == nil {
			targets = append(targets, target{path, address, ports})
		}
	}
	appendAddress("network.gateway", c.Network.Gateway)
	appendAddress("network.apiVIP", c.Network.APIVIP, 6443)
	appendAddress("network.ingressVIP", c.Network.IngressVIP, 80, 443)
	for i, value := range c.Network.DNSServers {
		appendAddress(fmt.Sprintf("network.dnsServers[%d]", i), value)
	}
	for i, host := range c.Hosts {
		if host.Address != "" {
			prefix, _ := netip.ParsePrefix(host.Address)
			targets = append(targets, target{path: fmt.Sprintf("hosts[%d].address", i), addr: prefix.Addr()})
		}
	}
	for _, target := range targets {
		if err := p.Environment.RouteAvailable(ctx, target.addr); err != nil {
			add("network.route", domain.CheckFail, target.path, "no usable route was observed")
		} else {
			add("network.route", domain.CheckPass, target.path, "a usable route was observed")
		}
		if target.path == "network.gateway" || strings.HasPrefix(target.path, "network.dnsServers") {
			continue
		}
		if usesFedoraHelper(c) && (target.path == "network.apiVIP" || target.path == "network.ingressVIP") {
			continue
		}
		inUse, err := p.Environment.AddressInUse(ctx, target.addr, target.ports)
		if err != nil {
			add("network.addressAvailable", domain.CheckFail, target.path, "address availability could not be observed safely")
		} else if inUse {
			add("network.addressAvailable", domain.CheckFail, target.path, "address or required port is already in use")
		} else {
			add("network.addressAvailable", domain.CheckPass, target.path, "no local address or required-port conflict was observed")
		}
	}
	if usesFedoraHelper(c) {
		listeners := []struct {
			id, path, name string
			address        netip.Addr
			port           int
		}{
			{"network.loadBalancer.api", "network.apiVIP", "API", netip.MustParseAddr(c.Network.APIVIP), 6443},
			{"network.loadBalancer.mcs", "network.apiVIP", "MCS", netip.MustParseAddr(c.Network.APIVIP), 22623},
			{"network.loadBalancer.http", "network.ingressVIP", "HTTP ingress", netip.MustParseAddr(c.Network.IngressVIP), 80},
			{"network.loadBalancer.https", "network.ingressVIP", "HTTPS ingress", netip.MustParseAddr(c.Network.IngressVIP), 443},
		}
		for _, listener := range listeners {
			if err := p.Environment.TCPReachable(ctx, listener.address, listener.port); err != nil {
				add(listener.id, domain.CheckFail, listener.path, "expected Fedora helper HAProxy "+listener.name+" listener is not reachable")
			} else {
				add(listener.id, domain.CheckPass, listener.path, "expected Fedora helper HAProxy "+listener.name+" listener is reachable")
			}
		}
	}
}

func usesFedoraHelper(c domain.ClusterDefinition) bool { return c.Helper != nil }

func containsAddress(values []netip.Addr, wanted netip.Addr) bool {
	for _, value := range values {
		if value.Unmap() == wanted.Unmap() {
			return true
		}
	}
	return false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSuffix(value, "."), wanted) {
			return true
		}
	}
	return false
}
