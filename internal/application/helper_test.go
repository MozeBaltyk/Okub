package application

import (
	"bytes"
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestRenderHelperIsDeterministicAndSecretFree(t *testing.T) {
	repo := t.TempDir()
	validator := ConfigurationValidator{Source: sourceStub{data: []byte(helperTestConfig())}, CacheDirectory: repo}
	first, report, err := RenderHelper("ignored", repo, validator)
	if err != nil || !report.Valid {
		t.Fatalf("render error=%v report=%#v", err, report)
	}
	before, err := os.ReadFile(first.UserData)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := RenderHelper("ignored", repo, validator)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(second.UserData)
	if !bytes.Equal(before, after) {
		t.Fatal("render is nondeterministic")
	}
	text := string(before)
	for _, forbidden := range []string{"pull-secret", "PRIVATE KEY", "password:"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("secret marker %q present", forbidden)
		}
	}
	for _, required := range []string{"dnsmasq", "haproxy", "nginx", "encoding: b64", "resize_rootfs: true"} {
		if !strings.Contains(text, required) {
			t.Errorf("missing %q", required)
		}
	}
	for _, path := range []string{first.UserData, first.MetaData, first.NetworkConfig} {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode=%o", path, info.Mode().Perm())
		}
	}
}

func TestHelperSetupIsOneFailFastScriptAndPXERemainsDisabled(t *testing.T) {
	repo := t.TempDir()
	config := strings.Replace(helperTestConfig(), "boot: {method: iso}", "boot: {method: pxe, artifactsBaseURL: http://192.168.1.10:8080/artifacts/pxe}", 1)
	result, report, err := RenderHelper("ignored", repo, ConfigurationValidator{Source: sourceStub{data: []byte(config)}, CacheDirectory: repo})
	if err != nil || !report.Valid {
		t.Fatalf("render error=%v report=%#v", err, report)
	}
	data, err := os.ReadFile(result.UserData)
	if err != nil {
		t.Fatal(err)
	}
	var user struct {
		RunCmd     [][]string `yaml:"runcmd"`
		WriteFiles []struct {
			Path, Content string
		} `yaml:"write_files"`
	}
	if err := yaml.Unmarshal(data, &user); err != nil {
		t.Fatal(err)
	}
	if len(user.RunCmd) != 1 || !bytes.Equal([]byte(strings.Join(user.RunCmd[0], "\x00")), []byte("/usr/local/sbin/okub-helper-setup")) {
		t.Fatalf("runcmd=%v", user.RunCmd)
	}
	var script, dnsmasq, nginx string
	for _, file := range user.WriteFiles {
		decoded, decodeErr := base64.StdEncoding.DecodeString(file.Content)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		switch file.Path {
		case "/usr/local/sbin/okub-helper-setup":
			script = string(decoded)
		case "/etc/dnsmasq.d/okub.conf":
			dnsmasq = string(decoded)
		case "/etc/nginx/nginx.conf":
			nginx = string(decoded)
		}
	}
	ordered := []string{"set -euo pipefail", "dnf -y install", "dnsmasq --test", "firewall-offline-cmd --check-config", "systemctl enable --now firewalld.service"}
	position := -1
	for _, marker := range ordered {
		next := strings.Index(script, marker)
		if next <= position {
			t.Fatalf("setup marker %q is absent or out of order", marker)
		}
		position = next
	}
	if strings.Contains(dnsmasq, "dhcp-boot=") || !strings.Contains(dnsmasq, "tftp-root=/srv/okub/tftp") {
		t.Fatalf("PXE must be disabled with TFTP directory retained:\n%s", dnsmasq)
	}
	if !strings.Contains(nginx, "listen 192.168.1.1:8080;") || strings.Contains(nginx, "listen 80") {
		t.Fatalf("nginx must not conflict with HAProxy ingress listeners:\n%s", nginx)
	}
	if !strings.Contains(script, "--zone=internal --add-source=192.168.1.0/24") || strings.Contains(script, "@LAN_CIDR@") {
		t.Fatalf("setup must classify the validated LAN CIDR in firewalld:\n%s", script)
	}
}
