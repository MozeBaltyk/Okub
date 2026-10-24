package domain

// Release is one exact OKD release and its checksum-pinned inputs.
type Release struct {
	RequestedVersion string         `json:"requestedVersion"`
	Version          string         `json:"version"`
	Architecture     string         `json:"architecture"`
	InstallerVersion string         `json:"installerVersion"`
	ImagePullSpec    string         `json:"imagePullSpec"`
	ImageDigest      string         `json:"imageDigest"`
	Inputs           []ReleaseInput `json:"inputs"`
}

// ReleaseInput is one downloadable, content-addressed release input.
type ReleaseInput struct {
	Name   string `json:"name"`
	URL    string `json:"-"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}
