package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
	"golang.org/x/sys/unix"
)

type ArtifactStore struct{}

func (ArtifactStore) Prepare(output string, inputs []ports.GeneratedFile, force bool) (ports.BuildStage, error) {
	if output == "" {
		return ports.BuildStage{}, errors.New("output directory is required")
	}
	parent := filepath.Dir(filepath.Clean(output))
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return ports.BuildStage{}, fmt.Errorf("create output parent: %w", err)
	}
	lock, err := lockOutput(output)
	if err != nil {
		return ports.BuildStage{}, err
	}
	exists, preserveReports, err := inspectBuildOutput(output, inputs, force)
	if err != nil {
		lock.Close()
		return ports.BuildStage{}, err
	}
	root, err := os.MkdirTemp(parent, ".okub-build-")
	if err != nil {
		lock.Close()
		return ports.BuildStage{}, fmt.Errorf("create build staging directory: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		os.RemoveAll(root)
		lock.Close()
		return ports.BuildStage{}, err
	}
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		os.RemoveAll(root)
		lock.Close()
		return ports.BuildStage{}, err
	}
	generated := filepath.Join(root, "generated")
	if err := os.Mkdir(generated, 0o700); err != nil {
		os.RemoveAll(root)
		lock.Close()
		return ports.BuildStage{}, err
	}
	for _, input := range inputs {
		if filepath.Base(input.Name) != input.Name {
			os.RemoveAll(root)
			lock.Close()
			return ports.BuildStage{}, errors.New("installer input name is invalid")
		}
		if err := os.WriteFile(filepath.Join(work, input.Name), input.Content, 0o600); err != nil {
			os.RemoveAll(root)
			lock.Close()
			return ports.BuildStage{}, fmt.Errorf("stage installer input: %w", err)
		}
		if err := os.WriteFile(filepath.Join(generated, input.Name), input.Content, 0o600); err != nil {
			os.RemoveAll(root)
			lock.Close()
			return ports.BuildStage{}, fmt.Errorf("stage generated input: %w", err)
		}
	}
	if preserveReports {
		if err := copyReports(filepath.Join(output, "reports"), filepath.Join(root, "reports")); err != nil {
			os.RemoveAll(root)
			lock.Close()
			return ports.BuildStage{}, err
		}
	}
	return ports.BuildStage{Root: root, Work: work, Lock: lock, Replace: exists}, nil
}

func (ArtifactStore) InstallerPath(stage ports.BuildStage) string {
	return filepath.Join(stage.Root, "openshift-install")
}

