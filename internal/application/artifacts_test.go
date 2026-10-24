package application

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mozebaltyk/Okub/internal/adapters/filesystem"
	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
)

type buildCredentials struct{ secret string }

func (c buildCredentials) ReadCredential(path string) ([]byte, error) {
	if strings.Contains(path, "pull") {
		return []byte(c.secret), nil
	}
	return []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AA== test"), nil
}

type buildCache struct {
	release      domain.Release
	openErrorFor string
}

func (c buildCache) Lookup(string, string, string) (domain.Release, bool, error) {
	return c.release, true, nil
}
func (c buildCache) OpenBlob(_ string, input domain.ReleaseInput) (io.ReadCloser, error) {
	if input.Name == c.openErrorFor {
		return nil, errors.New("blob unavailable")
	}
	return io.NopCloser(strings.NewReader(input.Name)), nil
}
func (buildCache) HasBlob(string, domain.ReleaseInput) (bool, error)      { return true, nil }
func (buildCache) StoreBlob(string, domain.ReleaseInput, io.Reader) error { return nil }
func (buildCache) Publish(string, domain.Release) error                   { return nil }

type buildExtractor struct {
	extracted   *[]string
	failForTool string
}

func (buildExtractor) Extract(_ io.Reader, destination string) error {
	return os.WriteFile(destination, []byte("fake"), 0o700)
}

func (e buildExtractor) ExtractExecutable(source io.Reader, destination, executable string) error {
	archive, err := io.ReadAll(source)
	if err != nil {
		return err
	}
	wantInput := map[string]string{"openshift-install": "installer", "oc": "client"}[executable]
	if string(archive) != wantInput {
		return errors.New("wrong cached input")
	}
	if executable == e.failForTool {
		return errors.New("extraction failed")
	}
	if e.extracted != nil {
		*e.extracted = append(*e.extracted, wantInput+":"+executable)
	}
	return os.WriteFile(destination, []byte(executable), 0o700)
}

type buildInstaller struct {
	version, mode          string
	versionErr             error
	commandPath, directory *string
}

func (i buildInstaller) Version(context.Context, string) (string, error) {
	return "openshift-install " + i.version, i.versionErr
}

