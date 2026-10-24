package schemas

import (
	"os"
	"strings"
	"testing"

	"github.com/mozebaltyk/Okub/internal/domain"
	"go.yaml.in/yaml/v3"
)

func TestBaseDomainContractMatchesDomainValidation(t *testing.T) {
	schema, err := V1Alpha1()
	if err != nil {
		t.Fatal(err)
	}
	example, err := os.ReadFile("../examples/sno.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		domain string
		valid  bool
	}{
		{"example.test", true},
		{"a.b", true},
		{"bad..test", false},
		{"-bad.test", false},
		{"bad-.test", false},
		{"Bad.test", false},
		{"*.example.test", false},
		{"example.test.", false},
		{strings.Repeat("a", 64) + ".test", false},
	}
	for _, test := range tests {
		t.Run(test.domain, func(t *testing.T) {
			var document map[string]any
			if err := yaml.Unmarshal(example, &document); err != nil {
				t.Fatal(err)
			}
			document["cluster"].(map[string]any)["baseDomain"] = test.domain
			schemaValid := schema.Validate(document) == nil

			report := domain.Validate(domain.ClusterDefinition{Cluster: domain.Cluster{BaseDomain: test.domain}})
			domainValid := true
			for _, finding := range report.Findings {
				if finding.ID == "dns.domain" {
					domainValid = false
					break
				}
			}
			if schemaValid != test.valid || domainValid != test.valid {
				t.Fatalf("schema valid = %t, domain valid = %t, want %t", schemaValid, domainValid, test.valid)
			}
		})
	}
}

func TestV1Alpha1AcceptsRendezvousAndRejectsRelativeInstallationDisk(t *testing.T) {
	schema, err := V1Alpha1()
	if err != nil {
		t.Fatal(err)
	}
	example, err := os.ReadFile("../examples/sno.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(example, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("example with rendezvousIP is invalid: %v", err)
	}
	document["hosts"].([]any)[0].(map[string]any)["installationDisk"] = "vda"
	if err := schema.Validate(document); err == nil {
		t.Fatal("schema accepted a relative installation disk")
	}
}

func TestV1Alpha1MACsRequireColonForm(t *testing.T) {
	schema, err := V1Alpha1()
	if err != nil {
		t.Fatal(err)
	}
	example, err := os.ReadFile("../examples/sno.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(example, &document); err != nil {
		t.Fatal(err)
	}
	document["hosts"].([]any)[0].(map[string]any)["interfaces"].([]any)[0].(map[string]any)["macAddress"] = "52-54-00-00-00-10"
	if err := schema.Validate(document); err == nil {
		t.Fatal("schema accepted a hyphenated host MAC")
	}
	if err := yaml.Unmarshal(example, &document); err != nil {
		t.Fatal(err)
	}
	document["helper"].(map[string]any)["wanMAC"] = "02-00-00-00-10-01"
	if err := schema.Validate(document); err == nil {
		t.Fatal("schema accepted a hyphenated helper MAC")
	}
}
