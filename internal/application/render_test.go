package application

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
)

type credentialStub struct {
	values map[string][]byte
	err    error
	calls  *[]string
}

func (s credentialStub) ReadCredential(path string) ([]byte, error) {
	if s.calls != nil {
		*s.calls = append(*s.calls, path)
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.values[path], nil
}

type publisherStub struct {
	output string
	files  []ports.GeneratedFile
}

func (p *publisherStub) Publish(output string, files []ports.GeneratedFile, _, _ bool) (string, error) {
	p.output = output
	p.files = files
	return "create", nil
}

func TestRenderValidatesAndResolvesOutputOverride(t *testing.T) {
	config, err := os.ReadFile("../../examples/sno.yaml")
	if err != nil {
		t.Fatal(err)
	}
	config = []byte(strings.Replace(string(config), "  rendezvousIP: 192.168.101.20\n", "", 1))
	credentials := credentialStub{values: map[string][]byte{
		"/home/tester/.config/okub/pull-secret.json": []byte(`{"auths":{}}`),
		"/home/tester/.ssh/ed25519.pub":              []byte("ssh-ed25519 AAAATEST"),
	}}
	publisher := &publisherStub{}
	workflow := InstallerInputRenderer{
		Validator:   ConfigurationValidator{Source: sourceStub{data: config}, CacheDirectory: "/cache", HomeDirectory: "/home/tester"},
		Credentials: credentials, Publisher: publisher,
	}
	config = []byte(strings.Replace(string(config), "credentials:", "artifacts: {outputDirectory: ../invalid}\ncredentials:", 1))
	workflow.Validator.Source = sourceStub{data: config}
	if _, report, err := workflow.Render("ignored", "~/override", false, false); err != nil || !report.Valid {
		t.Fatalf("Render(valid override) report = %#v, error = %v", report, err)
	}
	if publisher.output != "/home/tester/override" {
		t.Fatalf("published to %q", publisher.output)
	}

	calls := []string{}
	workflow.Credentials = credentialStub{calls: &calls}
	if _, report, err := workflow.Render("ignored", "../outside", false, false); err != nil || report.Valid {
		t.Fatalf("Render(invalid override) report = %#v, error = %v", report, err)
	}
	if len(calls) != 0 {
		t.Fatalf("invalid override read credentials: %v", calls)
	}
}

func TestRenderDHCPUsesExplicitRendezvousIP(t *testing.T) {
	config, err := os.ReadFile("../../examples/sno.yaml")
	if err != nil {
		t.Fatal(err)
	}
	config = []byte(strings.Replace(string(config), "    address: 192.168.101.20/24\n", "", 1))
	if start := bytes.Index(config, []byte("helper:\n")); start >= 0 {
		if end := bytes.Index(config[start:], []byte("\ncredentials:")); end >= 0 {
			config = append(config[:start], config[start+end+1:]...)
		}
	}
	publisher := &publisherStub{}
	workflow := InstallerInputRenderer{
		Validator: ConfigurationValidator{Source: sourceStub{data: config}, CacheDirectory: "/cache", HomeDirectory: "/home/tester"},
		Credentials: credentialStub{values: map[string][]byte{
			"/home/tester/.config/okub/pull-secret.json": []byte("{\"auths\":{}}\n"),
			"/home/tester/.ssh/ed25519.pub":              []byte("ssh-ed25519 AAAATEST okub-test\n"),
		}},
		Publisher: publisher,
	}
	_, report, err := workflow.Render("ignored", "", false, true)
	if err != nil || !report.Valid {
		t.Fatalf("Render() report = %#v, error = %v", report, err)
	}
	var agent string
	for _, file := range publisher.files {
		if file.Name == "agent-config.yaml" {
			agent = string(file.Content)
		}
	}
	if !strings.Contains(agent, "rendezvousIP: 192.168.101.20") || strings.Contains(agent, "networkConfig:") {
		t.Fatalf("DHCP agent-config is invalid:\n%s", agent)
	}
}

func TestRenderSelectsLexicallyFirstStaticMasterForRendezvous(t *testing.T) {
	definition := domain.ClusterDefinition{
		Cluster: domain.Cluster{Architecture: "x86_64"},
		Network: domain.Network{MachineCIDR: "192.168.100.0/24"},
		Hosts: []domain.Host{
			{Name: "master-z", Role: "master", Address: "192.168.100.30/24", InstallationDisk: "/dev/vda", Interfaces: []domain.Interface{{Name: "eno1", MACAddress: "52:54:00:00:00:30"}}},
			{Name: "master-a", Role: "master", Address: "192.168.100.20/24", InstallationDisk: "/dev/vdb", Interfaces: []domain.Interface{{Name: "eno1", MACAddress: "52:54:00:00:00:20"}}},
		},
	}
	files, err := renderInstallerFiles(definition, "{}", "ssh-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Name == "agent-config.yaml" && !strings.Contains(string(file.Content), "rendezvousIP: 192.168.100.20") {
			t.Fatalf("agent-config did not select master-a:\n%s", file.Content)
		}
	}
}

