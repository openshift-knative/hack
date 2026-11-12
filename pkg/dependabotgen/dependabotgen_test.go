package dependabotgen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteGeneratesRenovateConfigPointer asserts that the generated
// renovate.json is a minimal pointer to the central preset hosted in the
// hack repository, so config changes no longer need to fan out to every
// consuming repository.
func TestWriteGeneratesRenovateConfigPointer(t *testing.T) {
	dir := t.TempDir()

	cfg := NewDependabotConfig()
	cfg.WithGo("main")
	if err := cfg.Write(dir, ""); err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, RenovateConfigPath))
	if err != nil {
		t.Fatalf("failed to read generated %s: %v", RenovateConfigPath, err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("generated %s is not valid JSON: %v\n%s", RenovateConfigPath, err, data)
	}

	extends, ok := config["extends"].([]any)
	if !ok || len(extends) != 1 {
		t.Fatalf("expected %s to extend exactly one preset, got: %v", RenovateConfigPath, config["extends"])
	}
	want := "github>openshift-knative/hack//renovate/default.json"
	if extends[0] != want {
		t.Errorf("expected extends %q, got %q", want, extends[0])
	}

	// The pointer must stay minimal: shared configuration belongs in the
	// central preset, not in files duplicated across consuming repositories.
	for _, field := range []string{"packageRules", "enabledManagers", "ignorePaths", "commitMessagePrefix"} {
		if _, exists := config[field]; exists {
			t.Errorf("generated %s should not set %q - it belongs in the central preset", RenovateConfigPath, field)
		}
	}
}
