// Package domain contains Okub's configuration model and pure rules.
package domain

import (
	"path/filepath"
	"strings"
)

// ClusterDefinition is the versioned desired state loaded from okub.yaml.
type ClusterDefinition struct {
	APIVersion  string      `yaml:"apiVersion" json:"apiVersion"`
	Kind        string      `yaml:"kind" json:"kind"`
	Metadata    Metadata    `yaml:"metadata" json:"metadata"`
	Cluster     Cluster     `yaml:"cluster" json:"cluster"`
	Platform    Platform    `yaml:"platform" json:"platform"`
	Network     Network     `yaml:"network" json:"network"`
	Hosts       []Host      `yaml:"hosts" json:"hosts"`
	Boot        Boot        `yaml:"boot" json:"boot"`
	Artifacts   Artifacts   `yaml:"artifacts" json:"artifacts"`
	Credentials Credentials `yaml:"credentials" json:"credentials"`
	Helper      *Helper     `yaml:"helper,omitempty" json:"helper,omitempty"`
}

type Metadata struct {
	Name string `yaml:"name" json:"name"`
}

type Cluster struct {
	BaseDomain   string `yaml:"baseDomain" json:"baseDomain"`
	Version      string `yaml:"version" json:"version"`
	Architecture string `yaml:"architecture" json:"architecture"`
	Topology     string `yaml:"topology" json:"topology"`
}

type Platform struct {
	Type string `yaml:"type" json:"type"`
}

type Network struct {
	MachineCIDR  string   `yaml:"machineCIDR" json:"machineCIDR"`
	ClusterCIDR  string   `yaml:"clusterCIDR" json:"clusterCIDR"`
	HostPrefix   int      `yaml:"hostPrefix" json:"hostPrefix"`
	ServiceCIDR  string   `yaml:"serviceCIDR" json:"serviceCIDR"`
	DNSServers   []string `yaml:"dnsServers" json:"dnsServers"`
	Gateway      string   `yaml:"gateway" json:"gateway,omitempty"`
	APIVIP       string   `yaml:"apiVIP" json:"apiVIP,omitempty"`
	IngressVIP   string   `yaml:"ingressVIP" json:"ingressVIP,omitempty"`
	RendezvousIP string   `yaml:"rendezvousIP" json:"rendezvousIP,omitempty"`
}

type Host struct {
	Name             string      `yaml:"name" json:"name"`
	Role             string      `yaml:"role" json:"role"`
	Interfaces       []Interface `yaml:"interfaces" json:"interfaces"`
	Address          string      `yaml:"address" json:"address,omitempty"`
	InstallationDisk string      `yaml:"installationDisk" json:"installationDisk"`
}

type Interface struct {
	Name       string `yaml:"name" json:"name"`
	MACAddress string `yaml:"macAddress" json:"macAddress"`
}

type Boot struct {
	Method string `yaml:"method" json:"method"`
}

type Artifacts struct {
	OutputDirectory string `yaml:"outputDirectory" json:"outputDirectory"`
	CacheDirectory  string `yaml:"cacheDirectory" json:"cacheDirectory"`
	ChecksumPolicy  string `yaml:"checksumPolicy" json:"checksumPolicy"`
}

type Credentials struct {
	PullSecretFile   string `yaml:"pullSecretFile" json:"pullSecretFile"`
	SSHPublicKeyFile string `yaml:"sshPublicKeyFile" json:"sshPublicKeyFile"`
}

// Helper is the explicit desired state for the optional Fedora network-services VM.
type Helper struct {
	FedoraRelease   int    `yaml:"fedoraRelease" json:"fedoraRelease"`
	VMName          string `yaml:"vmName" json:"vmName"`
	StoragePool     string `yaml:"storagePool" json:"storagePool"`
	WANCIDR         string `yaml:"wanCIDR,omitempty" json:"wanCIDR,omitempty"`
	WANMAC          string `yaml:"wanMAC" json:"wanMAC"`
	LANMAC          string `yaml:"lanMAC" json:"lanMAC"`
	LibvirtURI      string `yaml:"libvirtURI,omitempty" json:"libvirtURI,omitempty"`
	VCPU            int    `yaml:"vcpu,omitempty" json:"vcpu,omitempty"`
	MemoryMiB       int    `yaml:"memoryMiB,omitempty" json:"memoryMiB,omitempty"`
	RootDiskSizeGiB int    `yaml:"rootDiskSizeGiB,omitempty" json:"rootDiskSizeGiB,omitempty"`
}

// ApplyDefaults applies only defaults defined by the v1alpha1 contract.
func ApplyDefaults(c *ClusterDefinition, outputRoot, cacheDirectory, homeDirectory string) {
	for i := range c.Hosts {
		for j := range c.Hosts[i].Interfaces {
			c.Hosts[i].Interfaces[j].MACAddress = strings.ToLower(c.Hosts[i].Interfaces[j].MACAddress)
		}
	}
	if c.Artifacts.OutputDirectory == "" && c.Metadata.Name != "" {
		c.Artifacts.OutputDirectory = filepath.Join(outputRoot, c.Metadata.Name)
	}
	if c.Artifacts.CacheDirectory == "" {
		c.Artifacts.CacheDirectory = cacheDirectory
	}
	if c.Artifacts.ChecksumPolicy == "" {
		c.Artifacts.ChecksumPolicy = "required"
	}
	if c.Helper != nil {
		c.Helper.WANMAC = strings.ToLower(c.Helper.WANMAC)
		c.Helper.LANMAC = strings.ToLower(c.Helper.LANMAC)
		if c.Helper.WANCIDR == "" {
			c.Helper.WANCIDR = "172.31.255.0/24"
		}
		if c.Helper.LibvirtURI == "" {
			c.Helper.LibvirtURI = "qemu:///system"
		}
		if c.Helper.VCPU == 0 {
			c.Helper.VCPU = 2
		}
		if c.Helper.MemoryMiB == 0 {
			c.Helper.MemoryMiB = 2048
		}
		if c.Helper.RootDiskSizeGiB == 0 {
			c.Helper.RootDiskSizeGiB = 20
		}
	}
	c.Artifacts.OutputDirectory = resolveHome(c.Artifacts.OutputDirectory, homeDirectory)
	c.Artifacts.CacheDirectory = resolveHome(c.Artifacts.CacheDirectory, homeDirectory)
	c.Credentials.PullSecretFile = resolveHome(c.Credentials.PullSecretFile, homeDirectory)
	c.Credentials.SSHPublicKeyFile = resolveHome(c.Credentials.SSHPublicKeyFile, homeDirectory)
}

func resolveHome(path, homeDirectory string) string {
	if strings.HasPrefix(path, "~/") && homeDirectory != "" {
		return filepath.Join(homeDirectory, strings.TrimPrefix(path, "~/"))
	}
	return path
}
