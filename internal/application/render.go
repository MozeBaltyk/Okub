package application

import (
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
	"go.yaml.in/yaml/v3"
)

// InstallerInputRenderer validates desired state and publishes installer inputs.
type InstallerInputRenderer struct {
	Validator   ConfigurationValidator
	Credentials ports.CredentialSource
	Publisher   ports.GeneratedPublisher
}

// RenderResult is the stable, secret-free result of config render.
type RenderResult struct {
	Action          string   `json:"action"`
	OutputDirectory string   `json:"outputDirectory"`
	Files           []string `json:"files"`
	DryRun          bool     `json:"dryRun"`
}

// CredentialError identifies a referenced-file failure without its contents.
type CredentialError struct {
	Path string
	Err  error
}

func (e *CredentialError) Error() string { return fmt.Sprintf("read credential %q: %v", e.Path, e.Err) }
func (e *CredentialError) Unwrap() error { return e.Err }

// Render creates deterministic typed Agent-based Installer input files.
func (r InstallerInputRenderer) Render(configPath, outputOverride string, force, dryRun bool) (RenderResult, domain.ValidationReport, error) {
	validator := r.Validator
	validator.OutputDirectoryOverride = outputOverride
	validated, err := validator.Validate(configPath)
	if err != nil || !validated.Report.Valid {
		return RenderResult{}, validated.Report, err
	}
	definition := validated.Definition
	pullSecret, err := r.Credentials.ReadCredential(definition.Credentials.PullSecretFile)
	if err != nil {
		return RenderResult{}, validated.Report, &CredentialError{Path: definition.Credentials.PullSecretFile, Err: err}
	}
	sshKey, err := r.Credentials.ReadCredential(definition.Credentials.SSHPublicKeyFile)
	if err != nil {
		return RenderResult{}, validated.Report, &CredentialError{Path: definition.Credentials.SSHPublicKeyFile, Err: err}
	}
	files, err := renderInstallerFiles(definition, strings.TrimSpace(string(pullSecret)), strings.TrimSpace(string(sshKey)))
	if err != nil {
		return RenderResult{}, validated.Report, err
	}
	action, err := r.Publisher.Publish(definition.Artifacts.OutputDirectory, files, force, dryRun)
	if err != nil {
		return RenderResult{}, validated.Report, err
	}
	return RenderResult{
		Action: action, OutputDirectory: filepath.Join(definition.Artifacts.OutputDirectory, "generated"),
		Files: []string{"agent-config.yaml", "install-config.yaml"}, DryRun: dryRun,
	}, validated.Report, nil
}

type installConfig struct {
	APIVersion   string            `yaml:"apiVersion"`
	BaseDomain   string            `yaml:"baseDomain"`
	Compute      []machinePool     `yaml:"compute"`
	ControlPlane machinePool       `yaml:"controlPlane"`
	Metadata     installerMetadata `yaml:"metadata"`
	Networking   installNetworking `yaml:"networking"`
	Platform     installerPlatform `yaml:"platform"`
	PullSecret   string            `yaml:"pullSecret"`
	SSHKey       string            `yaml:"sshKey"`
}

type machinePool struct {
	Architecture   string `yaml:"architecture"`
	Hyperthreading string `yaml:"hyperthreading"`
	Name           string `yaml:"name"`
	Replicas       int    `yaml:"replicas"`
}

type installerMetadata struct {
	Name string `yaml:"name"`
}
type cidr struct {
	CIDR string `yaml:"cidr"`
}
type clusterCIDR struct {
	CIDR       string `yaml:"cidr"`
	HostPrefix int    `yaml:"hostPrefix"`
}
type installNetworking struct {
	ClusterNetwork []clusterCIDR `yaml:"clusterNetwork"`
	MachineNetwork []cidr        `yaml:"machineNetwork"`
	NetworkType    string        `yaml:"networkType"`
	ServiceNetwork []string      `yaml:"serviceNetwork"`
}
type installerPlatform struct {
	None      *struct{}          `yaml:"none,omitempty"`
	Baremetal *baremetalPlatform `yaml:"baremetal,omitempty"`
}
type baremetalPlatform struct {
	APIVIPs     []string `yaml:"apiVIPs"`
	IngressVIPs []string `yaml:"ingressVIPs"`
}

