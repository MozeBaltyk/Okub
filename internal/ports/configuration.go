// Package ports defines external capabilities required by application workflows.
package ports

// ConfigurationSource reads a desired-state configuration document.
type ConfigurationSource interface {
	Read(path string) ([]byte, error)
}
