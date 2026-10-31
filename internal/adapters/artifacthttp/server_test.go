package artifacthttp

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
)

func TestServerTransfersOnlyVerifiedArtifactsAndCancels(t *testing.T) {
	content := []byte("artifact content")
	digest := sha256.Sum256(content)
	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	artifact := ports.PublishedArtifact{Artifact: domain.Artifact{Path: "artifacts/agent.x86_64.iso", Size: int64(len(content)), SHA256: fmt.Sprintf("sha256:%x", digest)}, FilePath: path}
	set := ports.PublishedArtifactSet{Artifacts: []ports.PublishedArtifact{artifact}}
	address := freeAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- (Server{ShutdownTimeout: time.Second}).Serve(ctx, set, address, func() error { close(ready); return nil })
	}()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not become ready")
	}
	base := "http://" + address.String()
	response, err := http.Get(base + "/artifacts/agent.x86_64.iso")
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || string(data) != string(content) {
		t.Fatalf("status=%d body=%q err=%v", response.StatusCode, data, readErr)
	}
	for _, requestPath := range []string{"/", "/artifacts/", "/artifact", "/artifacts/../artifact"} {
		response, err := http.Get(base + requestPath)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("%s status=%d, want 404", requestPath, response.StatusCode)
		}
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", len(content))), 0o600); err != nil {
		t.Fatal(err)
	}
	response, err = http.Get(base + "/artifacts/agent.x86_64.iso")
	if err != nil {
		t.Fatal(err)
	}
	data, readErr = io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || string(data) != string(content) {
		t.Fatalf("modified source changed verified snapshot: status=%d body=%q err=%v", response.StatusCode, data, readErr)
	}
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("shutdown error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestServerRejectsOccupiedPortBeforeReadiness(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	address := listener.Addr().(*net.TCPAddr).AddrPort()
	ready := false
	err = (Server{}).Serve(context.Background(), ports.PublishedArtifactSet{}, address, func() error { ready = true; return nil })
	if err == nil || ready {
		t.Fatalf("error=%v ready=%t", err, ready)
	}
}

func freeAddress(t *testing.T) netip.AddrPort {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().(*net.TCPAddr).AddrPort()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}
