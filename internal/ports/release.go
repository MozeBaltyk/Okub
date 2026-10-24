package ports

import (
	"context"
	"errors"
	"io"

	"github.com/mozebaltyk/Okub/internal/domain"
)

var ErrIntegrity = errors.New("download integrity check failed")

// ReleaseResolver resolves one requested stream/version to exact pinned inputs.
type ReleaseResolver interface {
	Resolve(context.Context, string, string) (domain.Release, error)
}

// Downloader opens one bounded-lifetime artifact response.
type Downloader interface {
	Open(context.Context, string) (io.ReadCloser, error)
}

// ReleaseCache stores checksum-addressed blobs and deterministic release manifests.
type ReleaseCache interface {
	Lookup(string, string, string) (domain.Release, bool, error)
	OpenBlob(string, domain.ReleaseInput) (io.ReadCloser, error)
	HasBlob(string, domain.ReleaseInput) (bool, error)
	StoreBlob(string, domain.ReleaseInput, io.Reader) error
	Publish(string, domain.Release) error
}