func (ArtifactStore) FinalizeArtifacts(stage ports.BuildStage, artifactType, architecture string) ([]domain.Artifact, error) {
	type expected struct{ source, destination string }
	var files []expected
	switch artifactType {
	case "iso":
		files = []expected{{"agent." + architecture + ".iso", "artifacts/agent." + architecture + ".iso"}}
	case "pxe":
		bootArtifacts := filepath.Join(stage.Work, "boot-artifacts")
		if info, err := os.Lstat(bootArtifacts); err != nil || !info.IsDir() {
			return nil, errors.New("installer boot-artifacts output is missing or unsafe")
		}
		files = []expected{
			{"boot-artifacts/agent." + architecture + "-vmlinuz", "artifacts/pxe/agent." + architecture + "-vmlinuz"},
			{"boot-artifacts/agent." + architecture + "-initrd.img", "artifacts/pxe/agent." + architecture + "-initrd.img"},
			{"boot-artifacts/agent." + architecture + "-rootfs.img", "artifacts/pxe/agent." + architecture + "-rootfs.img"},
		}
		ipxe := filepath.Join(bootArtifacts, "agent."+architecture+".ipxe")
		if info, err := os.Lstat(ipxe); err == nil {
			if !info.Mode().IsRegular() || info.Size() == 0 {
				return nil, errors.New("optional installer iPXE output is not a non-empty regular file")
			}
			files = append(files, expected{"boot-artifacts/agent." + architecture + ".ipxe", "artifacts/pxe/agent." + architecture + ".ipxe"})
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect optional installer iPXE output: %w", err)
		}
	default:
		return nil, errors.New("artifact type is unsupported")
	}
	artifacts := make([]domain.Artifact, 0, len(files)+1)
	for _, item := range files {
		source := filepath.Join(stage.Work, item.source)
		info, err := os.Lstat(source)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return nil, fmt.Errorf("expected installer output %q is missing or empty", item.source)
		}
		destination := filepath.Join(stage.Root, filepath.FromSlash(item.destination))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return nil, err
		}
		if err := os.Rename(source, destination); err != nil {
			return nil, fmt.Errorf("stage installer output: %w", err)
		}
		artifact, err := inspectArtifact(destination, item.destination)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	if artifactType == "pxe" {
		manifestData, err := json.Marshal(struct {
			Artifacts []domain.Artifact `json:"artifacts"`
		}{Artifacts: artifacts})
		if err != nil {
			return nil, err
		}
		manifestData = append(manifestData, '\n')
		manifestPath := filepath.Join(stage.Root, "artifacts", "pxe", "manifest.json")
		if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
			return nil, err
		}
		manifestArtifact, err := inspectArtifact(manifestPath, "artifacts/pxe/manifest.json")
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, manifestArtifact)
	}
	if err := os.Remove(ArtifactStore{}.InstallerPath(stage)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.Rename(stage.Work, filepath.Join(stage.Root, "state")); err != nil {
		return nil, fmt.Errorf("stage installer state: %w", err)
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts, nil
}

func (ArtifactStore) WriteResultFiles(stage ports.BuildStage, log, run []byte) error {
	if err := os.MkdirAll(filepath.Join(stage.Root, "logs"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage.Root, "logs", "openshift-install.log"), log, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stage.Root, "run.json"), run, 0o600)
}

func (ArtifactStore) WriteFailureLog(output string, log []byte) error {
	clean := filepath.Clean(output)
	stage, err := os.CreateTemp(filepath.Dir(clean), filepath.Base(clean)+".failed-*.log")
	if err != nil {
		return err
	}
	stagePath := stage.Name()
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.Remove(stagePath)
		}
	}()
	if err := stage.Chmod(0o600); err != nil {
		stage.Close()
		return err
	}
	if _, err := stage.Write(log); err != nil {
		stage.Close()
		return err
	}
	if err := stage.Close(); err != nil {
		return err
	}
	succeeded = true
	return nil
}

func (ArtifactStore) Publish(output string, stage ports.BuildStage, force bool) (string, error) {
	_, err := os.Lstat(output)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if exists && !stage.Replace && !force {
		return "", errors.New("completed output exists; use --force to replace it")
	}
	if !exists {
		if err := unix.Renameat2(unix.AT_FDCWD, stage.Root, unix.AT_FDCWD, output, unix.RENAME_NOREPLACE); err != nil {
			return "", fmt.Errorf("publish completed output: %w", err)
		}
		return "create", nil
	}
	if err := unix.Renameat2(unix.AT_FDCWD, stage.Root, unix.AT_FDCWD, output, unix.RENAME_EXCHANGE); err != nil {
		return "", fmt.Errorf("replace completed output: %w", err)
	}
	_ = os.RemoveAll(stage.Root)
	return "update", nil
}

func inspectBuildOutput(output string, inputs []ports.GeneratedFile, force bool) (exists, preserveReports bool, err error) {
	info, err := os.Lstat(output)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("inspect output directory: %w", err)
	}
	if !info.IsDir() {
		return true, false, errors.New("output exists and is not a directory")
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		return true, false, fmt.Errorf("inspect output directory: %w", err)
	}
	completed := false
	for _, entry := range entries {
		switch entry.Name() {
		case "generated", "reports":
		case "artifacts", "logs", "state", "monitor", "run.json":
			completed = true
		default:
			if !force {
				return true, false, fmt.Errorf("output contains unrecognized entry %q; use --force to replace it", entry.Name())
			}
		}
	}
	if completed && !force {
		return true, false, errors.New("completed output exists; use --force to replace it")
	}
	compatible := true
	if _, err := os.Lstat(filepath.Join(output, "generated")); err == nil {
		action, _, compareErr := compareGenerated(filepath.Join(output, "generated"), inputs)
		if compareErr != nil {
			return true, false, compareErr
		}
		compatible = action == "unchanged"
		if !compatible && !force {
			return true, false, errors.New("generated output differs; use --force to replace it")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return true, false, fmt.Errorf("inspect generated output: %w", err)
	}
	return true, compatible, nil
}

