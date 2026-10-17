package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := execute(args)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			if !strings.Contains(stdout, "Usage:\n  okub [command]") || !strings.Contains(stdout, "version") {
				t.Fatalf("stdout does not contain root help:\n%s", stdout)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := execute(args)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			if stdout != "okub test-version\n" {
				t.Fatalf("stdout = %q, want version", stdout)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
		})
	}
}

func TestInvalidUsage(t *testing.T) {
	stdout, stderr, code := execute([]string{"bogus"})
	if code != exitInvalidUsage {
		t.Fatalf("exit code = %d, want %d", code, exitInvalidUsage)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if stderr != "okub: unknown command \"bogus\" for \"okub\"\n" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func execute(args []string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := Execute(args, "test-version", &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}
