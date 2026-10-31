package application

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"

	"github.com/mozebaltyk/Okub/internal/domain"
	"github.com/mozebaltyk/Okub/internal/ports"
)

type ArtifactServer struct {
	Validator ConfigurationValidator
	Source    ports.ArtifactSetSource
	Network   ports.LocalAddressSource
	Transport ports.ArtifactTransport
	PXE       ports.PXETransport
}

type ArtifactServingNetworkError struct{ Err error }

func (e *ArtifactServingNetworkError) Error() string {
	return fmt.Sprintf("artifact serving network failure: %v", e.Err)
}
func (e *ArtifactServingNetworkError) Unwrap() error { return e.Err }

type ArtifactServeRequest struct {
	ConfigPath              string
	OutputDirectoryOverride string
	ManifestPath            string
	Mode                    string
	BindAddress             string
	AdvertisedAddress       string
	Port                    int
	PXEPorts                ports.PXEPorts
	Ready                   func(ArtifactServeResult) error
}

type ArtifactServeResult struct {
	State            string   `json:"state"`
	Mode             string   `json:"mode"`
	Endpoint         string   `json:"endpoint"`
	Artifacts        []string `json:"artifacts"`
	PixiecoreVersion string   `json:"pixiecoreVersion,omitempty"`
	IPXESHA256       string   `json:"ipxeSHA256,omitempty"`
}

