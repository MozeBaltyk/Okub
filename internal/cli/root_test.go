package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mozebaltyk/Okub/internal/domain"
)

func TestHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := execute(args)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			if !strings.Contains(stdout, "Usage:\n  okub [command]") || !strings.Contains(stdout, "version") {
				t.Fatalf("stdout does not contain root help:\n%s", stdout)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	stdout, stderr, code := execute([]string{"config", "validate", "--config", "../../examples/sno.yaml"})
	if code != 0 || stderr != "" || stdout != "configuration is valid\n" {
		t.Fatalf("stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	stdout, stderr, code = execute([]string{"config", "validate", "--json", "--config", "../../examples/sno.yaml"})
	var report domain.ValidationReport
	if code != 0 || stderr != "" || json.Unmarshal([]byte(stdout), &report) != nil || !report.Valid || report.Findings == nil {
		t.Fatalf("stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
}

func TestConfigValidateExitCodesAndStreams(t *testing.T) {
	stdout, stderr, code := execute([]string{"config", "validate", "--config", "does-not-exist.yaml"})
	if code != exitFilesystem || stdout != "" || !strings.Contains(stderr, "read configuration") {
		t.Fatalf("stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	stdout, stderr, code = execute([]string{"config", "validate", "--config", "../../tests/fixtures/invalid-config.yaml"})
	if code != exitValidation || stderr != "" || !strings.Contains(stdout, "config.schema") || !strings.Contains(stdout, "network.cidrOverlap") || !strings.Contains(stdout, "topology.sno") {
		t.Fatalf("stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	stdout, stderr, code = execute([]string{"config", "validate", "--config", "../../tests/fixtures/unsupported-version.yaml"})
	if code != exitInvalidUsage || stderr != "" || !strings.Contains(stdout, "apiVersion.unsupported") {
		t.Fatalf("stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
}

func TestConfigValidateJSONFailures(t *testing.T) {
	tests := []struct {
		name, fixture, stdout string
		code                  int
	}{
		{
			name: "invalid", fixture: "../../tests/fixtures/invalid-config.yaml", code: exitValidation,
			stdout: `{"valid":false,"findings":[{"id":"config.schema","severity":"error","path":"cluster.baseDomain","message":"value does not satisfy schema pattern"},{"id":"dns.domain","severity":"error","path":"cluster.baseDomain","message":"must be a DNS domain without a wildcard or trailing dot"},{"id":"config.schema","severity":"error","path":"hosts","message":"value does not satisfy schema minItems"},{"id":"field.required","severity":"error","path":"hosts","message":"at least one host is required"},{"id":"topology.sno","severity":"error","path":"hosts","message":"SNO requires exactly one master and no workers"},{"id":"config.schema","severity":"error","path":"metadata.name","message":"value does not satisfy schema pattern"},{"id":"dns.label","severity":"error","path":"metadata.name","message":"must be a lowercase DNS label"},{"id":"network.cidrOverlap","severity":"error","path":"network.clusterCIDR","message":"cluster CIDR overlaps machine CIDR"},{"id":"config.schema","severity":"error","path":"network.dnsServers","message":"value does not satisfy schema minItems"},{"id":"field.required","severity":"error","path":"network.dnsServers","message":"at least one DNS server is required"},{"id":"network.hostPrefix","severity":"error","path":"network.hostPrefix","message":"host prefix must be greater than 25 and no greater than 32"},{"id":"network.cidrOverlap","severity":"error","path":"network.serviceCIDR","message":"service CIDR overlaps machine CIDR"}]}` + "\n",
		},
		{
			name: "unsupported version", fixture: "../../tests/fixtures/unsupported-version.yaml", code: exitInvalidUsage,
			stdout: `{"valid":false,"findings":[{"id":"apiVersion.unsupported","severity":"error","path":"apiVersion","message":"API version is not supported"}]}` + "\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, code := execute([]string{"config", "validate", "--json", "--config", test.fixture})
			if code != test.code || stdout != test.stdout || stderr != "" {
				t.Fatalf("stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := execute(args)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			if stdout != "okub test-version\n" {
				t.Fatalf("stdout = %q, want version", stdout)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
		})
	}
}

func TestInvalidUsage(t *testing.T) {
	stdout, stderr, code := execute([]string{"bogus"})
	if code != exitInvalidUsage {
		t.Fatalf("exit code = %d, want %d", code, exitInvalidUsage)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if stderr != "okub: unknown command \"bogus\" for \"okub\"\n" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func execute(args []string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := Execute(args, "test-version", &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}
