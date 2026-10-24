package helper

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type acceptSignature struct{ called bool }

func (v *acceptSignature) Verify(_ context.Context, _ string, key []byte) error {
	v.called = true
	if len(key) < 100 {
		return fmt.Errorf("missing key")
	}
	return nil
}

func TestImageSyncVerifiesAndReusesEveryByte(t *testing.T) {
	image := []byte("small fixture image")
	sum := sha256.Sum256(image)
	mux := http.NewServeMux()
	mux.HandleFunc("/"+ChecksumFilename, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintf(w, "SHA256 (%s) = %x\n", ImageFilename, sum) })
	mux.HandleFunc("/"+ImageFilename, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(image) })
	server := httptest.NewServer(mux)
	defer server.Close()
	verifier := &acceptSignature{}
	syncer := ImageSync{Client: server.Client(), Verifier: verifier, ImageURL: server.URL + "/" + ImageFilename, ChecksumURL: server.URL + "/" + ChecksumFilename, AllowHTTP: true}
	repo := t.TempDir()
	got, err := syncer.Sync(context.Background(), repo, false)
	if err != nil || !verifier.called || got.Reused {
		t.Fatalf("sync=%+v err=%v", got, err)
	}
	offline, err := syncer.Sync(context.Background(), repo, true)
	if err != nil || !offline.Reused {
		t.Fatalf("offline=%+v err=%v", offline, err)
	}
	if err = os.WriteFile(got.ImagePath, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = syncer.Sync(context.Background(), repo, true); err == nil {
		t.Fatal("tampered cache accepted")
	}
}

func TestImageSyncRejectsSymlinkCacheTarget(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".local", "cache", "fedora", "44")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, ChecksumFilename)); err != nil {
		t.Fatal(err)
	}
	syncer := ImageSync{Client: http.DefaultClient, Verifier: &acceptSignature{}, ImageURL: "http://invalid/" + ImageFilename, ChecksumURL: "http://invalid/" + ChecksumFilename, AllowHTTP: true}
	if _, err := syncer.Sync(context.Background(), repo, false); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestImageCacheAllowsExistingReadOnlySharedParent(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".local"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(repo, ".local", "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivatePath(repo, ".local", "cache", "fedora", "44"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(repo, ".local", "cache", "fedora"), filepath.Join(repo, ".local", "cache", "fedora", "44")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("private cache directory %s mode=%v", path, info.Mode().Perm())
		}
	}
}

func TestParseChecksumRequiresExactFilename(t *testing.T) {
	sum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if _, err := parseChecksum([]byte(sum+"  "+ImageFilename+".evil\n"), ImageFilename); err == nil {
		t.Fatal("suffix filename accepted")
	}
}

func TestDownloadReverifiesConcurrentWinner(t *testing.T) {
	for _, test := range []struct {
		name       string
		winner     string
		wantError  bool
		winnerMode os.FileMode
	}{
		{name: "identical", winner: "content", winnerMode: 0o644},
		{name: "different", winner: "hostile", winnerMode: 0o644, wantError: true},
		{name: "unsafe mode", winner: "content", winnerMode: 0o666, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "target")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if err := os.WriteFile(target, []byte(test.winner), test.winnerMode); err != nil {
					t.Error(err)
				}
				if err := os.Chmod(target, test.winnerMode); err != nil {
					t.Error(err)
				}
				_, _ = w.Write([]byte("content"))
			}))
			defer server.Close()
			err := download(context.Background(), server.Client(), server.URL, target, 1024)
			if (err != nil) != test.wantError {
				t.Fatalf("error=%v wantError=%t", err, test.wantError)
			}
		})
	}
}