func (s ArtifactServer) Serve(ctx context.Context, request ArtifactServeRequest) (ArtifactServeResult, domain.ValidationReport, error) {
	if request.Mode != "http" && request.Mode != "pxe" {
		return ArtifactServeResult{}, domain.ValidationReport{}, &CompatibilityError{Message: "artifact serving mode must be http or pxe"}
	}
	if request.Port < 1 || request.Port > 65535 {
		return ArtifactServeResult{}, domain.ValidationReport{}, &CompatibilityError{Message: "HTTP port must be between 1 and 65535"}
	}
	bind, err := netip.ParseAddr(request.BindAddress)
	if err != nil {
		return ArtifactServeResult{}, domain.ValidationReport{}, &CompatibilityError{Message: "bind address must be an explicit IP address"}
	}
	advertised, err := netip.ParseAddr(request.AdvertisedAddress)
	if err != nil || advertised.IsUnspecified() || advertised.IsLoopback() {
		return ArtifactServeResult{}, domain.ValidationReport{}, &CompatibilityError{Message: "advertised address must be an explicit non-wildcard, non-loopback IP address"}
	}
	if !bind.IsUnspecified() && bind.Unmap() != advertised.Unmap() {
		return ArtifactServeResult{}, domain.ValidationReport{}, &CompatibilityError{Message: "advertised address must match the explicitly bound interface address"}
	}
	validator := s.Validator
	validator.OutputDirectoryOverride = request.OutputDirectoryOverride
	validated, err := validator.Validate(request.ConfigPath)
	if err != nil || !validated.Report.Valid {
		return ArtifactServeResult{}, validated.Report, err
	}
	machineNetwork, err := netip.ParsePrefix(validated.Definition.Network.MachineCIDR)
	if err != nil || !machineNetwork.Contains(advertised) {
		return ArtifactServeResult{}, serveValidation(validated.Report, "serve.addressOutsideMachineCIDR", "--advertise", "advertised address is outside the configured machine network"), nil
	}
	local, err := s.Network.HasLocalAddress(advertised)
	if err != nil {
		return ArtifactServeResult{}, validated.Report, fmt.Errorf("inspect local addresses: %w", err)
	}
	if !local {
		return ArtifactServeResult{}, serveValidation(validated.Report, "serve.addressNotLocal", "--advertise", "advertised address is not assigned to a local interface"), nil
	}
	set, err := s.Source.LoadAndVerify(request.ManifestPath)
	if err != nil {
		return ArtifactServeResult{}, validated.Report, err
	}
	endpoint := "http://" + net.JoinHostPort(advertised.String(), fmt.Sprint(request.Port))
	result := ArtifactServeResult{State: "ready", Mode: request.Mode, Endpoint: endpoint, Artifacts: make([]string, 0, len(set.Artifacts))}
	for _, artifact := range set.Artifacts {
		result.Artifacts = append(result.Artifacts, endpoint+"/"+artifact.Path)
	}
	ready := request.Ready
	if ready == nil {
		ready = func(ArtifactServeResult) error { return nil }
	}
	if request.Mode == "pxe" {
		if validated.Definition.Cluster.Architecture != "x86_64" {
			return ArtifactServeResult{}, serveValidation(validated.Report, "serve.unsupportedPXEArchitecture", "cluster.architecture", "PXE serving supports only x86_64 UEFI clients"), nil
		}
		scriptPath := "artifacts/pxe/agent." + validated.Definition.Cluster.Architecture + ".ipxe"
		if !hasPublishedArtifact(set, scriptPath) {
			return ArtifactServeResult{}, serveValidation(validated.Report, "serve.missingIPXEScript", "--manifest", "PXE serving requires the installer-published architecture-specific iPXE script"), nil
		}
		expectedBaseURL := endpoint + "/artifacts/pxe"
		if validated.Definition.Boot.Method != "pxe" || validated.Definition.Boot.ArtifactsBaseURL != expectedBaseURL {
			return ArtifactServeResult{}, serveValidation(validated.Report, "serve.artifactsBaseURLMismatch", "boot.artifactsBaseURL", "PXE artifact base URL does not match the advertised serving endpoint"), nil
		}
		for _, item := range []struct {
			name string
			port int
		}{{"DHCP", request.PXEPorts.DHCP}, {"TFTP", request.PXEPorts.TFTP}, {"PXE", request.PXEPorts.PXE}, {"Pixiecore HTTP", request.PXEPorts.HTTP}} {
			if item.port < 1 || item.port > 65535 {
				return ArtifactServeResult{}, validated.Report, &CompatibilityError{Message: fmt.Sprintf("%s port must be between 1 and 65535", item.name)}
			}
		}
		if s.PXE == nil {
			return ArtifactServeResult{}, validated.Report, errors.New("PXE transport is not configured")
		}
		macs := make([]string, 0)
		for _, host := range validated.Definition.Hosts {
			for _, iface := range host.Interfaces {
				mac, parseErr := net.ParseMAC(iface.MACAddress)
				if parseErr != nil {
					return ArtifactServeResult{}, validated.Report, fmt.Errorf("validated MAC address %q is invalid: %w", iface.MACAddress, parseErr)
				}
				macs = append(macs, mac.String())
			}
		}
		err = s.PXE.Serve(ctx, set, netip.AddrPortFrom(bind, uint16(request.Port)), advertised, request.PXEPorts, macs, func(info ports.PXEInfo) error {
			result.PixiecoreVersion = info.PixiecoreVersion
			result.IPXESHA256 = info.IPXESHA256
			return ready(result)
		})
		result.State = "stopped"
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			err = &ArtifactServingNetworkError{Err: err}
		}
		return result, validated.Report, err
	}
	err = s.Transport.Serve(ctx, set, netip.AddrPortFrom(bind, uint16(request.Port)), func() error { return ready(result) })
	result.State = "stopped"
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		err = &ArtifactServingNetworkError{Err: err}
	}
	return result, validated.Report, err
}

func hasPublishedArtifact(set ports.PublishedArtifactSet, path string) bool {
	for _, artifact := range set.Artifacts {
		if artifact.Path == path {
			return true
		}
	}
	return false
}

func serveValidation(report domain.ValidationReport, id, path, message string) domain.ValidationReport {
	report.Valid = false
	report.Findings = append(report.Findings, domain.Finding{ID: id, Severity: domain.SeverityError, Path: path, Message: message})
	return report
}
