// Package openshift safely extracts and invokes openshift-install.
package openshift

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/mozebaltyk/Okub/internal/ports"
)

const (
	maxArchiveEntries = 1024
	maxInstallerSize  = 640 << 20
	maxArchiveSize    = 1 << 30
	maxCommandLog     = 16 << 20
)

type Extractor struct{}

func (Extractor) Extract(source io.Reader, destination string) error {
	return (Extractor{}).ExtractExecutable(source, destination, "openshift-install")
}

// ExtractExecutable securely extracts exactly one expected top-level executable.
func (Extractor) ExtractExecutable(source io.Reader, destination, executable string) error {
	if executable != "openshift-install" && executable != "oc" {
		return errors.New("unsupported executable requested")
	}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.Remove(destination)
		}
	}()
	gzipReader, err := gzip.NewReader(source)
	if err != nil {
		return errors.New("installer archive is not valid gzip")
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	found := false
	seen := map[string]struct{}{}
	var totalSize int64
	for entries := 0; ; entries++ {
		if entries >= maxArchiveEntries {
			return errors.New("installer archive contains too many entries")
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.New("installer archive is invalid")
		}
		if header.Size < 0 || header.Size > maxArchiveSize-totalSize {
			return errors.New("installer archive expands beyond the allowed size")
		}
		totalSize += header.Size
		name := strings.TrimPrefix(header.Name, "./")
		clean := path.Clean(name)
		if clean == "." || clean != name || strings.HasPrefix(clean, "../") || path.IsAbs(header.Name) || path.IsAbs(name) {
			return errors.New("installer archive contains an unsafe path")
		}
		if _, duplicate := seen[clean]; duplicate {
			return errors.New("installer archive contains a duplicate path")
		}
		seen[clean] = struct{}{}
		if header.Typeflag == tar.TypeSymlink {
			return errors.New("installer archive contains a link")
		}
		if header.Typeflag == tar.TypeLink {
			linkName := strings.TrimPrefix(header.Linkname, "./")
			linkClean := path.Clean(linkName)
			if clean == executable || header.Size != 0 || linkClean == "." || linkClean != linkName || strings.HasPrefix(linkClean, "../") || path.IsAbs(header.Linkname) || path.IsAbs(linkName) {
				return errors.New("installer archive contains an invalid hard link")
			}
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return errors.New("installer archive contains an unsupported entry")
		}
		if clean != executable {
			continue
		}
		if found || header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maxInstallerSize {
			return errors.New("installer archive contains an invalid executable")
		}
		file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
		if err != nil {
			return fmt.Errorf("create installer executable: %w", err)
		}
		written, copyErr := io.Copy(file, io.LimitReader(reader, header.Size+1))
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil || written != header.Size {
			os.Remove(destination)
			return errors.New("extract installer executable failed")
		}
		found = true
	}
	if !found {
		return fmt.Errorf("archive does not contain %s", executable)
	}
	succeeded = true
	return nil
}

type Runner struct{}

func (Runner) Version(ctx context.Context, executable string) (string, error) {
	result, err := (Runner{}).Run(ctx, executable, ports.InstallerCommand{Args: []string{"version"}})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(result.Log)), nil
}

func (Runner) Run(ctx context.Context, executable string, command ports.InstallerCommand) (ports.InstallerResult, error) {
	started := time.Now().UTC()
	cmd := exec.CommandContext(ctx, executable, command.Args...)
	cmd.Dir = command.Directory
	cmd.Env = commandEnvironment(os.Environ(), command.Environment)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}
	cmd.WaitDelay = 5 * time.Second
	var output limitedBuffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	log := redact(output.Bytes(), command.Secrets)
	result := ports.InstallerResult{Args: redactArgs(command.Args, command.Secrets), Log: log, StartedAt: started, FinishedAt: time.Now().UTC()}
	if err == nil {
		return result, nil
	}
	result.ExitCode = -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}
	if ctx.Err() != nil {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return result, ctx.Err()
	}
	return result, errors.New("openshift-install command failed")
}

func commandEnvironment(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	result := make([]string, 0, len(base)+len(overrides))
	for _, item := range base {
		name, _, _ := strings.Cut(item, "=")
		if _, replaced := overrides[name]; !replaced {
			result = append(result, item)
		}
	}
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, name+"="+overrides[name])
	}
	return result
}

func redactArgs(args, secrets []string) []string {
	redacted := make([]string, len(args))
	for i, arg := range args {
		redacted[i] = string(redact([]byte(arg), secrets))
	}
	return redacted
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	remaining := maxCommandLog - b.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = b.Buffer.Write(value)
	}
	return original, nil
}

func redact(value []byte, secrets []string) []byte {
	text := string(value)
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return []byte(text)
}