func TestInstallerVersionAcceptsExecutablePath(t *testing.T) {
	output := "/home/operator/Okub/.local/tools/4.20.0-okd-scos.17/openshift-install 4.20.0-okd-scos.17\nbuilt from commit abc123\n"
	if got := installerVersion(output); got != "4.20.0-okd-scos.17" {
		t.Fatalf("installerVersion() = %q", got)
	}
}
func (i buildInstaller) Run(ctx context.Context, _ string, command ports.InstallerCommand) (ports.InstallerResult, error) {
	result := ports.InstallerResult{Args: command.Args, Log: []byte("safe [REDACTED]"), StartedAt: time.Unix(1, 0).UTC(), FinishedAt: time.Unix(2, 0).UTC()}
	if command.Environment["OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE"] == "" {
		return result, errors.New("release image override is missing")
	}
	if i.mode == "cancel" {
		return result, context.Canceled
	}
	if i.mode == "fail" {
		return result, errors.New("failed")
	}
	if len(command.Args) > 1 && command.Args[1] == "wait-for" {
		return result, nil
	}
	wantPath := filepath.Dir(command.Directory)
	if inheritedPath := os.Getenv("PATH"); inheritedPath != "" {
		wantPath += string(os.PathListSeparator) + inheritedPath
	}
	if command.Environment["PATH"] != wantPath {
		return result, errors.New("private tool directory does not precede inherited PATH")
	}
	if i.commandPath != nil {
		*i.commandPath = command.Environment["PATH"]
	}
	if i.directory != nil {
		*i.directory = command.Directory
	}
	create := command.Args[2]
	outputs := map[string]string{}
	if create == "image" {
		outputs["agent.x86_64.iso"] = "iso"
	} else {
		outputs["boot-artifacts/agent.x86_64-vmlinuz"] = "kernel"
		outputs["boot-artifacts/agent.x86_64-initrd.img"] = "initrd"
		outputs["boot-artifacts/agent.x86_64-rootfs.img"] = "rootfs"
		outputs["boot-artifacts/agent.x86_64.ipxe"] = "ipxe"
	}
	if i.mode == "missing" {
		delete(outputs, "boot-artifacts/agent.x86_64-vmlinuz")
	}
	if i.mode == "empty" {
		outputs["agent.x86_64.iso"] = ""
	}
	if i.mode == "no-ipxe" {
		delete(outputs, "boot-artifacts/agent.x86_64.ipxe")
	}
	if i.mode == "symlink-dir" {
		outside := filepath.Join(command.Directory, "outside-boot-artifacts")
		if err := os.Mkdir(outside, 0o700); err != nil {
			return result, err
		}
		if err := os.Symlink(outside, filepath.Join(command.Directory, "boot-artifacts")); err != nil {
			return result, err
		}
	}
	for name, content := range outputs {
		path := filepath.Join(command.Directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return result, err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return result, err
		}
	}
	if i.mode == "symlink" {
		path := filepath.Join(command.Directory, "boot-artifacts", "agent.x86_64-vmlinuz")
		if err := os.Remove(path); err != nil {
			return result, err
		}
		if err := os.Symlink("agent.x86_64-rootfs.img", path); err != nil {
			return result, err
		}
	}
	if i.mode == "directory" {
		path := filepath.Join(command.Directory, "boot-artifacts", "agent.x86_64-vmlinuz")
		if err := os.Remove(path); err != nil {
			return result, err
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			return result, err
		}
	}
	return result, nil
}

func TestArtifactBuildISOAndPXEWithMetadataAndForce(t *testing.T) {
	for _, artifactType := range []string{"iso", "pxe"} {
		t.Run(artifactType, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "output")
			builder := testBuilder("okub-secret", buildInstaller{version: "4.20.1-okd"})
			result, report, err := builder.Build(context.Background(), "ignored", output, artifactType, false, false)
			if err != nil || !report.Valid || result.Action != "create" || len(result.Artifacts) == 0 {
				t.Fatalf("result=%#v report=%#v error=%v", result, report, err)
			}
			run, err := os.ReadFile(filepath.Join(output, "run.json"))
			if err != nil || strings.Contains(string(run), "okub-secret") || strings.Contains(string(run), output) || strings.Contains(string(run), "/cache") || !strings.Contains(string(run), `"configSHA256":"sha256:`) || !strings.Contains(string(run), `"releaseImagePullSpec"`) {
				t.Fatalf("run=%s error=%v", run, err)
			}
			for _, artifact := range result.Artifacts {
				if artifact.Size <= 0 || !strings.HasPrefix(artifact.SHA256, "sha256:") || filepath.IsAbs(artifact.Path) {
					t.Fatalf("artifact=%#v", artifact)
				}
			}
			if artifactType == "pxe" {
				want := []string{"artifacts/pxe/agent.ipxe", "artifacts/pxe/initramfs.img", "artifacts/pxe/kernel", "artifacts/pxe/manifest.json", "artifacts/pxe/rootfs.img"}
				got := make([]string, len(result.Artifacts))
				for i := range result.Artifacts {
					got[i] = result.Artifacts[i].Path
				}
				if strings.Join(got, ",") != strings.Join(want, ",") {
					t.Fatalf("PXE artifacts=%v, want %v", got, want)
				}
			}
			for _, name := range []string{"install-config.yaml", "agent-config.yaml"} {
				info, statErr := os.Stat(filepath.Join(output, "generated", name))
				if statErr != nil {
					t.Fatalf("generated input %s: %v", name, statErr)
				}
				if info.Mode().Perm() != 0o600 {
					t.Fatalf("generated input %s mode=%v", name, info.Mode())
				}
			}
			for _, name := range []string{"openshift-install", "oc"} {
				if _, err := os.Stat(filepath.Join(output, name)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("temporary tool %s was published: %v", name, err)
				}
			}
			if _, _, err := builder.Build(context.Background(), "ignored", output, artifactType, false, false); err == nil {
				t.Fatal("existing output was overwritten without force")
			}
			if updated, _, err := builder.Build(context.Background(), "ignored", output, artifactType, true, false); err != nil || updated.Action != "update" {
				t.Fatalf("updated=%#v error=%v", updated, err)
			}
			updatedRun, err := os.ReadFile(filepath.Join(output, "run.json"))
			if err != nil || !bytes.Equal(run, updatedRun) {
				t.Fatalf("run metadata is not deterministic: error=%v\nfirst=%s\nsecond=%s", err, run, updatedRun)
			}
		})
	}
}

