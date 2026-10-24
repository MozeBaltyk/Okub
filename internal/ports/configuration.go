// Package ports defines external capabilities required by application workflows.
package ports

// ConfigurationSource reads a desired-state configuration document.
type ConfigurationSource interface {
	Read(path string) ([]byte, error)
}

// CredentialSource reads a referenced credential without exposing it to presentation code.
type CredentialSource interface {
	ReadCredential(path string) ([]byte, error)
}

// GeneratedFile is one sensitive installer input to publish.
type GeneratedFile struct {
	Name    string
	Content []byte
}

// GeneratedPublisher atomically publishes a complete generated directory.
type GeneratedPublisher interface {
	Publish(outputDirectory string, files []GeneratedFile, force, dryRun bool) (string, error)
}
