// Package filesystem provides local filesystem adapters.
package filesystem

import "os"

// ConfigurationSource reads configuration from the local filesystem.
type ConfigurationSource struct{}

func (ConfigurationSource) Read(path string) ([]byte, error) {
	return os.ReadFile(path)
}
