package helper

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mozebaltyk/Okub/internal/application"
	"github.com/mozebaltyk/Okub/internal/domain"
)

type configBytes []byte

func (c configBytes) Read(string) ([]byte, error) { return c, nil }

type runnerFunc func(context.Context, string, []string, []string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, directory string, env, args []string) ([]byte, error) {
	return f(ctx, directory, env, args)
}

func TestOwnedAddressesAreExact(t *testing.T) {
	for _, address := range []string{"libvirt_network.wan", "libvirt_network.lan", "libvirt_volume.base_image", "libvirt_volume.root", "libvirt_cloudinit_disk.seed", "libvirt_domain.helper"} {
		if !ownedAddresses[address] {
			t.Fatalf("missing %s", address)
		}
	}
	if len(ownedAddresses) != 6 {
		t.Fatalf("owned addresses=%d", len(ownedAddresses))
	}
}

func TestValidateHostNetworksAllowsStoppedOwnedNetworks(t *testing.T) {
	wanBridge, lanBridge := bridgeNames("okub-helper")
	present := map[string]bool{"libvirt_network.wan": true, "libvirt_network.lan": true}
	if _, err := validateHostNetworks(nil, present, wanBridge, lanBridge, "172.31.255.0/24", "192.168.101.0/24"); err != nil {
		t.Fatalf("stopped networks rejected: %v", err)
	}
	hostile := []InterfaceCIDR{{Name: "hostile0", Prefix: mustPrefix("192.168.101.20/24")}}
	if _, err := validateHostNetworks(hostile, present, wanBridge, lanBridge, "172.31.255.0/24", "192.168.101.0/24"); err == nil {
		t.Fatal("hostile overlapping interface accepted")
	}
	wrongOwned := []InterfaceCIDR{{Name: wanBridge, Prefix: mustPrefix("172.31.255.2/24")}}
	if _, err := validateHostNetworks(wrongOwned, present, wanBridge, lanBridge, "172.31.255.0/24", "192.168.101.0/24"); err == nil {
		t.Fatal("owned bridge with wrong address accepted")
	}
}

func TestDownVerifiesIdentityWithoutHostNetworkGate(t *testing.T) {
	uuid := "12345678-1234-1234-1234-123456789abc"
	runner := runnerFunc(func(_ context.Context, _ string, _ []string, args []string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.HasSuffix(joined, " uri"):
			return []byte("qemu:///system\n"), nil
		case strings.Contains(joined, "pool-info"):
			return []byte("State: running\n"), nil
		case strings.Contains(joined, " state list"):
			addresses := make([]string, 0, len(ownedAddresses))
			for address := range ownedAddresses {
				addresses = append(addresses, address)
			}
			sort.Strings(addresses)
			return []byte(strings.Join(addresses, "\n")), nil
		case strings.Contains(joined, " show -json"):
			return []byte(fmt.Sprintf(`{"values":{"root_module":{"resources":[{"address":"libvirt_domain.helper","values":{"name":"okub-helper","id":%q}}]}}}`, uuid)), nil
		case strings.Contains(joined, " domuuid "):
			return []byte(uuid + "\n"), nil
		case strings.Contains(joined, " domstate "):
			return []byte("shut off\n"), nil
		default:
			return nil, nil
		}
	})
	networkCalls := 0
	manager := Manager{Runner: runner, LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil }, InterfaceCIDRs: func() ([]InterfaceCIDR, error) {
		networkCalls++
		return []InterfaceCIDR{{Name: "hostile0", Prefix: mustPrefix("192.168.101.20/24")}}, nil
	}}
	definition := domain.ClusterDefinition{Helper: &domain.Helper{VMName: "okub-helper", StoragePool: "images", LibvirtURI: "qemu:///system"}}
	result, err := manager.Run(context.Background(), t.TempDir(), definition, "down", true)
	if err != nil || !result.Planned || networkCalls != 0 {
		t.Fatalf("result=%+v err=%v network calls=%d", result, err, networkCalls)
	}
}

