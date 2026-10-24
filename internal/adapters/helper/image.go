package helper

import (
	"bufio"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	ImageFilename          = "Fedora-Cloud-Base-Generic-44-1.7.x86_64.qcow2"
	ChecksumFilename       = "Fedora-Cloud-44-1.7-x86_64-CHECKSUM"
	ImageURL               = "https://download.fedoraproject.org/pub/fedora/linux/releases/44/Cloud/x86_64/images/" + ImageFilename
	ChecksumURL            = "https://download.fedoraproject.org/pub/fedora/linux/releases/44/Cloud/x86_64/images/" + ChecksumFilename
	maxImageSize     int64 = 2 << 30
)

//go:embed fedora-44.gpg
var fedora44Key []byte

type SignatureVerifier interface {
	Verify(context.Context, string, []byte) error
}
type GPGV struct{}

func (GPGV) Verify(ctx context.Context, path string, key []byte) error {
	dir, err := os.MkdirTemp("", "okub-gpgv-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	ring := filepath.Join(dir, "fedora-44.gpg")
	if err = os.WriteFile(ring, key, 0o600); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "gpgv", "--keyring", ring, path)
	if out, e := cmd.CombinedOutput(); e != nil {
		return fmt.Errorf("Fedora CHECKSUM OpenPGP signature verification failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

type ImageSync struct {
	Client                *http.Client
	Verifier              SignatureVerifier
	ImageURL, ChecksumURL string
	AllowHTTP             bool
}
type ImageResult struct {
	ImagePath string `json:"imagePath"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Reused    bool   `json:"reused"`
}
type imageManifest struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func (s ImageSync) Sync(ctx context.Context, repo string, offline bool) (ImageResult, error) {
	imageURL, checksumURL := s.ImageURL, s.ChecksumURL
	if imageURL == "" {
		imageURL = ImageURL
	}
	if checksumURL == "" {
		checksumURL = ChecksumURL
	}
	if !s.AllowHTTP {
		for _, raw := range []string{imageURL, checksumURL} {
			u, e := url.Parse(raw)
			if e != nil || u.Scheme != "https" || u.User != nil {
				return ImageResult{}, errors.New("Fedora image URLs must use credential-free HTTPS")
			}
		}
	}
	dir := filepath.Join(repo, ".local", "cache", "fedora", "44")
	if err := ensurePrivatePath(repo, ".local", "cache", "fedora", "44"); err != nil {
		return ImageResult{}, err
	}
	checksumPath := filepath.Join(dir, ChecksumFilename)
	imagePath := filepath.Join(dir, ImageFilename)
	if offline {
		digest, expectedSize, err := manifestDigest(filepath.Join(dir, "manifest.json"), imageURL)
		if err != nil {
			return ImageResult{}, err
		}
		size, err := verifyFile(imagePath, digest, maxImageSize)
		if err == nil && size != expectedSize {
			err = errors.New("cached Fedora image size does not match its manifest")
		}
		return ImageResult{imagePath, digest, size, true}, err
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "https" || req.URL.User != nil {
				return errors.New("unsafe redirect")
			}
			return nil
		}}
	}
	if err := download(ctx, client, checksumURL, checksumPath, 1<<20); err != nil {
		return ImageResult{}, err
	}
	verifier := s.Verifier
	if verifier == nil {
		verifier = GPGV{}
	}
	if err := verifier.Verify(ctx, checksumPath, fedora44Key); err != nil {
		return ImageResult{}, err
	}
	checksum, err := os.ReadFile(checksumPath)
	if err != nil {
		return ImageResult{}, err
	}
	digest, err := parseChecksum(checksum, ImageFilename)
	if err != nil {
		return ImageResult{}, err
	}
	if size, e := verifyFile(imagePath, digest, maxImageSize); e == nil {
		if err := writeManifest(filepath.Join(dir, "manifest.json"), imageManifest{imageURL, digest, size}); err != nil {
			return ImageResult{}, err
		}
		return ImageResult{imagePath, digest, size, true}, nil
	}
	if _, statErr := os.Lstat(imagePath); statErr == nil {
		return ImageResult{}, errors.New("cached Fedora image is invalid; remove it explicitly before retrying")
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return ImageResult{}, statErr
	}
	if err = download(ctx, client, imageURL, imagePath, maxImageSize); err != nil {
		return ImageResult{}, err
	}
	size, err := verifyFile(imagePath, digest, maxImageSize)
	if err != nil {
		_ = os.Remove(imagePath)
		return ImageResult{}, err
	}
	if err = writeManifest(filepath.Join(dir, "manifest.json"), imageManifest{imageURL, digest, size}); err != nil {
		return ImageResult{}, err
	}
	return ImageResult{imagePath, digest, size, false}, nil
}

func ensurePrivatePath(root string, parts ...string) error {
	current := root
	sharedCacheParent := len(parts) >= 2 && parts[0] == ".local" && parts[1] == "cache"
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err = os.Mkdir(current, 0o700); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		modeOK := info.Mode().Perm() == 0o700
		if sharedCacheParent && index == 1 {
			modeOK = info.Mode().Perm()&0o022 == 0
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !modeOK {
			return errors.New("helper local path contains a symlink, non-directory, or nonprivate directory")
		}
	}
	return nil
}

func download(ctx context.Context, c *http.Client, raw, path string, max int64) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Size() > max {
			return errors.New("cache target is not a bounded regular file")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return e
	}
	resp, e := c.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > max {
		return errors.New("download exceeds size limit")
	}
	tmp, e := os.CreateTemp(filepath.Dir(path), ".download-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0o600)
	n, e := io.Copy(tmp, io.LimitReader(resp.Body, max+1))
	if e == nil && n > max {
		e = errors.New("download exceeds size limit")
	}
	if e == nil {
		e = tmp.Sync()
	}
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e == nil {
		_ = os.Chmod(name, 0o644)
		e = os.Link(name, path)
		if errors.Is(e, os.ErrExist) {
			identical, compareErr := identicalRegularFiles(name, path, max)
			if compareErr != nil {
				return compareErr
			}
			if identical {
				return nil
			}
			return errors.New("cache target was concurrently created with different content")
		}
	}
	return e
}

func identicalRegularFiles(first, second string, max int64) (bool, error) {
	digest := func(path string) (int64, [sha256.Size]byte, error) {
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return 0, [sha256.Size]byte{}, err
		}
		file := os.NewFile(uintptr(fd), path)
		defer file.Close()
		var stat unix.Stat_t
		if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o777 != 0o644 || stat.Nlink != 1 || stat.Size > max || stat.Size < 0 {
			return 0, [sha256.Size]byte{}, errors.New("cache race winner is not a bounded regular file")
		}
		hash := sha256.New()
		if _, err = io.Copy(hash, file); err != nil {
			return 0, [sha256.Size]byte{}, err
		}
		var sum [sha256.Size]byte
		copy(sum[:], hash.Sum(nil))
		return stat.Size, sum, nil
	}
	firstSize, firstDigest, err := digest(first)
	if err != nil {
		return false, err
	}
	secondSize, secondDigest, err := digest(second)
	return firstSize == secondSize && firstDigest == secondDigest, err
}
func parseChecksum(data []byte, name string) (string, error) {
	var found string
	s := bufio.NewScanner(strings.NewReader(string(data)))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		var digest string
		if prefix := "SHA256 (" + name + ") = "; strings.HasPrefix(line, prefix) {
			digest = strings.TrimPrefix(line, prefix)
		}
		if fields := strings.Fields(line); len(fields) == 2 && (fields[1] == name || fields[1] == "*"+name) {
			digest = fields[0]
		}
		if len(digest) == 64 {
			if _, e := hex.DecodeString(digest); e == nil {
				digest = strings.ToLower(digest)
				if found != "" && found != digest {
					return "", errors.New("conflicting image checksums")
				}
				found = digest
			}
		}
	}
	if found == "" {
		return "", errors.New("signed CHECKSUM does not contain the exact Fedora image SHA-256")
	}
	return found, s.Err()
}
func verifyFile(path, digest string, max int64) (int64, error) {
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e != nil {
		return 0, e
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > max {
		return 0, errors.New("cached Fedora image is not a bounded regular file")
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return 0, e
	}
	if hex.EncodeToString(h.Sum(nil)) != digest {
		return 0, errors.New("cached Fedora image SHA-256 mismatch")
	}
	return info.Size(), nil
}
func writeManifest(path string, m imageManifest) error {
	data, _ := json.MarshalIndent(m, "", "  ")
	data = append(data, '\n')
	if existing, err := readPrivateManifest(path); err == nil {
		if string(existing) == string(data) {
			return nil
		}
		return errors.New("Fedora image manifest already exists with different content")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".manifest-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Link(name, path)
		if errors.Is(err, os.ErrExist) {
			existing, readErr := readPrivateManifest(path)
			if readErr != nil {
				return readErr
			}
			if string(existing) == string(data) {
				return nil
			}
			return errors.New("Fedora image manifest was concurrently created with different content")
		}
	}
	return err
}

func readPrivateManifest(path string) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o777 != 0o600 || stat.Nlink != 1 || stat.Size > 1<<20 || stat.Size < 0 {
		return nil, errors.New("Fedora image manifest cache target is not a private regular file")
	}
	return io.ReadAll(file)
}
func manifestDigest(path, imageURL string) (string, int64, error) {
	data, e := readPrivateManifest(path)
	if e != nil {
		return "", 0, errors.New("verified Fedora image manifest is not cached")
	}
	var m imageManifest
	if json.Unmarshal(data, &m) != nil || m.URL != imageURL || len(m.SHA256) != 64 || m.Size <= 0 || m.Size > maxImageSize {
		return "", 0, errors.New("verified Fedora image manifest is invalid")
	}
	return m.SHA256, m.Size, nil
}