func copyReports(source, destination string) error {
	info, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() {
		return errors.New("reports path is not a safe directory")
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("inspect reports directory: %w", err)
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		return fmt.Errorf("stage reports directory: %w", err)
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("reports directory contains a symlink")
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("reports directory contains a non-regular file")
		}
		data, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			return fmt.Errorf("read report %q: %w", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), data, 0o600); err != nil {
			return fmt.Errorf("stage report %q: %w", entry.Name(), err)
		}
	}
	return nil
}

func (ArtifactStore) Discard(stage ports.BuildStage) {
	_ = os.RemoveAll(stage.Root)
	if stage.Lock != nil {
		_ = stage.Lock.Close()
	}
}

func (ArtifactStore) PrepareMonitor(output string) (ports.BuildStage, error) {
	lock, err := lockOutput(output)
	if err != nil {
		return ports.BuildStage{}, err
	}
	outputInfo, err := os.Lstat(output)
	if err != nil || !outputInfo.IsDir() {
		lock.Close()
		return ports.BuildStage{}, errors.New("published output directory is unavailable or unsafe")
	}
	info, err := os.Lstat(filepath.Join(output, "state"))
	if err != nil || !info.IsDir() {
		lock.Close()
		return ports.BuildStage{}, errors.New("published installer state is unavailable")
	}
	root, err := os.MkdirTemp(output, ".monitor-")
	if err != nil {
		lock.Close()
		return ports.BuildStage{}, err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		os.RemoveAll(root)
		lock.Close()
		return ports.BuildStage{}, err
	}
	return ports.BuildStage{Root: root, Work: filepath.Join(output, "state"), Lock: lock}, nil
}

func (ArtifactStore) MonitorAction(stage ports.BuildStage) (string, error) {
	target := filepath.Join(filepath.Dir(stage.Root), "monitor")
	_, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return "create", nil
	}
	if err != nil {
		return "", err
	}
	return "update", nil
}

func (ArtifactStore) PublishMonitor(output string, stage ports.BuildStage, log, result []byte, force bool) (string, error) {
	if err := os.WriteFile(filepath.Join(stage.Root, "openshift-install.log"), log, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stage.Root, "result.json"), result, 0o600); err != nil {
		return "", err
	}
	target := filepath.Join(output, "monitor")
	_, err := os.Lstat(target)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if exists && !force {
		return "", errors.New("monitor result exists; use --force to replace it")
	}
	if !exists {
		return "create", unix.Renameat2(unix.AT_FDCWD, stage.Root, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
	}
	if err := unix.Renameat2(unix.AT_FDCWD, stage.Root, unix.AT_FDCWD, target, unix.RENAME_EXCHANGE); err != nil {
		return "", err
	}
	_ = os.RemoveAll(stage.Root)
	return "update", nil
}

func inspectArtifact(path, relative string) (domain.Artifact, error) {
	file, err := os.Open(path)
	if err != nil {
		return domain.Artifact{}, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return domain.Artifact{}, errors.Join(copyErr, closeErr)
	}
	return domain.Artifact{Path: filepath.ToSlash(relative), Size: size, SHA256: "sha256:" + hex.EncodeToString(hash.Sum(nil))}, nil
}

type outputLock struct{ file *os.File }

func lockOutput(output string) (*outputLock, error) {
	clean := filepath.Clean(output)
	lockPath := filepath.Join(filepath.Dir(clean), "."+filepath.Base(clean)+".lock")
	fd, err := unix.Open(lockPath, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open output lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), lockPath)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG {
		file.Close()
		return nil, errors.New("output lock is not a regular file")
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("another operation owns the output directory")
	}
	return &outputLock{file: file}, nil
}

func (l *outputLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	return errors.Join(err, l.file.Close())
}
