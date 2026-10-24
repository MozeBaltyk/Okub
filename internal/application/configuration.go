// Package application coordinates Okub use cases across domain rules and ports.
package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
	configschemas "github.com/mozebaltyk/Okub/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
)

// ConfigurationValidator implements the read-only configuration validation workflow.
type ConfigurationValidator struct {
	Source                  ports.ConfigurationSource
	CacheDirectory          string
	OutputDirectoryRoot     string
	HomeDirectory           string
	OutputDirectoryOverride string
}

// ConfigurationResult contains the normalized definition and its report.
type ConfigurationResult struct {
	Definition domain.ClusterDefinition
	Report     domain.ValidationReport
}

// ReadError identifies a filesystem failure at the configuration boundary.
type ReadError struct {
	Path string
	Err  error
}

func (e *ReadError) Error() string { return fmt.Sprintf("read configuration %q: %v", e.Path, e.Err) }
func (e *ReadError) Unwrap() error { return e.Err }

// Validate reads one strict YAML document, applies defaults, and runs pure checks.
func (v ConfigurationValidator) Validate(path string) (ConfigurationResult, error) {
	data, err := v.Source.Read(path)
	if err != nil {
		return ConfigurationResult{}, &ReadError{Path: path, Err: err}
	}

	var raw any
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&raw); err != nil {
		return decodeFailure(domain.ClusterDefinition{}, err), nil
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple YAML documents are not supported")
		}
		return decodeFailure(domain.ClusterDefinition{}, err), nil
	}

	supportedVersion := false
	if document, ok := raw.(map[string]any); ok {
		if version, ok := document["apiVersion"].(string); ok && version != "okub.io/v1alpha1" {
			return ConfigurationResult{Report: domain.ValidationReport{Findings: []domain.Finding{{
				ID: "apiVersion.unsupported", Severity: domain.SeverityError, Path: "apiVersion", Message: "API version is not supported",
			}}}}, nil
		} else if ok {
			supportedVersion = true
		}
	}

	instanceJSON, err := json.Marshal(raw)
	if err != nil {
		return decodeFailure(domain.ClusterDefinition{}, errors.New("YAML contains a non-JSON value")), nil
	}
	var instance any
	if err := json.Unmarshal(instanceJSON, &instance); err != nil {
		return decodeFailure(domain.ClusterDefinition{}, errors.New("YAML cannot be represented as JSON")), nil
	}
	schema, err := configschemas.V1Alpha1()
	if err != nil {
		return ConfigurationResult{}, fmt.Errorf("compile configuration schema: %w", err)
	}
	schemaErr := schema.Validate(instance)

	var definition domain.ClusterDefinition
	decoder = yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&definition); err != nil {
		if schemaErr != nil {
			return ConfigurationResult{Report: schemaFailure(schemaErr)}, nil
		}
		return decodeFailure(definition, err), nil
	}
	if v.OutputDirectoryOverride != "" {
		definition.Artifacts.OutputDirectory = v.OutputDirectoryOverride
	}
	outputRoot := v.OutputDirectoryRoot
	if outputRoot == "" {
		outputRoot = filepath.Join(".", ".local", "build")
	}
	domain.ApplyDefaults(&definition, outputRoot, v.CacheDirectory, v.HomeDirectory)
	report := domain.Validate(definition)
	if schemaErr != nil {
		if !supportedVersion {
			report = withoutFinding(report, "apiVersion.unsupported")
		}
		report = mergeReports(schemaFailure(schemaErr), report)
	}
	return ConfigurationResult{Definition: definition, Report: report}, nil
}

func withoutFinding(report domain.ValidationReport, id string) domain.ValidationReport {
	findings := make([]domain.Finding, 0, len(report.Findings))
	for _, finding := range report.Findings {
		if finding.ID != id {
			findings = append(findings, finding)
		}
	}
	return domain.ValidationReport{Valid: len(findings) == 0, Findings: findings}
}

func decodeFailure(definition domain.ClusterDefinition, _ error) ConfigurationResult {
	report := domain.ValidationReport{Findings: []domain.Finding{{
		ID: "config.decode", Severity: domain.SeverityError, Path: "$", Message: "configuration cannot be decoded into ClusterDefinition",
	}}}
	return ConfigurationResult{Definition: definition, Report: report}
}

func schemaFailure(err error) domain.ValidationReport {
	findings := []domain.Finding{}
	var validationErr *jsonschema.ValidationError
	if errors.As(err, &validationErr) {
		var visit func(*jsonschema.ValidationError)
		visit = func(current *jsonschema.ValidationError) {
			if len(current.Causes) != 0 {
				for _, cause := range current.Causes {
					visit(cause)
				}
				return
			}
			keywordPath := current.ErrorKind.KeywordPath()
			keyword := "constraint"
			if len(keywordPath) != 0 {
				keyword = keywordPath[len(keywordPath)-1]
			}
			findings = append(findings, domain.Finding{
				ID: "config.schema", Severity: domain.SeverityError, Path: configurationPath(current.InstanceLocation),
				Message: "value does not satisfy schema " + keyword,
			})
		}
		visit(validationErr)
	} else {
		findings = append(findings, domain.Finding{ID: "config.schema", Severity: domain.SeverityError, Path: "$", Message: "configuration does not satisfy schema"})
	}
	sortFindings(findings)
	return domain.ValidationReport{Findings: findings}
}

func mergeReports(reports ...domain.ValidationReport) domain.ValidationReport {
	findings := []domain.Finding{}
	seen := map[domain.Finding]struct{}{}
	for _, report := range reports {
		for _, finding := range report.Findings {
			if _, found := seen[finding]; found {
				continue
			}
			seen[finding] = struct{}{}
			findings = append(findings, finding)
		}
	}
	sortFindings(findings)
	return domain.ValidationReport{Valid: len(findings) == 0, Findings: findings}
}

func sortFindings(findings []domain.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		if findings[i].ID != findings[j].ID {
			return findings[i].ID < findings[j].ID
		}
		return findings[i].Message < findings[j].Message
	})
}

func configurationPath(parts []string) string {
	if len(parts) == 0 {
		return "$"
	}
	var path strings.Builder
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err == nil {
			fmt.Fprintf(&path, "[%s]", part)
		} else {
			if path.Len() != 0 {
				path.WriteByte('.')
			}
			path.WriteString(part)
		}
	}
	return path.String()
}
