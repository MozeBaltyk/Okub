package openshift

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mozebaltyk/Okub/internal/ports"
)

func TestExtractorExtractsOnlyExpectedExecutable(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "openshift-install")
	archive := tarArchive(t, []tarEntry{{name: "README", body: "ignored"}, {name: "openshift-install", body: "binary"}})
	if err := (Extractor{}).Extract(bytes.NewReader(archive), destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	info, _ := os.Stat(destination)
	if err != nil || string(data) != "binary" || info.Mode().Perm() != 0o700 {
		t.Fatalf("data=%q mode=%v error=%v", data, info.Mode(), err)
	}
}

func TestExtractorExtractsOCFromClientArchive(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "oc")
	archive := tarArchive(t, []tarEntry{{name: "README.md", body: "ignored"}, {name: "oc", body: "client"}, {name: "kubectl", kind: tar.TypeLink, linkName: "oc"}})
	if err := (Extractor{}).ExtractExecutable(bytes.NewReader(archive), destination, "oc"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != "client" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

func TestInstallerSizeLimitIncludesOKD420(t *testing.T) {
	const officialInstallerSize int64 = 631_353_528
	if officialInstallerSize > maxInstallerSize || maxInstallerSize > maxArchiveSize {
		t.Fatalf("official size=%d installer limit=%d archive limit=%d", officialInstallerSize, maxInstallerSize, maxArchiveSize)
	}
}

func TestExtractorRejectsArchiveAttacks(t *testing.T) {
	tests := []struct {
		name    string
		entries []tarEntry
	}{
		{"traversal", []tarEntry{{name: "../openshift-install", body: "bad"}}},
		{"symlink", []tarEntry{{name: "link", kind: tar.TypeSymlink}, {name: "openshift-install", body: "bad"}}},
		{"target symlink", []tarEntry{{name: "openshift-install", kind: tar.TypeSymlink}}},
		{"target hardlink", []tarEntry{{name: "openshift-install", kind: tar.TypeLink, linkName: "other"}}},
		{"unsafe hardlink target", []tarEntry{{name: "kubectl", kind: tar.TypeLink, linkName: "../oc"}, {name: "openshift-install", body: "bad"}}},
		{"duplicate", []tarEntry{{name: "openshift-install", body: "one"}, {name: "openshift-install", body: "two"}}},
		{"duplicate unrelated cleaned path", []tarEntry{{name: "README", body: "one"}, {name: "./README", body: "two"}, {name: "openshift-install", body: "bad"}}},
		{"oversized", []tarEntry{{name: "openshift-install", size: maxInstallerSize + 1}}},
		{"non-canonical", []tarEntry{{name: "directory/../openshift-install", body: "bad"}}},
		{"non-canonical absolute after prefix", []tarEntry{{name: ".//openshift-install", body: "bad"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "openshift-install")
			if err := (Extractor{}).Extract(bytes.NewReader(tarArchive(t, test.entries)), destination); err == nil {
				t.Fatal("Extract succeeded")
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatalf("executable remains: %v", err)
			}
		})
	}
}

func TestExtractorRejectsMalformedArchive(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "openshift-install")
	if err := (Extractor{}).Extract(strings.NewReader("not an archive"), destination); err == nil {
		t.Fatal("Extract succeeded")
	}
}

func TestRunnerRedactsSecretsAndUsesArgumentArray(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'args:%s secret:%s' \"$*\" 'TOP-SECRET'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := (Runner{}).Run(context.Background(), script, ports.InstallerCommand{Args: []string{"one", "two words", "TOP-SECRET"}, Secrets: []string{"TOP-SECRET"}})
	if err != nil || strings.Contains(string(result.Log), "TOP-SECRET") || strings.Contains(strings.Join(result.Args, " "), "TOP-SECRET") || !strings.Contains(string(result.Log), "[REDACTED]") || result.Args[1] != "two words" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestRunnerOverridesEnvironment(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE", "mutable")
	result, err := (Runner{}).Run(context.Background(), script, ports.InstallerCommand{Environment: map[string]string{"OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE": "pinned"}})
	if err != nil || string(result.Log) != "pinned" {
		t.Fatalf("log=%q error=%v", result.Log, err)
	}
}

func TestRunnerCancellation(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrap 'exit 0' INT\nsleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := (Runner{}).Run(ctx, script, ports.InstallerCommand{})
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() == nil {
		t.Fatalf("error=%v context=%v", err, ctx.Err())
	}
}

type tarEntry struct {
	name, body, linkName string
	kind                 byte
	size                 int64
}

func tarArchive(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	writer := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		size := int64(len(entry.body))
		if entry.size != 0 {
			size = entry.size
		}
		if err := writer.WriteHeader(&tar.Header{Name: entry.name, Typeflag: kind, Size: size, Mode: 0o755, Linkname: entry.linkName}); err != nil {
			t.Fatal(err)
		}
		if entry.body != "" {
			if _, err := writer.Write([]byte(entry.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil && !strings.Contains(err.Error(), "missed writing") {
		t.Fatal(err)
	}
	_ = gzipWriter.Close()
	return output.Bytes()
}