func TestRuntimeFilesRejectUnsafeModesAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "up.log")
	if err := os.WriteFile(logPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateLog(logPath, []byte("new")); err == nil {
		t.Fatal("nonprivate old log accepted")
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateLog(logPath, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(logPath); err != nil || info.Mode().Perm() != 0o600 || !info.Mode().IsRegular() {
		t.Fatalf("log info=%v err=%v", info, err)
	}
	if err := os.Symlink(filepath.Join(dir, "target"), filepath.Join(dir, "state")); err != nil {
		t.Fatal(err)
	}
	if err := secureOptionalRuntimeFile(filepath.Join(dir, "state")); err == nil {
		t.Fatal("runtime symlink accepted")
	}
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".local"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(repo, ".local", "fedora-helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivatePath(repo, ".local", "fedora-helper"); err == nil {
		t.Fatal("nonprivate runtime directory accepted")
	}
}

func TestSecurePlanTightensOpenTofuMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.tfplan")
	if err := os.WriteFile(path, []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := securePlan(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("plan mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestSecurePlanRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("plan"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "plan.tfplan")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := securePlan(link); err == nil {
		t.Fatal("expected symlink plan rejection")
	}
}

func TestSecureManagedRuntimeFileTightensModeAndRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "terraform.tfstate")
	if err := os.WriteFile(state, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := secureManagedRuntimeFile(state); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(state)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("state info=%v err=%v", info, err)
	}
	if err := os.Symlink(state, filepath.Join(dir, "state-link")); err != nil {
		t.Fatal(err)
	}
	if err := secureManagedRuntimeFile(filepath.Join(dir, "state-link")); err == nil {
		t.Fatal("expected symlink state rejection")
	}
}

func mustPrefix(value string) netip.Prefix {
	return netip.MustParsePrefix(value)
}

func TestValidatePlanAllowsOnlyMissingCreates(t *testing.T) {
	changes := []string{}
	for address := range ownedAddresses {
		changes = append(changes, fmt.Sprintf(`{"address":%q,"change":{"actions":["create"],"before":null,"after":{"running":true}}}`, address))
	}
	plan := []byte(`{"resource_changes":[` + strings.Join(changes, ",") + `]}`)
	if changed, err := validatePlan(plan, map[string]bool{}); err != nil || !changed {
		t.Fatalf("safe create rejected: changed=%t err=%v", changed, err)
	}
	unsafe := []byte(`{"resource_changes":[{"address":"libvirt_domain.helper","change":{"actions":["delete","create"],"before":{},"after":{"running":true}}}]}`)
	if _, err := validatePlan(unsafe, map[string]bool{"libvirt_domain.helper": true}); err == nil {
		t.Fatal("replacement accepted")
	}
}

func TestValidatePlanAcceptsEveryResumableSubset(t *testing.T) {
	addresses := make([]string, 0, len(ownedAddresses))
	for address := range ownedAddresses {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	for mask := 0; mask < (1 << len(addresses)); mask++ {
		present := map[string]bool{}
		changes := []string{}
		for i, address := range addresses {
			action := "create"
			before := "null"
			after := `{}`
			if mask&(1<<i) != 0 {
				present[address] = true
				action = "no-op"
				before = `{}`
			}
			if address == "libvirt_domain.helper" && action == "create" {
				after = `{"running":true}`
			}
			changes = append(changes, fmt.Sprintf(`{"address":%q,"change":{"actions":[%q],"before":%s,"after":%s}}`, address, action, before, after))
		}
		changed, err := validatePlan([]byte(`{"resource_changes":[`+strings.Join(changes, ",")+`]}`), present)
		if err != nil {
			t.Fatalf("mask %06b: %v", mask, err)
		}
		if changed != (mask != (1<<len(addresses))-1) {
			t.Fatalf("mask %06b changed=%t", mask, changed)
		}
	}
}

func TestCommandFailurePreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := commandFailure(ctx, errors.New("secret output"), "safe"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestRenderedCloudInitPassesLifecycleSemantics(t *testing.T) {
	const config = `apiVersion: okub.io/v1alpha1
kind: Cluster
metadata: {name: lab}
cluster: {baseDomain: example.test, version: "4.20", architecture: x86_64, topology: sno}
platform: {type: none}
network: {machineCIDR: 192.168.101.0/24, clusterCIDR: 10.128.0.0/14, hostPrefix: 23, serviceCIDR: 172.30.0.0/16, dnsServers: [192.168.101.1], gateway: 192.168.101.1, apiVIP: 192.168.101.10, ingressVIP: 192.168.101.11}
hosts:
- {name: master-0, role: master, interfaces: [{name: eno1, macAddress: "52:54:00:00:00:10"}], address: 192.168.101.20/24, installationDisk: /dev/vda}
boot: {method: iso}
credentials: {pullSecretFile: pull.json, sshPublicKeyFile: id.pub}
helper: {fedoraRelease: 44, vmName: okub-helper, storagePool: images, wanMAC: "02:00:00:00:00:01", lanMAC: "02:00:00:00:00:02"}
`
	repo := t.TempDir()
	validator := application.ConfigurationValidator{Source: configBytes(config), CacheDirectory: repo}
	rendered, report, err := application.RenderHelper("ignored", repo, validator)
	if err != nil || !report.Valid {
		t.Fatalf("render err=%v report=%#v", err, report)
	}
	validated, err := validator.Validate("ignored")
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{rendered.UserData, rendered.MetaData, rendered.NetworkConfig}
	if err = validateCloudInputs(paths, *validated.Definition.Helper, validated.Definition); err != nil {
		t.Fatal(err)
	}
}
