package ports

import (
	"context"
	"io"
	"time"

	"github.com/mozebaltyk/Okub/internal/domain"
)

type InstallerExtractor interface {
	Extract(io.Reader, string) error
}

type ToolExtractor interface {
	ExtractExecutable(io.Reader, string, string) error
}

type InstallerCommand struct {
	Args        []string
	Directory   string
	Environment map[string]string
	Secrets     []string
}

type InstallerResult struct {
	Args       []string
	Log        []byte
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   int
}

type Installer interface {
	Version(context.Context, string) (string, error)
	Run(context.Context, string, InstallerCommand) (InstallerResult, error)
}

type BuildStage struct {
	Root    string
	Work    string
	Lock    io.Closer
	Replace bool
}

type ArtifactStore interface {
	Prepare(string, []GeneratedFile, bool) (BuildStage, error)
	InstallerPath(BuildStage) string
	FinalizeArtifacts(BuildStage, string, string) ([]domain.Artifact, error)
	WriteResultFiles(BuildStage, []byte, []byte) error
	WriteFailureLog(string, []byte) error
	Publish(string, BuildStage, bool) (string, error)
	Discard(BuildStage)
	PrepareMonitor(string) (BuildStage, error)
	MonitorAction(BuildStage) (string, error)
	PublishMonitor(string, BuildStage, []byte, []byte, bool) (string, error)
}
