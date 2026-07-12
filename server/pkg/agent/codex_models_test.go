package agent

import "testing"

func TestParseCodexModelCatalogFiltersHiddenAndMarksFirstDefault(t *testing.T) {
	raw := []byte(`{"models":[{"slug":"gpt-5.6-sol","display_name":"GPT-5.6 Sol","visibility":"list","default_reasoning_level":"low","supported_reasoning_levels":[{"effort":"low","description":"fast"},{"effort":"ultra","description":"deep"}]},{"slug":"hidden","display_name":"Hidden","visibility":"hide"},{"slug":"gpt-5.5","display_name":"","visibility":"list"}]}`)
	models, err := parseCodexModelCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %+v, want two visible entries", models)
	}
	if models[0].ID != "gpt-5.6-sol" || !models[0].Default {
		t.Fatalf("first model = %+v, want default gpt-5.6-sol", models[0])
	}
	if models[0].Thinking == nil || models[0].Thinking.DefaultLevel != "low" || len(models[0].Thinking.SupportedLevels) != 2 || models[0].Thinking.SupportedLevels[1].Value != "ultra" {
		t.Fatalf("thinking catalog = %+v", models[0].Thinking)
	}
	if models[1].Label != "gpt-5.5" || models[1].Default {
		t.Fatalf("second model = %+v, want slug fallback and no default", models[1])
	}
}

func TestCodexSupportsDebugModels(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"codex-cli 0.121.9", false},
		{"codex-cli 0.122.0", true},
		{"codex-cli 1.0.0", true},
		{"unknown", false},
	} {
		if got := codexSupportsDebugModels(tc.version); got != tc.want {
			t.Fatalf("codexSupportsDebugModels(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}
