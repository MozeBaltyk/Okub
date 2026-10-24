// Package filesystem provides local filesystem adapters.
package filesystem

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/mozebaltyk/Okub/internal/ports"
	"golang.org/x/sys/unix"
)

// ConfigurationSource reads configuration from the local filesystem.
type ConfigurationSource struct{}

func (ConfigurationSource) Read(path string) ([]byte, error) {
	return os.ReadFile(path)
}

const maxCredentialSize = 16 << 20

// ReadCredential reads a bounded regular file. Its errors never include file contents.
func (ConfigurationSource) ReadCredential(path string) ([]byte, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("credential path is not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCredentialSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCredentialSize {
		return nil, fmt.Errorf("credential file exceeds %d bytes", maxCredentialSize)
	}
	return data, nil
}

// Publisher publishes both generated files as one directory rename.
type Publisher struct {
	renameNoReplace func(string, string) error
	renameExchange  func(string, string) error
}

func (p Publisher) Publish(outputDirectory string, files []ports.GeneratedFile, force, dryRun bool) (string, error) {
	if outputDirectory == "" {
		return "", errors.New("output directory is required")
	}
	if err := validateGeneratedFiles(files); err != nil {
		return "", err
	}
	target := filepath.Join(outputDirectory, "generated")
	action, exists, err := compareGenerated(target, files)
	if err != nil {
		return "", err
	}
	if action == "unchanged" || dryRun {
		return action, nil
	}
	if exists && !force {
		return "", errors.New("generated output differs; use --force to replace it")
	}
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	stage, err := os.MkdirTemp(outputDirectory, ".generated-stage-")
	if err != nil {
		return "", fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := os.Chmod(stage, 0o700); err != nil {
		return "", fmt.Errorf("secure staging directory: %w", err)
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(stage, file.Name), file.Content, 0o600); err != nil {
			return "", fmt.Errorf("stage generated file %q: %w", file.Name, err)
		}
	}
	if !exists {
		rename := p.renameNoReplace
		if rename == nil {
			rename = renameNoReplace
		}
		if err := rename(stage, target); err != nil {
			return "", fmt.Errorf("publish generated directory: %w", err)
		}
		return action, nil
	}
	exchange := p.renameExchange
	if exchange == nil {
		exchange = renameExchange
	}
	if err := exchange(stage, target); err != nil {
		return "", fmt.Errorf("publish generated directory: %w", err)
	}
	if err := os.RemoveAll(stage); err != nil {
		return "", fmt.Errorf("remove replaced generated directory: %w", err)
	}
	return action, nil
}

func renameNoReplace(oldPath, newPath string) error {
	return unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_NOREPLACE)
}

func renameExchange(oldPath, newPath string) error {
	return unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_EXCHANGE)
}

func validateGeneratedFiles(files []ports.GeneratedFile) error {
	if len(files) == 0 {
		return errors.New("at least one generated file is required")
	}
	names := make(map[string]struct{}, len(files))
	for _, file := range files {
		if file.Name == "" || file.Name == "." || filepath.Base(file.Name) != file.Name {
			return errors.New("generated file name is invalid")
		}
		if _, exists := names[file.Name]; exists {
			return fmt.Errorf("generated file name %q is duplicated", file.Name)
		}
		names[file.Name] = struct{}{}
	}
	return nil
}

func compareGenerated(target string, files []ports.GeneratedFile) (action string, exists bool, err error) {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return "create", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect generated directory: %w", err)
	}
	if !info.IsDir() {
		return "", true, errors.New("generated output exists and is not a directory")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return "", true, fmt.Errorf("inspect generated directory: %w", err)
	}
	wanted := append([]ports.GeneratedFile(nil), files...)
	sort.Slice(wanted, func(i, j int) bool { return wanted[i].Name < wanted[j].Name })
	if len(entries) != len(wanted) {
		return "update", true, nil
	}
	for i, entry := range entries {
		if entry.Name() != wanted[i].Name || entry.Type()&os.ModeSymlink != 0 {
			return "update", true, nil
		}
		info, err := entry.Info()
		if err != nil {
			return "", true, fmt.Errorf("inspect generated file %q: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() != int64(len(wanted[i].Content)) {
			return "update", true, nil
		}
		data, err := os.ReadFile(filepath.Join(target, entry.Name()))
		if err != nil {
			return "", true, fmt.Errorf("read generated file %q: %w", entry.Name(), err)
		}
		if !bytes.Equal(data, wanted[i].Content) {
			return "update", true, nil
		}
	}
	return "unchanged", true, nil
}
