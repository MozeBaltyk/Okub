package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cacheadapter "github.com/mozebaltyk/Okub/internal/adapters/cache"
	"github.com/mozebaltyk/Okub/internal/adapters/filesystem"
	"github.com/mozebaltyk/Okub/internal/application"
	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
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
	for _, topology := range []string{"sno", "compact", "ha"} {
		t.Run(topology, func(t *testing.T) {
			config := "../../examples/" + topology + ".yaml"
			stdout, stderr, code := execute([]string{"config", "validate", "--config", config})
			if code != 0 || stderr != "" || stdout != "configuration is valid\n" {
				t.Fatalf("stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
			}

			stdout, stderr, code = execute([]string{"config", "validate", "--json", "--config", config})
			var report domain.ValidationReport
			if code != 0 || stderr != "" || json.Unmarshal([]byte(stdout), &report) != nil || !report.Valid || report.Findings == nil {
				t.Fatalf("stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
			}
		})
	}
}

func TestConfigValidateCompactAndHANegativeFixtures(t *testing.T) {
	tests := []struct{ fixture, id string }{
		{"compact-invalid-host-count.yaml", "topology.compact"},
		{"ha-invalid-host-count.yaml", "topology.ha"},
		{"ha-invalid-network.yaml", "network.staticDNS"},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			stdout, stderr, code := execute([]string{"config", "validate", "--json", "--config", "../../tests/fixtures/" + test.fixture})
			if code != exitValidation || stderr != "" || !strings.Contains(stdout, `"id":"`+test.id+`"`) {
				t.Fatalf("stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
			}
		})
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
			stdout: `{"valid":false,"findings":[{"id":"config.schema","severity":"error","path":"cluster.baseDomain","message":"value does not satisfy schema pattern"},{"id":"dns.domain","severity":"error","path":"cluster.baseDomain","message":"must be a DNS domain without a wildcard or trailing dot"},{"id":"config.schema","severity":"error","path":"hosts","message":"value does not satisfy schema minItems"},{"id":"field.required","severity":"error","path":"hosts","message":"at least one host is required"},{"id":"topology.sno","severity":"error","path":"hosts","message":"SNO requires exactly one master and no workers"},{"id":"config.schema","severity":"error","path":"metadata.name","message":"value does not satisfy schema pattern"},{"id":"dns.label","severity":"error","path":"metadata.name","message":"must be a lowercase DNS label"},{"id":"network.cidrOverlap","severity":"error","path":"network.clusterCIDR","message":"cluster CIDR overlaps machine CIDR"},{"id":"config.schema","severity":"error","path":"network.dnsServers","message":"value does not satisfy schema minItems"},{"id":"field.required","severity":"error","path":"network.dnsServers","message":"at least one DNS server is required"},{"id":"network.hostPrefix","severity":"error","path":"network.hostPrefix","message":"host prefix must be greater than 25 and no greater than 32"},{"id":"field.required","severity":"error","path":"network.rendezvousIP","message":"rendezvous IP is required when no master has a static address"},{"id":"network.cidrOverlap","severity":"error","path":"network.serviceCIDR","message":"service CIDR overlaps machine CIDR"}]}` + "\n",
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

func TestConfigRenderJSONDryRunOverwriteAndSecrets(t *testing.T) {
	root := t.TempDir()
	pullSecret := filepath.Join(root, "pull-secret.json")
	sshKey := filepath.Join(root, "id.pub")
	output := filepath.Join(root, "output")
	config := filepath.Join(root, "okub.yaml")
	secretOne := "FIRST-CREDENTIAL-MARKER"
	secretTwo := "SECOND-CREDENTIAL-MARKER"
	if err := os.WriteFile(pullSecret, []byte(`{"auths":{"example":{"auth":"`+secretOne+`"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sshKey, []byte("ssh-ed25519 AAAATEST test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeRenderConfig(t, config, pullSecret, sshKey)

	stdout, stderr, code := execute([]string{"config", "render", "--json", "--dry-run", "--config", config, "--output", output})
	wantDryRun := `{"action":"create","outputDirectory":` + quotedJSON(filepath.Join(output, "generated")) + `,"files":["agent-config.yaml","install-config.yaml"],"dryRun":true}` + "\n"
	if code != 0 || stderr != "" || stdout != wantDryRun {
		t.Fatalf("dry-run stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("dry-run created output: %v", err)
	}

	stdout, stderr, code = execute([]string{"config", "render", "--json", "--config", config, "--output", output})
	if code != 0 || stderr != "" || strings.Contains(stdout, secretOne) || !strings.Contains(stdout, `"action":"create"`) {
		t.Fatalf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	stdout, stderr, code = execute([]string{"config", "render", "--config", config, "--output", output})
	if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "unchanged installer configuration") {
		t.Fatalf("unchanged stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	if err := os.WriteFile(pullSecret, []byte(`{"auths":{"example":{"auth":"`+secretTwo+`"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = execute([]string{"config", "render", "--dry-run", "--config", config, "--output", output})
	if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "would update installer configuration") {
		t.Fatalf("update dry-run stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	installed, err := os.ReadFile(filepath.Join(output, "generated", "install-config.yaml"))
	if err != nil || !strings.Contains(string(installed), secretOne) || strings.Contains(string(installed), secretTwo) {
		t.Fatalf("dry-run changed installed file: error = %v", err)
	}
	stdout, stderr, code = execute([]string{"config", "render", "--json", "--config", config, "--output", output})
	if code != exitFilesystem || stdout != "" || !strings.Contains(stderr, "--force") || strings.Contains(stderr, secretOne) || strings.Contains(stderr, secretTwo) {
		t.Fatalf("refusal stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	stdout, stderr, code = execute([]string{"config", "render", "--force", "--config", config, "--output", output})
	if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "updated installer configuration") {
		t.Fatalf("force stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
}

func TestConfigRenderCredentialReadError(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "okub.yaml")
	missing := filepath.Join(root, "missing-pull-secret")
	writeRenderConfig(t, config, missing, filepath.Join(root, "missing-key"))
	stdout, stderr, code := execute([]string{"config", "render", "--config", config, "--output", filepath.Join(root, "out")})
	if code != exitFilesystem || stdout != "" || !strings.Contains(stderr, "read credential") || !strings.Contains(stderr, missing) {
		t.Fatalf("stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
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

func TestWorkspaceRootDiscoveryAnchorsDefaultsWithoutChangingDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "one", "two")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := workspaceRootFrom(nested); got != root {
		t.Fatalf("workspace root=%q, want %q", got, root)
	}
	outside := filepath.Join(t.TempDir(), "one", "two")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := workspaceRootFrom(outside); got != outside {
		t.Fatalf("outside fallback=%q, want %q", got, outside)
	}
}

func TestDryRunIsGlobalButRejectedByReadOnlyCommands(t *testing.T) {
	stdout, stderr, code := execute([]string{"--dry-run", "config", "validate", "--config", "../../examples/sno.yaml"})
	if code != exitInvalidUsage || stdout != "" || !strings.Contains(stderr, "only supported") {
		t.Fatalf("stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
}

func TestPreflightCLIJSONSuccessAndOfflineFailure(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "okub.yaml")
	pull := filepath.Join(root, "pull-secret.json")
	key := filepath.Join(root, "id.pub")
	writeRenderConfig(t, config, pull, key)
	fake := &cliPreflightFake{}
	factory := func(validator application.ConfigurationValidator) application.Preflight {
		return application.Preflight{Validator: validator, DNS: fake, Environment: fake, Cache: fake}
	}

	stdout, stderr, code := executeWithFactory([]string{"preflight", "run", "--json", "--config", config}, factory)
	var report domain.PreflightReport
	if code != 0 || stderr != "" || json.Unmarshal([]byte(stdout), &report) != nil || !report.Valid || len(report.Checks) == 0 {
		t.Fatalf("stdout=%q stderr=%q code=%d report=%#v", stdout, stderr, code, report)
	}
	stdout, stderr, code = executeWithFactory([]string{"preflight", "run", "--offline", "--config", config}, factory)
	if code != exitValidation || stderr != "" || !strings.Contains(stdout, "fail [offline.verifiedCache]") || fake.endpointCalls != 3 {
		t.Fatalf("stdout=%q stderr=%q code=%d endpointCalls=%d", stdout, stderr, code, fake.endpointCalls)
	}
}

func TestImagesSyncCLIContracts(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "okub.yaml")
	writeRenderConfig(t, config, filepath.Join(root, "pull"), filepath.Join(root, "key"))
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "credentials:\n", "artifacts:\n  cacheDirectory: "+root+"/cache\ncredentials:\n", 1))
	if err := os.WriteFile(config, data, 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := newCLIImageFixture()
	cache := cacheadapter.Release{}
	factories := commandFactories{images: func(validator application.ConfigurationValidator) application.ImageSync {
		return application.ImageSync{Validator: validator, Resolver: fixture, Downloader: fixture, Cache: cache}
	}}
	stdout, stderr, code := executeWithFactories([]string{"images", "sync", "--json", "--config", config}, factories)
	var result application.ImageSyncResult
	if code != 0 || stderr != "" || json.Unmarshal([]byte(stdout), &result) != nil || result.Cached || len(result.Inputs) != 3 {
		t.Fatalf("stdout=%q stderr=%q code=%d result=%#v", stdout, stderr, code, result)
	}
	stdout, stderr, code = executeWithFactories([]string{"images", "sync", "--offline", "--config", config}, factories)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "already verified release 4.20.1-okd") || fixture.resolveCalls != 1 || fixture.downloadCalls != 3 {
		t.Fatalf("stdout=%q stderr=%q code=%d resolves=%d downloads=%d", stdout, stderr, code, fixture.resolveCalls, fixture.downloadCalls)
	}
	if err := os.RemoveAll(filepath.Join(root, "cache")); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = executeWithFactories([]string{"images", "sync", "--offline", "--config", config}, factories)
	if code != exitFilesystem || stdout != "" || !strings.Contains(stderr, "verified cache operation failed") || !strings.Contains(stderr, "requested release is not available in the verified cache") {
		t.Fatalf("offline miss stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}

	bad := newCLIImageFixture()
	bad.resolveErr = errors.New("SECRET-MARKER endpoint failed")
	factories.images = func(validator application.ConfigurationValidator) application.ImageSync {
		return application.ImageSync{Validator: validator, Resolver: bad, Downloader: bad, Cache: cacheadapter.Release{}}
	}
	stdout, stderr, code = executeWithFactories([]string{"images", "sync", "--config", config, "--output", filepath.Join(root, "other")}, factories)
	if code != exitNetwork || stdout != "" || strings.Contains(stderr, "SECRET-MARKER") || !strings.Contains(stderr, "release or download operation failed") {
		t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func TestArtifactsAndMonitorCLIContracts(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "okub.yaml")
	writeRenderConfig(t, config, filepath.Join(root, "pull"), filepath.Join(root, "key"))
	release := cliBuildRelease()
	cache := cliBuildCache{release: release}
	installer := cliBuildInstaller{}
	preflight := &cliPreflightFake{}
	factories := commandFactories{
		preflight: func(validator application.ConfigurationValidator) application.Preflight {
			return application.Preflight{Validator: validator, DNS: preflight, Environment: preflight, Cache: preflight}
		},
		artifacts: func(validator application.ConfigurationValidator) application.ArtifactBuilder {
			return application.ArtifactBuilder{
				Validator: validator, Credentials: cliBuildCredentials{}, Cache: cache,
				Extractor: cliBuildExtractor{}, Installer: installer, Store: filesystem.ArtifactStore{}, OkubVersion: "test-version",
			}
		},
		monitor: func(validator application.ConfigurationValidator) application.InstallMonitor {
			return application.InstallMonitor{
				Validator: validator, Credentials: cliBuildCredentials{}, Cache: cache,
				Extractor: cliBuildExtractor{}, Installer: installer, Store: filesystem.ArtifactStore{},
			}
		},
	}
	output := filepath.Join(root, "output")
	stdout, stderr, code := executeWithFactories([]string{"artifacts", "build", "--type", "iso", "--json", "--config", config, "--output", output}, factories)
	var build application.ArtifactBuildResult
	if code != 0 || stderr != "" || json.Unmarshal([]byte(stdout), &build) != nil || build.Type != "iso" || len(build.Artifacts) != 1 {
		t.Fatalf("stdout=%q stderr=%q code=%d result=%#v", stdout, stderr, code, build)
	}

	stdout, stderr, code = executeWithFactories([]string{"install", "monitor", "--config", config, "--output", output}, factories)
	if code != 0 || stderr != "" || stdout != "installation monitor success\n" {
		t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	stdout, stderr, code = executeWithFactories([]string{"install", "monitor", "--force", "--json", "--config", config, "--output", output}, factories)
	var monitor application.MonitorResult
	if code != 0 || stderr != "" || json.Unmarshal([]byte(stdout), &monitor) != nil || monitor.Outcome != "success" {
		t.Fatalf("stdout=%q stderr=%q code=%d result=%#v", stdout, stderr, code, monitor)
	}

	stdout, stderr, code = executeWithFactories([]string{"artifacts", "build", "--type", "invalid"}, factories)
	if code != exitInvalidUsage || stdout != "" || !strings.Contains(stderr, "artifact type must be iso or pxe") {
		t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	endpointCalls := preflight.endpointCalls
	stdout, stderr, code = executeWithFactories([]string{"artifacts", "build", "--type", "iso", "--offline"}, factories)
	if code != exitExternal || stdout != "" || !strings.Contains(stderr, "requires registry access") || preflight.endpointCalls != endpointCalls {
		t.Fatalf("offline stdout=%q stderr=%q code=%d endpointCalls=%d", stdout, stderr, code, preflight.endpointCalls)
	}
	factories.artifacts = func(validator application.ConfigurationValidator) application.ArtifactBuilder {
		return application.ArtifactBuilder{
			Validator: validator, Credentials: cliBuildCredentials{}, Cache: cache,
			Extractor: cliBuildExtractor{}, Installer: cliBuildInstaller{fail: true}, Store: filesystem.ArtifactStore{}, OkubVersion: "test-version",
		}
	}
	stdout, stderr, code = executeWithFactories([]string{"artifacts", "build", "--type", "iso", "--config", config, "--output", filepath.Join(root, "failed")}, factories)
	if code != exitExternal || stdout != "" || strings.Contains(stderr, "SECRET-MARKER") || !strings.Contains(stderr, "openshift-install operation failed") {
		t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func TestArtifactsBuildPreflightFailureDoesNotInvokeBuilder(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "okub.yaml")
	writeRenderConfig(t, config, filepath.Join(root, "pull"), filepath.Join(root, "key"))
	preflight := &cliPreflightFake{endpointErr: errors.New("unreachable")}
	buildCalls := 0
	factories := commandFactories{
		preflight: func(validator application.ConfigurationValidator) application.Preflight {
			return application.Preflight{Validator: validator, DNS: preflight, Environment: preflight, Cache: preflight}
		},
		artifacts: func(application.ConfigurationValidator) application.ArtifactBuilder {
			buildCalls++
			return application.ArtifactBuilder{}
		},
	}
	stdout, stderr, code := executeWithFactories([]string{"artifacts", "build", "--type", "iso", "--json", "--config", config, "--output", filepath.Join(root, "output")}, factories)
	var report domain.PreflightReport
	if code != exitValidation || stderr != "" || json.Unmarshal([]byte(stdout), &report) != nil || report.Valid || buildCalls != 0 {
		t.Fatalf("stdout=%q stderr=%q code=%d report=%#v buildCalls=%d", stdout, stderr, code, report, buildCalls)
	}
	stdout, stderr, code = executeWithFactories([]string{"artifacts", "build", "--type", "iso", "--config", config, "--output", filepath.Join(root, "output")}, factories)
	if code != exitValidation || stderr != "" || !strings.Contains(stdout, "fail [connectivity.") || buildCalls != 0 {
		t.Fatalf("human stdout=%q stderr=%q code=%d buildCalls=%d", stdout, stderr, code, buildCalls)
	}
}

func TestArtifactBuildCancellationExitCodes(t *testing.T) {
	for _, test := range []struct {
		name       string
		contextErr error
		code       int
	}{
		{name: "cancelled", contextErr: context.Canceled, code: exitInterrupted},
		{name: "deadline", contextErr: context.DeadlineExceeded, code: exitExternal},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			config := filepath.Join(root, "okub.yaml")
			writeRenderConfig(t, config, filepath.Join(root, "pull"), filepath.Join(root, "key"))
			preflight := &cliPreflightFake{}
			cache := cliBuildCache{release: cliBuildRelease()}
			factories := commandFactories{
				preflight: func(validator application.ConfigurationValidator) application.Preflight {
					return application.Preflight{Validator: validator, DNS: preflight, Environment: preflight, Cache: preflight}
				},
				artifacts: func(validator application.ConfigurationValidator) application.ArtifactBuilder {
					return application.ArtifactBuilder{
						Validator: validator, Credentials: cliBuildCredentials{}, Cache: cache, Extractor: cliBuildExtractor{},
						Installer: cliBuildInstaller{versionErr: test.contextErr}, Store: filesystem.ArtifactStore{}, OkubVersion: "test-version",
					}
				},
			}
			output := filepath.Join(root, "output")
			stdout, stderr, code := executeWithFactoriesContext([]string{"artifacts", "build", "--type", "iso", "--config", config, "--output", output}, factories)
			if code != test.code || stdout != "" || test.contextErr == context.Canceled && stderr != "" {
				t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
			}
			if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("completed output exists: %v", err)
			}
		})
	}
}

func TestArtifactsServeCancellationHumanAndJSON(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(fmt.Sprintf("json=%t", jsonOutput), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			transport := cliServeTransport{cancel: cancel}
			factories := commandFactories{serve: func(validator application.ConfigurationValidator) application.ArtifactServer {
				return application.ArtifactServer{Validator: validator, Source: cliServeSource{}, Network: cliServeNetwork{}, Transport: transport}
			}}
			args := []string{"artifacts", "serve", "--mode", "http", "--manifest", "ignored-run.json", "--bind", "192.168.101.20", "--advertise", "192.168.101.20", "--port", "8080", "--config", "../../examples/sno.yaml"}
			if jsonOutput {
				args = append(args, "--json")
			}
			var stdout, stderr bytes.Buffer
			cmd := newRootCommand("test-version", factories)
			cmd.SetArgs(args)
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			code := executeCommand(cmd, ctx, &stderr)
			if code != exitInterrupted || stderr.String() != "" {
				t.Fatalf("stdout=%q stderr=%q code=%d", stdout.String(), stderr.String(), code)
			}
			if jsonOutput {
				var result application.ArtifactServeResult
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.State != "stopped" || result.Endpoint != "http://192.168.101.20:8080" {
					t.Fatalf("stdout=%q result=%#v error=%v", stdout.String(), result, err)
				}
			} else if stdout.String() != "serving 1 verified artifacts at http://192.168.101.20:8080\nartifact server stopped\n" {
				t.Fatalf("stdout=%q", stdout.String())
			}
		})
	}
}

type cliServeSource struct{}

func (cliServeSource) LoadAndVerify(string) (ports.PublishedArtifactSet, error) {
	return ports.PublishedArtifactSet{Artifacts: []ports.PublishedArtifact{{Artifact: domain.Artifact{Path: "artifacts/agent.x86_64.iso", Size: 3, SHA256: "sha256:" + strings.Repeat("a", 64)}}}}, nil
}

type cliServeNetwork struct{}

func (cliServeNetwork) HasLocalAddress(netip.Addr) (bool, error) { return true, nil }

type cliServeTransport struct{ cancel context.CancelFunc }

func (t cliServeTransport) Serve(ctx context.Context, _ ports.PublishedArtifactSet, _ netip.AddrPort, ready func() error) error {
	if err := ready(); err != nil {
		return err
	}
	t.cancel()
	<-ctx.Done()
	return ctx.Err()
}

type cliBuildCredentials struct{}

func (cliBuildCredentials) ReadCredential(path string) ([]byte, error) {
	if strings.Contains(path, "pull") {
		return []byte(`{"auths":{"quay.io":{"auth":"dXNlcjpwYXNz"}}}`), nil
	}
	return []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AA== test"), nil
}

type cliBuildCache struct{ release domain.Release }

func (c cliBuildCache) Lookup(string, string, string) (domain.Release, bool, error) {
	return c.release, true, nil
}
func (cliBuildCache) OpenBlob(string, domain.ReleaseInput) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("archive")), nil
}
func (cliBuildCache) HasBlob(string, domain.ReleaseInput) (bool, error)      { return true, nil }
func (cliBuildCache) StoreBlob(string, domain.ReleaseInput, io.Reader) error { return nil }
func (cliBuildCache) Publish(string, domain.Release) error                   { return nil }

type cliBuildExtractor struct{}

func (cliBuildExtractor) Extract(_ io.Reader, destination string) error {
	return os.WriteFile(destination, []byte("fake"), 0o700)
}

func (cliBuildExtractor) ExtractExecutable(_ io.Reader, destination, _ string) error {
	return os.WriteFile(destination, []byte("fake"), 0o700)
}

type cliBuildInstaller struct {
	fail       bool
	versionErr error
}

func (i cliBuildInstaller) Version(context.Context, string) (string, error) {
	return "openshift-install 4.20.1-okd", i.versionErr
}
func (i cliBuildInstaller) Run(_ context.Context, _ string, command ports.InstallerCommand) (ports.InstallerResult, error) {
	result := ports.InstallerResult{StartedAt: time.Unix(1, 0).UTC(), FinishedAt: time.Unix(2, 0).UTC(), Log: []byte("safe")}
	if i.fail {
		return result, errors.New("SECRET-MARKER")
	}
	if len(command.Args) > 2 && command.Args[0] == "agent" && command.Args[1] == "create" {
		if err := os.WriteFile(filepath.Join(command.Directory, "agent.x86_64.iso"), []byte("iso"), 0o600); err != nil {
			return result, err
		}
	}
	return result, nil
}

func cliBuildRelease() domain.Release {
	return domain.Release{
		RequestedVersion: "4.20", Version: "4.20.1-okd", Architecture: "x86_64", InstallerVersion: "4.20.1-okd",
		ImagePullSpec: "quay.io/okd/release@sha256:" + strings.Repeat("a", 64), ImageDigest: "sha256:" + strings.Repeat("a", 64),
		Inputs: []domain.ReleaseInput{{Name: "installer", Digest: "sha256:" + strings.Repeat("b", 64), Size: 7}, {Name: "client", Digest: "sha256:" + strings.Repeat("d", 64), Size: 6}, {Name: "release-metadata", Digest: "sha256:" + strings.Repeat("c", 64), Size: 8}},
	}
}

type cliImageFixture struct {
	release                     domain.Release
	contents                    map[string][]byte
	resolveErr                  error
	resolveCalls, downloadCalls int
}

func newCLIImageFixture() *cliImageFixture {
	fixture := &cliImageFixture{contents: map[string][]byte{"installer": []byte("installer"), "client": []byte("client"), "release-metadata": []byte("metadata")}}
	fixture.release = domain.Release{
		Version: "4.20.1-okd", Architecture: "x86_64", InstallerVersion: "4.20.1-okd",
		ImagePullSpec: "quay.io/okd/release@sha256:" + strings.Repeat("a", 64), ImageDigest: "sha256:" + strings.Repeat("a", 64),
	}
	for _, name := range []string{"release-metadata", "installer", "client"} {
		digest := sha256.Sum256(fixture.contents[name])
		fixture.release.Inputs = append(fixture.release.Inputs, domain.ReleaseInput{
			Name: name, URL: "https://fixture.invalid/" + name, Digest: fmt.Sprintf("sha256:%x", digest), Size: int64(len(fixture.contents[name])),
		})
	}
	return fixture
}

func (f *cliImageFixture) Resolve(context.Context, string, string) (domain.Release, error) {
	f.resolveCalls++
	return f.release, f.resolveErr
}
func (f *cliImageFixture) Open(_ context.Context, rawURL string) (io.ReadCloser, error) {
	f.downloadCalls++
	return io.NopCloser(bytes.NewReader(f.contents[strings.TrimPrefix(rawURL, "https://fixture.invalid/")])), nil
}

type cliPreflightFake struct {
	endpointCalls int
	endpointErr   error
}

func (f *cliPreflightFake) LookupHost(_ context.Context, _ string, name string) ([]netip.Addr, error) {
	address := "192.168.101.20"
	if strings.HasPrefix(name, "api.") || strings.HasPrefix(name, "api-int.") {
		address = "192.168.101.10"
	} else if strings.Contains(name, ".apps.") {
		address = "192.168.101.11"
	}
	return []netip.Addr{netip.MustParseAddr(address)}, nil
}
func (*cliPreflightFake) LookupAddr(context.Context, string, netip.Addr) ([]string, error) {
	return []string{"master-0.lab.example.test"}, nil
}
func (*cliPreflightFake) ReadCredential(path string) ([]byte, error) {
	if strings.Contains(path, "pull") {
		return []byte(`{"auths":{"quay.io":{"auth":"dXNlcjpwYXNz"}}}`), nil
	}
	return []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AA== test"), nil
}
func (*cliPreflightFake) DirectoryWritable(string) error { return nil }
func (*cliPreflightFake) ToolVersion(context.Context, string) (string, error) {
	return "openshift-install 4.20.1", nil
}
func (*cliPreflightFake) AddressInUse(context.Context, netip.Addr, []int) (bool, error) {
	return false, nil
}
func (*cliPreflightFake) TCPReachable(context.Context, netip.Addr, int) error { return nil }
func (*cliPreflightFake) RouteAvailable(context.Context, netip.Addr) error    { return nil }
func (f *cliPreflightFake) EndpointReachable(context.Context, string) error {
	f.endpointCalls++
	return f.endpointErr
}
func (*cliPreflightFake) Lookup(string, string, string) (domain.Release, bool, error) {
	return domain.Release{}, false, errors.New("missing")
}
func (*cliPreflightFake) OpenBlob(string, domain.ReleaseInput) (io.ReadCloser, error) {
	return nil, errors.New("missing")
}
func (*cliPreflightFake) HasBlob(string, domain.ReleaseInput) (bool, error)      { return false, nil }
func (*cliPreflightFake) StoreBlob(string, domain.ReleaseInput, io.Reader) error { return nil }
func (*cliPreflightFake) Publish(string, domain.Release) error                   { return nil }

func executeWithFactory(args []string, factory preflightFactory) (string, string, int) {
	return executeWithFactories(args, commandFactories{preflight: factory})
}

func executeWithFactories(args []string, factories commandFactories) (string, string, int) {
	var stdout, stderr bytes.Buffer
	cmd := newRootCommand("test-version", factories)
	cmd.SetArgs(args)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	code := 0
	if err := cmd.Execute(); err != nil {
		var commandErr *commandError
		if errors.As(err, &commandErr) {
			code = commandErr.code
			if !commandErr.quiet {
				stderr.WriteString("okub: " + commandErr.Error() + "\n")
			}
		} else {
			code = exitInvalidUsage
		}
	}
	return stdout.String(), stderr.String(), code
}

func executeWithFactoriesContext(args []string, factories commandFactories) (string, string, int) {
	var stdout, stderr bytes.Buffer
	cmd := newRootCommand("test-version", factories)
	cmd.SetArgs(args)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	code := executeCommand(cmd, context.Background(), &stderr)
	return stdout.String(), stderr.String(), code
}

func execute(args []string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := Execute(args, "test-version", &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func quotedJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func writeRenderConfig(t *testing.T, path, pullSecret, sshKey string) {
	t.Helper()
	data, err := os.ReadFile("../../examples/sno.yaml")
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Replace(string(data), "~/.config/okub/pull-secret.json", pullSecret, 1)
	content = strings.Replace(content, "~/.ssh/ed25519.pub", sshKey, 1)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