func TestArtifactBuildExtractsExactCachedTools(t *testing.T) {
	var extracted []string
	builder := testBuilder("secret", buildInstaller{version: "4.20.1-okd"})
	builder.Extractor = buildExtractor{extracted: &extracted}
	if _, _, err := builder.Build(context.Background(), "ignored", filepath.Join(t.TempDir(), "output"), "iso", false, false); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(extracted, ","), "installer:openshift-install,client:oc"; got != want {
		t.Fatalf("extracted %q, want %q", got, want)
	}
}

func TestArtifactBuildPrependsPrivateToolsToPath(t *testing.T) {
	inherited := filepath.Join(t.TempDir(), "host-one") + string(os.PathListSeparator) + filepath.Join(t.TempDir(), "host-two")
	t.Setenv("PATH", inherited)
	var commandPath, directory string
	builder := testBuilder("secret", buildInstaller{version: "4.20.1-okd", commandPath: &commandPath, directory: &directory})
	if _, _, err := builder.Build(context.Background(), "ignored", filepath.Join(t.TempDir(), "output"), "iso", false, false); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Dir(directory) + string(os.PathListSeparator) + inherited; commandPath != want {
		t.Fatalf("PATH=%q, want %q", commandPath, want)
	}
}

func TestArtifactBuildClientStagingFailuresAreCacheErrors(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*ArtifactBuilder)
		cause string
	}{
		{name: "missing input", cause: "cached client input is missing", setup: func(builder *ArtifactBuilder) {
			cache := builder.Cache.(buildCache)
			cache.release.Inputs = cache.release.Inputs[:1]
			builder.Cache = cache
		}},
		{name: "missing blob", cause: "open cached client input", setup: func(builder *ArtifactBuilder) {
			cache := builder.Cache.(buildCache)
			cache.openErrorFor = "client"
			builder.Cache = cache
		}},
		{name: "extraction", cause: "extract cached oc executable", setup: func(builder *ArtifactBuilder) {
			builder.Extractor = buildExtractor{failForTool: "oc"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := testBuilder("secret", buildInstaller{version: "4.20.1-okd"})
			test.setup(&builder)
			_, _, err := builder.Build(context.Background(), "ignored", filepath.Join(t.TempDir(), "output"), "iso", false, false)
			var cacheErr *CacheError
			if !errors.As(err, &cacheErr) || !strings.Contains(err.Error(), test.cause) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestArtifactBuildFailuresPublishNoCompletedOutput(t *testing.T) {
	for _, mode := range []string{"missing", "empty", "symlink", "symlink-dir", "directory", "fail", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "output")
			builder := testBuilder("secret", buildInstaller{version: "4.20.1-okd", mode: mode})
			typeName := "pxe"
			if mode == "empty" {
				typeName = "iso"
			}
			_, _, err := builder.Build(context.Background(), "ignored", output, typeName, false, false)
			if err == nil {
				t.Fatal("Build succeeded")
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation was not preserved: %v", err)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("completed output exists: %v", err)
			}
		})
	}
}

func TestArtifactBuildPXEAllowsMissingOptionalIPXEScript(t *testing.T) {
	result, _, err := testBuilder("secret", buildInstaller{version: "4.20.1-okd", mode: "no-ipxe"}).Build(context.Background(), "ignored", filepath.Join(t.TempDir(), "output"), "pxe", false, false)
	if err != nil || len(result.Artifacts) != 4 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	for _, artifact := range result.Artifacts {
		if artifact.Path == "artifacts/pxe/agent.ipxe" {
			t.Fatal("optional iPXE artifact was published when absent")
		}
	}
}

func TestArtifactBuildAfterRenderPreservesGeneratedAndReports(t *testing.T) {
	output := filepath.Join(t.TempDir(), "output")
	builder := testBuilder("secret", buildInstaller{version: "4.20.1-okd"})
	validated, err := builder.Validator.Validate("ignored")
	if err != nil || !validated.Report.Valid {
		t.Fatalf("validate: report=%#v error=%v", validated.Report, err)
	}
	inputs, err := renderInstallerFiles(validated.Definition, "secret", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AA== test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (filesystem.Publisher{}).Publish(output, inputs, false, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(output, "reports"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "reports", "validation.json"), []byte("report\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, _, err := builder.Build(context.Background(), "ignored", output, "iso", false, false)
	if err != nil || result.Action != "update" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	report, err := os.ReadFile(filepath.Join(output, "reports", "validation.json"))
	if err != nil || string(report) != "report\n" {
		t.Fatalf("report=%q error=%v", report, err)
	}
}

func TestArtifactBuildFailurePreservesPublishedOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "output")
	if _, _, err := testBuilder("secret", buildInstaller{version: "4.20.1-okd"}).Build(context.Background(), "ignored", output, "iso", false, false); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(output, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := testBuilder("secret", buildInstaller{version: "4.20.1-okd", mode: "fail"}).Build(context.Background(), "ignored", output, "iso", true, false); err == nil {
		t.Fatal("failed replacement succeeded")
	}
	after, err := os.ReadFile(filepath.Join(output, "run.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("published output changed: error=%v", err)
	}
}

func TestArtifactBuildRejectsVersionMismatch(t *testing.T) {
	_, _, err := testBuilder("secret", buildInstaller{version: "4.19.0"}).Build(context.Background(), "ignored", filepath.Join(t.TempDir(), "out"), "iso", false, false)
	var compatibility *CompatibilityError
	if !errors.As(err, &compatibility) {
		t.Fatalf("error=%v", err)
	}
}

func TestArtifactBuildPreservesVersionCancellation(t *testing.T) {
	for _, contextErr := range []error{context.Canceled, context.DeadlineExceeded} {
		output := filepath.Join(t.TempDir(), "output")
		_, _, err := testBuilder("secret", buildInstaller{versionErr: contextErr}).Build(context.Background(), "ignored", output, "iso", false, false)
		if !errors.Is(err, contextErr) {
			t.Fatalf("error=%v, want %v", err, contextErr)
		}
		if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("completed output exists after cancellation: %v", statErr)
		}
	}
}

func TestArtifactBuildRejectsOfflineMode(t *testing.T) {
	_, _, err := (ArtifactBuilder{}).Build(context.Background(), "ignored", "", "iso", false, true)
	var compatibility *CompatibilityError
	if !errors.As(err, &compatibility) || !strings.Contains(err.Error(), "registry access") {
		t.Fatalf("error=%v", err)
	}
}

func TestPullSecretRedactionsIncludeNestedValues(t *testing.T) {
	values := pullSecretRedactions([]byte(`{"auths":{"quay.io":{"auth":"SECRET-AUTH","email":"SECRET-EMAIL"}}}`), "/private/stage")
	joined := strings.Join(values, "\n")
	for _, value := range []string{"SECRET-AUTH", "SECRET-EMAIL", "/private/stage"} {
		if !strings.Contains(joined, value) {
			t.Fatalf("missing redaction value %q from %#v", value, values)
		}
	}
}

func TestInstallMonitorPreservesSuccessFailureAndCancellationResults(t *testing.T) {
	for _, mode := range []string{"", "fail", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "output")
			builder := testBuilder("monitor-secret", buildInstaller{version: "4.20.1-okd"})
			if _, _, err := builder.Build(context.Background(), "ignored", output, "iso", false, false); err != nil {
				t.Fatal(err)
			}
			monitor := InstallMonitor{
				Validator: builder.Validator, Credentials: builder.Credentials, Cache: builder.Cache, Extractor: buildExtractor{},
				Installer: buildInstaller{version: "4.20.1-okd", mode: mode}, Store: builder.Store,
			}
			result, _, err := monitor.Monitor(context.Background(), "ignored", output, false)
			if mode == "" && err != nil || mode != "" && err == nil {
				t.Fatalf("result=%#v error=%v", result, err)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation was not preserved: %v", err)
			}
			log, readErr := os.ReadFile(filepath.Join(output, "monitor", "openshift-install.log"))
			metadata, metadataErr := os.ReadFile(filepath.Join(output, "monitor", "result.json"))
			if readErr != nil || metadataErr != nil || strings.Contains(string(log), "monitor-secret") || !strings.Contains(string(metadata), `"outcome"`) || mode == "cancel" && !strings.Contains(string(metadata), `"outcome":"cancelled"`) {
				t.Fatalf("log=%q metadata=%q errors=%v/%v", log, metadata, readErr, metadataErr)
			}
		})
	}
}