type agentConfig struct {
	APIVersion   string            `yaml:"apiVersion"`
	Kind         string            `yaml:"kind"`
	Metadata     installerMetadata `yaml:"metadata"`
	RendezvousIP string            `yaml:"rendezvousIP"`
	Hosts        []agentHost       `yaml:"hosts"`
}
type agentHost struct {
	Hostname        string           `yaml:"hostname"`
	Role            string           `yaml:"role"`
	Interfaces      []agentInterface `yaml:"interfaces"`
	RootDeviceHints rootDeviceHints  `yaml:"rootDeviceHints"`
	NetworkConfig   *networkConfig   `yaml:"networkConfig,omitempty"`
}
type agentInterface struct {
	Name       string `yaml:"name"`
	MACAddress string `yaml:"macAddress"`
}
type rootDeviceHints struct {
	DeviceName string `yaml:"deviceName"`
}
type networkConfig struct {
	Interfaces  []nmstateInterface `yaml:"interfaces"`
	DNSResolver dnsResolver        `yaml:"dns-resolver"`
	Routes      routes             `yaml:"routes"`
}
type nmstateInterface struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	State      string `yaml:"state"`
	MACAddress string `yaml:"mac-address"`
	IPv4       ipv4   `yaml:"ipv4"`
}
type ipv4 struct {
	Enabled bool        `yaml:"enabled"`
	Address []ipAddress `yaml:"address"`
	DHCP    bool        `yaml:"dhcp"`
}
type ipAddress struct {
	IP           string `yaml:"ip"`
	PrefixLength int    `yaml:"prefix-length"`
}
type dnsResolver struct {
	Config dnsConfig `yaml:"config"`
}
type dnsConfig struct {
	Server []string `yaml:"server"`
}
type routes struct {
	Config []route `yaml:"config"`
}
type route struct {
	Destination      string `yaml:"destination"`
	NextHopAddress   string `yaml:"next-hop-address"`
	NextHopInterface string `yaml:"next-hop-interface"`
	TableID          int    `yaml:"table-id"`
}

func renderInstallerFiles(c domain.ClusterDefinition, pullSecret, sshKey string) ([]ports.GeneratedFile, error) {
	architecture := map[string]string{"x86_64": "amd64"}[c.Cluster.Architecture]
	masters, workers := 0, 0
	for _, host := range c.Hosts {
		if host.Role == "master" {
			masters++
		} else {
			workers++
		}
	}
	platform := installerPlatform{None: &struct{}{}}
	if c.Platform.Type == "baremetal" {
		platform = installerPlatform{Baremetal: &baremetalPlatform{APIVIPs: []string{c.Network.APIVIP}, IngressVIPs: []string{c.Network.IngressVIP}}}
	}
	install := installConfig{
		APIVersion: "v1", BaseDomain: c.Cluster.BaseDomain,
		Compute:      []machinePool{{Architecture: architecture, Hyperthreading: "Enabled", Name: "worker", Replicas: workers}},
		ControlPlane: machinePool{Architecture: architecture, Hyperthreading: "Enabled", Name: "master", Replicas: masters},
		Metadata:     installerMetadata{Name: c.Metadata.Name},
		Networking:   installNetworking{ClusterNetwork: []clusterCIDR{{CIDR: c.Network.ClusterCIDR, HostPrefix: c.Network.HostPrefix}}, MachineNetwork: []cidr{{CIDR: c.Network.MachineCIDR}}, NetworkType: "OVNKubernetes", ServiceNetwork: []string{c.Network.ServiceCIDR}},
		Platform:     platform, PullSecret: pullSecret, SSHKey: sshKey,
	}
	hosts := append([]domain.Host(nil), c.Hosts...)
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })
	agent := agentConfig{APIVersion: "v1beta1", Kind: "AgentConfig", Metadata: installerMetadata{Name: c.Metadata.Name}}
	if c.Network.RendezvousIP != "" {
		rendezvous, _ := netip.ParseAddr(c.Network.RendezvousIP)
		agent.RendezvousIP = rendezvous.String()
	}
	for _, host := range hosts {
		interfaces := append([]domain.Interface(nil), host.Interfaces...)
		sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].Name < interfaces[j].Name })
		item := agentHost{Hostname: host.Name, Role: host.Role, RootDeviceHints: rootDeviceHints{DeviceName: host.InstallationDisk}}
		for _, iface := range interfaces {
			mac, _ := net.ParseMAC(iface.MACAddress)
			item.Interfaces = append(item.Interfaces, agentInterface{Name: iface.Name, MACAddress: mac.String()})
		}
		if host.Address != "" {
			prefix, err := netip.ParsePrefix(host.Address)
			if err != nil {
				return nil, fmt.Errorf("render host network configuration: %w", err)
			}
			if host.Role == "master" && agent.RendezvousIP == "" {
				agent.RendezvousIP = prefix.Addr().String()
			}
			primary := item.Interfaces[0]
			item.NetworkConfig = &networkConfig{
				Interfaces:  []nmstateInterface{{Name: primary.Name, Type: "ethernet", State: "up", MACAddress: primary.MACAddress, IPv4: ipv4{Enabled: true, Address: []ipAddress{{IP: prefix.Addr().String(), PrefixLength: prefix.Bits()}}, DHCP: false}}},
				DNSResolver: dnsResolver{Config: dnsConfig{Server: c.Network.DNSServers}},
				Routes:      routes{Config: []route{{Destination: "0.0.0.0/0", NextHopAddress: c.Network.Gateway, NextHopInterface: primary.Name, TableID: 254}}},
			}
		}
		agent.Hosts = append(agent.Hosts, item)
	}
	installYAML, err := yaml.Marshal(install)
	if err != nil {
		return nil, fmt.Errorf("marshal install-config: %w", err)
	}
	agentYAML, err := yaml.Marshal(agent)
	if err != nil {
		return nil, fmt.Errorf("marshal agent-config: %w", err)
	}
	return []ports.GeneratedFile{{Name: "install-config.yaml", Content: installYAML}, {Name: "agent-config.yaml", Content: agentYAML}}, nil
}