func TestRenderInstallerFilesMatchesSNOGolden(t *testing.T) {
	config, err := os.ReadFile("../../examples/sno.yaml")
	if err != nil {
		t.Fatal(err)
	}
	publisher := &publisherStub{}
	workflow := InstallerInputRenderer{
		Validator: ConfigurationValidator{Source: sourceStub{data: config}, CacheDirectory: "/cache", HomeDirectory: "/home/tester"},
		Credentials: credentialStub{values: map[string][]byte{
			"/home/tester/.config/okub/pull-secret.json": []byte("{\"auths\":{}}\n"),
			"/home/tester/.ssh/ed25519.pub":              []byte("ssh-ed25519 AAAATEST okub-test\n"),
		}},
		Publisher: publisher,
	}
	first, report, err := workflow.Render("ignored", "", false, false)
	if err != nil || !report.Valid || first.Action != "create" {
		t.Fatalf("Render() result = %#v, report = %#v, error = %v", first, report, err)
	}
	for _, generated := range publisher.files {
		golden, err := os.ReadFile("testdata/sno-" + generated.Name)
		if err != nil {
			t.Fatal(err)
		}
		if string(generated.Content) != string(golden) {
			t.Errorf("%s differs from golden\n--- got ---\n%s\n--- want ---\n%s", generated.Name, generated.Content, golden)
		}
	}
	previous := append([]byte(nil), publisher.files[0].Content...)
	if _, _, err := workflow.Render("ignored", "", false, false); err != nil || string(previous) != string(publisher.files[0].Content) {
		t.Fatalf("repeated render is not deterministic: %v", err)
	}
}

func TestRenderCredentialErrorDoesNotReadSecondCredential(t *testing.T) {
	config := []byte(`apiVersion: okub.io/v1alpha1
kind: Cluster
metadata: {name: lab}
cluster: {baseDomain: example.test, version: "4.20", architecture: x86_64, topology: sno}
platform: {type: none}
network: {machineCIDR: 192.168.1.0/24, clusterCIDR: 10.128.0.0/14, hostPrefix: 23, serviceCIDR: 172.30.0.0/16, dnsServers: [192.168.1.1], gateway: 192.168.1.1}
hosts: [{name: master-0, role: master, interfaces: [{name: eno1, macAddress: "52:54:00:00:00:10"}], address: 192.168.1.20/24, installationDisk: /dev/vda}]
boot: {method: iso}
credentials: {pullSecretFile: missing-secret, sshPublicKeyFile: id.pub}
`)
	var calls []string
	workflow := InstallerInputRenderer{
		Validator:   ConfigurationValidator{Source: sourceStub{data: config}, CacheDirectory: "/cache"},
		Credentials: credentialStub{err: errors.New("file is unavailable"), calls: &calls}, Publisher: &publisherStub{},
	}
	_, _, err := workflow.Render("ignored", "", false, false)
	if err == nil || !strings.Contains(err.Error(), "missing-secret") || len(calls) != 1 {
		t.Fatalf("credential error = %q", err)
	}
}
