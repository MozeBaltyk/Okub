package artifacthttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"time"

	"github.com/mozebaltyk/Okub/internal/ports"
)

type Server struct {
	ShutdownTimeout time.Duration
}

type snapshot struct {
	file *os.File
	size int64
}

func (s Server) Serve(ctx context.Context, set ports.PublishedArtifactSet, address netip.AddrPort, ready func() error) error {
	files, err := verifiedSnapshots(set)
	if err != nil {
		return err
	}
	defer closeSnapshots(files)
	listener, err := net.Listen("tcp", address.String())
	if err != nil {
		return fmt.Errorf("bind HTTP listener %s: %w", address, err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		artifact, found := files[r.URL.Path]
		if !found || r.URL.RawPath != "" || r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		reader := io.NewSectionReader(artifact.file, 0, artifact.size)
		http.ServeContent(w, r, r.URL.Path, time.Time{}, reader)
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	if err := ready(); err != nil {
		_ = server.Close()
		<-serveErr
		return err
	}
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP listener failed: %w", err)
	case <-ctx.Done():
		timeout := s.ShutdownTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		err := server.Shutdown(shutdownCtx)
		cancel()
		if err != nil {
			_ = server.Close()
		}
		serveResult := <-serveErr
		if !errors.Is(serveResult, http.ErrServerClosed) {
			return fmt.Errorf("HTTP listener failed during shutdown: %w", serveResult)
		}
		return ctx.Err()
	}
}

func verifiedSnapshots(set ports.PublishedArtifactSet) (map[string]*snapshot, error) {
	files := make(map[string]*snapshot, len(set.Artifacts))
	for _, artifact := range set.Artifacts {
		file, err := snapshotVerified(artifact)
		if err != nil {
			closeSnapshots(files)
			return nil, fmt.Errorf("snapshot %s: %w", artifact.Path, err)
		}
		files["/"+artifact.Path] = &snapshot{file: file, size: artifact.Size}
	}
	return files, nil
}

func closeSnapshots(files map[string]*snapshot) {
	for _, artifact := range files {
		_ = artifact.file.Close()
	}
}

func snapshotVerified(artifact ports.PublishedArtifact) (*os.File, error) {
	info, err := os.Lstat(artifact.FilePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() != artifact.Size {
		return nil, errors.New("artifact changed")
	}
	source, err := os.Open(artifact.FilePath)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	// shortcut: snapshots need temporary disk equal to the served set; use fs-verity when Linux-only deployment is acceptable.
	file, err := os.CreateTemp("", ".okub-artifact-*")
	if err != nil {
		return nil, err
	}
	name := file.Name()
	if err := os.Remove(name); err != nil {
		file.Close()
		return nil, err
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), source)
	if err != nil || written != artifact.Size {
		file.Close()
		return nil, errors.New("artifact changed")
	}
	if "sha256:"+hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		file.Close()
		return nil, errors.New("artifact changed")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}
