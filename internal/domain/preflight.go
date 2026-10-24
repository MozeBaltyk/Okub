package domain

const (
	CheckPass = "pass"
	CheckFail = "fail"
	CheckSkip = "skip"
)

// PreflightCheck is one secret-free environmental observation.
type PreflightCheck struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Path    string `json:"path"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// PreflightReport is the complete, deterministic result of a preflight run.
type PreflightReport struct {
	Valid   bool             `json:"valid"`
	Offline bool             `json:"offline"`
	Checks  []PreflightCheck `json:"checks"`
}
