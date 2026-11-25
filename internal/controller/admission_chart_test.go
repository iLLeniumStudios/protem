package controller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The requester-identity policy exists twice: config/admission for kustomize and a chart template.
// Only the two prefix values may differ.
func TestChartAdmissionPolicyMatchesConfig(t *testing.T) {
	rules := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		_, after, found := strings.Cut(string(data), "  matchConditions:\n")
		if !found {
			t.Fatalf("%s has no matchConditions", path)
		}
		body, _, _ := strings.Cut(after, "\n---")
		var kept []string
		lines := strings.Split(body, "\n")
		for i := 0; i < len(lines); i++ {
			kept = append(kept, lines[i])
			if strings.HasSuffix(lines[i], "name: usernamePrefix") || strings.HasSuffix(lines[i], "name: groupsPrefix") {
				i++
			}
		}
		return strings.Join(kept, "\n")
	}
	config := rules(filepath.Join("..", "..", "config", "admission", "requester_identity.yaml"))
	chart := rules(filepath.Join("..", "..", "deploy", "helm", "protem", "templates", "admission-policy.yaml"))
	if config != chart {
		t.Errorf("the chart's policy rules differ from config/admission:\nconfig:\n%s\nchart:\n%s", config, chart)
	}
}
