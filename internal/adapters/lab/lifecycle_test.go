package lab

import (
	"context"
	"encoding/xml"
	"errors"
	"strings"
	"testing"

	"github.com/mozebaltyk/Okub/internal/domain"
)

type runnerFunc func(context.Context, string, []string, ...string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, directory string, env []string, args ...string) ([]byte, error) {
	return f(ctx, directory, env, args...)
}

func TestRunRejectsNoKVMBeforeToolsOrArtifacts(t *testing.T) {
	lookups := 0
	m := Manager{
		OpenKVM:  func() error { return errors.New("no kvm") },
		LookPath: func(string) (string, error) { lookups++; return "", nil },
	}
	definition := domain.ClusterDefinition{
		Platform: domain.Platform{Type: "none"}, Cluster: domain.Cluster{Topology: "sno"},
		Hosts: []domain.Host{{Interfaces: []domain.Interface{{MACAddress: "52:54:00:00:00:10"}}}},
	}
	_, err := m.Run(context.Background(), t.TempDir(), definition, "up", Request{RunID: "12345678-1234-4234-8234-123456789abc"}, false)
	if err == nil || !strings.Contains(err.Error(), "KVM") || lookups != 0 {
		t.Fatalf("err=%v tool lookups=%d", err, lookups)
	}
}

func TestLabBridgeCIDRUsesFinalUsableAddress(t *testing.T) {
	got, err := labBridgeCIDR("192.168.122.0/24")
	if err != nil || got != "192.168.122.254/24" {
		t.Fatalf("bridge=%q error=%v", got, err)
	}
}

func TestValidatePlanAllowsOnlyExactCreatesOrDeletes(t *testing.T) {
	create := []byte(`{"resource_changes":[{"address":"libvirt_network.lab","change":{"actions":["create"]}},{"address":"libvirt_volume.root","change":{"actions":["create"]}},{"address":"libvirt_domain.sno","change":{"actions":["create"]}}]}`)
	if changed, err := validatePlan(create, "up"); err != nil || !changed {
		t.Fatalf("safe create rejected: changed=%t err=%v", changed, err)
	}
	replace := []byte(`{"resource_changes":[{"address":"libvirt_network.lab","change":{"actions":["no-op"]}},{"address":"libvirt_volume.root","change":{"actions":["delete","create"]}},{"address":"libvirt_domain.sno","change":{"actions":["no-op"]}}]}`)
	if _, err := validatePlan(replace, "up"); err == nil {
		t.Fatal("replacement accepted")
	}
	foreign := []byte(`{"resource_changes":[{"address":"libvirt_domain.foreign","change":{"actions":["delete"]}}]}`)
	if _, err := validatePlan(foreign, "down"); err == nil {
		t.Fatal("foreign resource accepted")
	}
}

func TestOwnedStateRejectsUnexpectedAddress(t *testing.T) {
	if _, err := ownedState([]byte("libvirt_domain.sno\nlibvirt_domain.someone_else\n")); err == nil {
		t.Fatal("unexpected state address accepted")
	}
}

func TestCleanEnvironmentDropsTerraformInjection(t *testing.T) {
	got := strings.Join(cleanEnvironment([]string{"PATH=/bin", "TF_CLI_ARGS=-destroy", "TOFU_DATA_DIR=/tmp/evil"}, []string{"TF_IN_AUTOMATION=1"}), " ")
	if strings.Contains(got, "-destroy") || strings.Contains(got, "/tmp/evil") || !strings.Contains(got, "TF_IN_AUTOMATION=1") {
		t.Fatalf("environment=%q", got)
	}
}

func TestInspectIdentityVerifiesEveryLiveResourceAndAttachment(t *testing.T) {
	const runID = "12345678-1234-4234-8234-123456789abc"
	runner := runnerFunc(func(_ context.Context, _ string, _ []string, args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		switch {
		case strings.Contains(command, "tofu show -json"):
			return []byte(`{"values":{"outputs":{"run_id":{"value":"12345678-1234-4234-8234-123456789abc"}},"root_module":{"resources":[{"address":"libvirt_domain.sno","values":{"name":"okub-lab-123456781234-sno","id":"domain-id"}},{"address":"libvirt_network.lab","values":{"name":"okub-lab-123456781234-net","id":"network-id"}},{"address":"libvirt_volume.root","values":{"name":"okub-lab-123456781234-root.qcow2","id":"/pool/root.qcow2","pool":"default"}}]}}}`), nil
		case strings.Contains(command, "domuuid"):
			return []byte("domain-id\n"), nil
		case strings.Contains(command, "net-uuid"):
			return []byte("network-id\n"), nil
		case strings.Contains(command, "vol-path"):
			return []byte("/pool/root.qcow2\n"), nil
		case strings.Contains(command, "dumpxml"):
			return []byte(`<domain><description>Okub validation lab owner=` + runID + `</description><devices><disk><source file="/pool/root.qcow2"/></disk><interface><source network="okub-lab-123456781234-net"/></interface></devices></domain>`), nil
		case strings.Contains(command, "list --all --name"):
			return []byte("okub-lab-123456781234-sno\n"), nil
		default:
			return nil, errors.New("unexpected command: " + command)
		}
	})
	if err := inspectIdentity(context.Background(), runner, "/module", nil, "tofu", "virsh", runID); err != nil {
		t.Fatal(err)
	}
}

func TestDomainUsesRejectsExtraAttachments(t *testing.T) {
	var owned domainXML
	if err := xml.Unmarshal([]byte(`<domain><devices><disk><source file="/pool/root.qcow2"/></disk><interface><source network="owned"/></interface></devices></domain>`), &owned); err != nil {
		t.Fatal(err)
	}
	if !domainUses(owned.Devices.Disks, owned.Devices.Interfaces, "/pool/root.qcow2", "owned") {
		t.Fatal("exact owned attachments rejected")
	}
	var extra domainXML
	if err := xml.Unmarshal([]byte(`<domain><devices><disk><source file="/pool/root.qcow2"/></disk><disk device="disk"><source file="/pool/foreign.qcow2"/></disk><interface><source network="owned"/></interface></devices></domain>`), &extra); err != nil {
		t.Fatal(err)
	}
	if domainUses(extra.Devices.Disks, extra.Devices.Interfaces, "/pool/root.qcow2", "owned") {
		t.Fatal("foreign disk attachment accepted")
	}
}
