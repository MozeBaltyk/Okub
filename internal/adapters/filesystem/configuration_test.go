package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mozebaltyk/Okub/internal/ports"
	"golang.org/x/sys/unix"
)

func TestPublisherLifecycleAndPermissions(t *testing.T) {
	root := t.TempDir()
	files := generatedFiles("one")
	publisher := Publisher{}
	if action, err := publisher.Publish(root, files, false, true); err != nil || action != "create" {
		t.Fatalf("dry-run action = %q, error = %v", action, err)
	}
	if _, err := os.Stat(filepath.Join(root, "generated")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run wrote output: %v", err)
	}
	if action, err := publisher.Publish(root, files, false, false); err != nil || action != "create" {
		t.Fatalf("create action = %q, error = %v", action, err)
	}
	directoryInfo, err := os.Stat(filepath.Join(root, "generated"))
	if err != nil {
		t.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != 0o700 {
		t.Fatalf("generated directory mode = %v", directoryInfo.Mode().Perm())
	}
	for _, file := range files {
		info, err := os.Stat(filepath.Join(root, "generated", file.Name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode for %s = %v", file.Name, info.Mode().Perm())
		}
	}
	if action, err := publisher.Publish(root, files, false, false); err != nil || action != "unchanged" {
		t.Fatalf("repeat action = %q, error = %v", action, err)
	}
	changed := generatedFiles("two")
	if _, err := publisher.Publish(root, changed, false, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("differing output error = %v", err)
	}
	if action, err := publisher.Publish(root, changed, false, true); err != nil || action != "update" {
		t.Fatalf("update dry-run action = %q, error = %v", action, err)
	}
	if action, err := publisher.Publish(root, changed, true, false); err != nil || action != "update" {
		t.Fatalf("forced action = %q, error = %v", action, err)
	}
}

func TestPublisherLeavesOldDirectoryWhenAtomicExchangeFails(t *testing.T) {
	root := t.TempDir()
	if _, err := (Publisher{}).Publish(root, generatedFiles("old"), false, false); err != nil {
		t.Fatal(err)
	}
	publisher := Publisher{renameExchange: func(string, string) error {
		return errors.New("injected rename failure")
	}}
	if _, err := publisher.Publish(root, generatedFiles("new"), true, false); err == nil {
		t.Fatal("Publish() succeeded, want injected failure")
	}
	data, err := os.ReadFile(filepath.Join(root, "generated", "agent-config.yaml"))
	if err != nil || string(data) != "agent-old" {
		t.Fatalf("published data = %q, error = %v; old directory changed", data, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "generated" {
		t.Fatalf("output entries after rollback = %#v, error = %v", entries, err)
	}
}

func TestPublisherCleansStageWhenInitialPublishFails(t *testing.T) {
	root := t.TempDir()
	publisher := Publisher{renameNoReplace: func(string, string) error {
		return errors.New("injected rename failure")
	}}
	if _, err := publisher.Publish(root, generatedFiles("new"), false, false); err == nil {
		t.Fatal("Publish() succeeded, want injected failure")
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("output entries after failure = %#v, error = %v", entries, err)
	}
}

func TestPublisherRejectsInvalidSetBeforeDryRun(t *testing.T) {
	root := t.TempDir()
	tests := [][]ports.GeneratedFile{
		nil,
		{{Name: "../secret", Content: []byte("secret")}},
		{{Name: "same", Content: []byte("one")}, {Name: "same", Content: []byte("two")}},
	}
	for _, files := range tests {
		if _, err := (Publisher{}).Publish(root, files, false, true); err == nil {
			t.Fatalf("Publish(%#v) succeeded", files)
		}
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("invalid dry-runs changed output: entries = %#v, error = %v", entries, err)
	}
}

func TestReadCredentialRejectsNonRegularAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	if _, err := (ConfigurationSource{}).ReadCredential(root); err == nil {
		t.Fatal("ReadCredential(directory) succeeded")
	}
	pipe := filepath.Join(root, "pipe")
	if err := unix.Mkfifo(pipe, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (ConfigurationSource{}).ReadCredential(pipe); err == nil {
		t.Fatal("ReadCredential(named pipe) succeeded")
	}
	large := filepath.Join(root, "large")
	if err := os.WriteFile(large, make([]byte, maxCredentialSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (ConfigurationSource{}).ReadCredential(large); err == nil {
		t.Fatal("ReadCredential(oversized) succeeded")
	}
}

func TestReadCredentialAllowsSymlinkToRegularFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "credential")
	link := filepath.Join(root, "credential-link")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	data, err := (ConfigurationSource{}).ReadCredential(link)
	if err != nil || string(data) != "secret" {
		t.Fatalf("ReadCredential(symlink) = %q, %v", data, err)
	}
}

func generatedFiles(value string) []ports.GeneratedFile {
	return []ports.GeneratedFile{
		{Name: "install-config.yaml", Content: []byte("install-" + value)},
		{Name: "agent-config.yaml", Content: []byte("agent-" + value)},
	}
}