func TestInstallMonitorPreservesVersionCancellation(t *testing.T) {
	for _, contextErr := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(contextErr.Error(), func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "output")
			builder := testBuilder("secret", buildInstaller{version: "4.20.1-okd"})
			if _, _, err := builder.Build(context.Background(), "ignored", output, "iso", false, false); err != nil {
				t.Fatal(err)
			}
			monitor := InstallMonitor{
				Validator: builder.Validator, Credentials: builder.Credentials, Cache: builder.Cache, Extractor: buildExtractor{},
				Installer: buildInstaller{versionErr: contextErr}, Store: builder.Store,
			}
			_, _, err := monitor.Monitor(context.Background(), "ignored", output, false)
			if !errors.Is(err, contextErr) {
				t.Fatalf("error=%v, want %v", err, contextErr)
			}
			if _, statErr := os.Stat(filepath.Join(output, "monitor")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("monitor result exists after cancellation: %v", statErr)
			}
		})
	}
}

func testBuilder(secret string, installer buildInstaller) ArtifactBuilder {
	release := domain.Release{
		RequestedVersion: "4.20", Version: "4.20.1-okd", Architecture: "x86_64", InstallerVersion: "4.20.1-okd",
		ImagePullSpec: "quay.io/okd/release@sha256:" + strings.Repeat("a", 64), ImageDigest: "sha256:" + strings.Repeat("a", 64),
		Inputs: []domain.ReleaseInput{{Name: "installer", Digest: "sha256:" + strings.Repeat("b", 64), Size: 7}, {Name: "client", Digest: "sha256:" + strings.Repeat("d", 64), Size: 6}, {Name: "release-metadata", Digest: "sha256:" + strings.Repeat("c", 64), Size: 8}},
	}
	return ArtifactBuilder{
		Validator:   ConfigurationValidator{Source: sourceStub{data: []byte(preflightConfig)}, CacheDirectory: "/cache"},
		Credentials: buildCredentials{secret: secret}, Cache: buildCache{release: release}, Extractor: buildExtractor{}, Installer: installer,
		Store: filesystem.ArtifactStore{}, OkubVersion: "test-version",
	}
}
