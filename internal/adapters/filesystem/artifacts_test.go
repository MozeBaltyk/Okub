package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactStoreSerializesOutputOwners(t *testing.T) {
	store := ArtifactStore{}
	output := filepath.Join(t.TempDir(), "output")
	first, err := store.Prepare(output, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := store.Prepare(output, nil, false); err == nil {
		store.Discard(second)
		t.Fatal("second output owner acquired the lock")
	}
	store.Discard(first)
	stage, err := store.Prepare(output, nil, false)
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	store.Discard(stage)
}

func TestArtifactStoreRejectsSymlinkedMonitorState(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "output")
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(output, "state")); err != nil {
		t.Fatal(err)
	}
	if stage, err := (ArtifactStore{}).PrepareMonitor(output); err == nil {
		(ArtifactStore{}).Discard(stage)
		t.Fatal("symlinked state was accepted")
	}
}

func TestFailureLogsAreUniqueAndNonDestructive(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "output")
	store := ArtifactStore{}
	if err := store.WriteFailureLog(output, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteFailureLog(output, []byte("second")); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(root, "output.failed-*.log"))
	if err != nil || len(paths) != 2 {
		t.Fatalf("failure logs=%v error=%v", paths, err)
	}
	contents := map[string]bool{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		contents[string(data)] = true
	}
	if !contents["first"] || !contents["second"] {
		t.Fatalf("failure log contents=%v", contents)
	}
}
