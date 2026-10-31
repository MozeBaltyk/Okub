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

func TestRenderPXEIncludesAuthoritativeArtifactBaseURL(t *testing.T) {
	definition := domain.ClusterDefinition{
		Cluster: domain.Cluster{Architecture: "x86_64"},
		Boot:    domain.Boot{Method: "pxe", ArtifactsBaseURL: "http://192.168.100.10:8080/artifacts/pxe"},
		Hosts: []domain.Host{{
			Name: "master-0", Role: "master", Address: "192.168.100.20/24", InstallationDisk: "/dev/vda",
			Interfaces: []domain.Interface{{Name: "eno1", MACAddress: "52:54:00:00:00:20"}},
		}},
	}
	files, err := renderInstallerFiles(definition, "{}", "ssh-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Name == "agent-config.yaml" {
			if !strings.Contains(string(file.Content), "bootArtifactsBaseURL: http://192.168.100.10:8080/artifacts/pxe") {
				t.Fatalf("agent-config is missing PXE base URL:\n%s", file.Content)
			}
			return
		}
	}
	t.Fatal("agent-config.yaml was not rendered")
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

func TestRenderCompactAndHAContracts(t *testing.T) {
	tests := []struct {
		name, platform, workers, extra string
		hosts                          []string
	}{
		{name: "compact", platform: "    none: {}", workers: "replicas: 0", hosts: []string{"master-0", "master-1", "master-2"}},
		{name: "ha", platform: "    baremetal:", workers: "replicas: 2", extra: "bootArtifactsBaseURL: http://192.168.103.5:8080/artifacts/pxe", hosts: []string{"master-0", "master-1", "master-2", "worker-0", "worker-1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := os.ReadFile("../../examples/" + test.name + ".yaml")
			if err != nil {
				t.Fatal(err)
			}
			publisher := &publisherStub{}
			workflow := InstallerInputRenderer{
				Validator: ConfigurationValidator{Source: sourceStub{data: config}, CacheDirectory: "/cache", HomeDirectory: "/home/tester"},
				Credentials: credentialStub{values: map[string][]byte{
					"/home/tester/.config/okub/pull-secret.json": []byte(`{"auths":{}}`),
					"/home/tester/.ssh/id_ed25519.pub":           []byte("ssh-ed25519 AAAATEST"),
				}},
				Publisher: publisher,
			}
			if _, report, err := workflow.Render("ignored", "", false, true); err != nil || !report.Valid {
				t.Fatalf("Render() report = %#v, error = %v", report, err)
			}
			files := map[string]string{}
			for _, file := range publisher.files {
				files[file.Name] = string(file.Content)
			}
			install, agent := files["install-config.yaml"], files["agent-config.yaml"]
			for _, required := range []string{"replicas: 3", test.workers, test.platform} {
				if !strings.Contains(install, required) {
					t.Errorf("install-config missing %q:\n%s", required, install)
				}
			}
			if test.name == "ha" && (!strings.Contains(install, "192.168.103.10") || !strings.Contains(install, "192.168.103.11")) {
				t.Errorf("baremetal VIPs missing:\n%s", install)
			}
			if test.extra != "" && !strings.Contains(agent, test.extra) {
				t.Errorf("agent-config missing %q:\n%s", test.extra, agent)
			}
			position := -1
			for _, host := range test.hosts {
				next := strings.Index(agent, "hostname: "+host)
				if next <= position {
					t.Fatalf("hosts are not deterministically ordered: %v\n%s", test.hosts, agent)
				}
				position = next
			}
			first := append([]ports.GeneratedFile(nil), publisher.files...)
			if _, _, err := workflow.Render("ignored", "", false, true); err != nil || !generatedFilesEqual(first, publisher.files) {
				t.Fatalf("repeated render changed output: %v", err)
			}
		})
	}
}

func generatedFilesEqual(a, b []ports.GeneratedFile) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || !bytes.Equal(a[i].Content, b[i].Content) {
			return false
		}
	}
	return true
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
